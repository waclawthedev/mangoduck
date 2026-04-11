package conversation

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"strings"
	"time"

	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/config"
	llmchat "mangoduck/internal/llm/chat"
	"mangoduck/internal/telegram/chats"
	"mangoduck/internal/telegram/shared"
	"mangoduck/internal/telegram/tgerr"
	"mangoduck/internal/telegram/tghtml"
)

type Responder interface {
	Reply(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error)
}

const ToolPlaceholderText = "Running a tool..."

const defaultChatRequestTimeout = 30 * time.Second

var (
	placeholderSender = func(c tele.Context, text string) (*tele.Message, error) {
		return c.Bot().Send(c.Recipient(), text)
	}
	placeholderEditor = func(c tele.Context, msg tele.Editable, text string) error {
		_, err := c.Bot().Edit(msg, text, tele.ModeHTML)
		return err
	}
	requestImageBuilder = buildRequestImage
)

func sendReply(c tele.Context, text string) error {
	normalizedText := tghtml.Normalize(text)
	err := c.Send(normalizedText, tele.ModeHTML)
	if err == nil {
		return nil
	}

	if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
		return err
	}

	sanitizedText := tghtml.Sanitize(normalizedText)
	if sanitizedText != "" && sanitizedText != normalizedText {
		err = c.Send(sanitizedText, tele.ModeHTML)
		if err == nil {
			return nil
		}

		if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
			return err
		}
	}

	escapedText := html.EscapeString(normalizedText)
	if escapedText == normalizedText {
		return err
	}

	return c.Send(escapedText, tele.ModeHTML)
}

func editPlaceholderReply(c tele.Context, msg tele.Editable, text string) error {
	normalizedText := tghtml.Normalize(text)
	err := placeholderEditor(c, msg, normalizedText)
	if err == nil {
		return nil
	}

	if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
		return err
	}

	sanitizedText := tghtml.Sanitize(normalizedText)
	if sanitizedText != "" && sanitizedText != normalizedText {
		err = placeholderEditor(c, msg, sanitizedText)
		if err == nil {
			return nil
		}

		if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
			return err
		}
	}

	escapedText := html.EscapeString(normalizedText)
	if escapedText == normalizedText {
		return err
	}

	return placeholderEditor(c, msg, escapedText)
}

func Chat(cfg config.Config, chatsRepo chats.Repository, responder Responder) func(tele.Context) error {
	return func(c tele.Context) error {
		sender := c.Sender()
		if sender == nil {
			return errors.New("sender is nil")
		}

		message := strings.TrimSpace(c.Text())
		photo := resolveRequestPhoto(c.Message())
		if message == "" && photo == nil {
			return nil
		}

		message, shouldRespond := normalizeIncomingMessage(c, message)
		if !shouldRespond || (message == "" && photo == nil) {
			return nil
		}

		currentChatRecord, _, err := chats.EnsureCurrentChat(c, chatsRepo)
		if err != nil {
			return err
		}

		_, err = chats.RequireResolvedActiveChat(c, currentChatRecord)
		if err != nil {
			if errors.Is(err, shared.ErrResponseHandled) {
				return nil
			}

			return err
		}

		stopTyping := startTyping(c)
		defer stopTyping()

		replyCtx, cancel := context.WithTimeout(context.Background(), resolveChatRequestTimeout(cfg))
		defer cancel()

		currentMessage := c.Message()
		var placeholderMessage *tele.Message
		var request llmchat.Request
		request.ChatID = c.Chat().ID
		request.UserTGID = sender.ID
		request.Message = buildLLMMessage(sender, currentMessage, message)
		request.Image, err = requestImageBuilder(replyCtx, c, photo)
		if err != nil {
			if strings.TrimSpace(message) == "" {
				return fmt.Errorf("reading telegram photo: %w", err)
			}

			request.Image = nil
		}
		request.IsAdmin = cfg.IsAdminTGID(sender.ID)
		request.NotifyToolCall = func(statusText string) error {
			statusText = strings.TrimSpace(statusText)
			if statusText == "" {
				statusText = ToolPlaceholderText
			}

			if placeholderMessage != nil {
				editErr := editPlaceholderReply(c, placeholderMessage, statusText)
				if editErr != nil && !errors.Is(tgerr.Normalize(editErr), tgerr.ErrMessageNotModified) {
					return fmt.Errorf("updating chat placeholder: %w", editErr)
				}

				return nil
			}

			sentMessage, sendErr := placeholderSender(c, statusText)
			if sendErr != nil {
				return fmt.Errorf("sending chat placeholder: %w", sendErr)
			}

			placeholderMessage = sentMessage
			return nil
		}

		reply, err := responder.Reply(replyCtx, &request)
		if err != nil {
			return fmt.Errorf("creating chat reply: %w", err)
		}

		if reply == nil {
			return errors.New("chat reply is nil")
		}

		if placeholderMessage != nil {
			replyText := strings.TrimSpace(reply.Text)
			if replyText == "" {
				replyText = llmchat.DefaultNoResponseText
			}

			err = editPlaceholderReply(c, placeholderMessage, replyText)
			if err != nil {
				if errors.Is(tgerr.Normalize(err), tgerr.ErrMessageNotModified) {
					return nil
				}

				return fmt.Errorf("editing chat placeholder: %w", err)
			}

			return nil
		}

		return sendReply(c, reply.Text)
	}
}

func resolveRequestPhoto(message *tele.Message) *tele.Photo {
	if message == nil {
		return nil
	}

	if message.Photo != nil {
		return message.Photo
	}

	if message.ReplyTo != nil && message.ReplyTo.Photo != nil {
		return message.ReplyTo.Photo
	}

	if message.ExternalReply != nil && len(message.ExternalReply.Photo) > 0 {
		return &message.ExternalReply.Photo[0]
	}

	return nil
}

func buildRequestImage(ctx context.Context, c tele.Context, photo *tele.Photo) (*llmchat.InputImage, error) {
	if photo == nil {
		return nil, nil
	}

	botAPI := c.Bot()
	if botAPI == nil {
		return nil, errors.New("bot is nil")
	}

	bot, ok := botAPI.(*tele.Bot)
	if !ok {
		return nil, errors.New("bot is not a telebot bot")
	}

	file := photo.MediaFile()
	if file == nil {
		return nil, errors.New("photo file is nil")
	}

	filePath, err := telegramFilePath(ctx, bot, file.FileID)
	if err != nil {
		return nil, err
	}

	fileBytes, err := telegramFileBytes(ctx, bot, filePath)
	if err != nil {
		return nil, err
	}
	if len(fileBytes) == 0 {
		return nil, errors.New("photo file is empty")
	}

	var image llmchat.InputImage
	image.MIMEType = http.DetectContentType(fileBytes)
	image.DataBase64 = base64.StdEncoding.EncodeToString(fileBytes)

	if !strings.HasPrefix(image.MIMEType, "image/") {
		image.MIMEType = "image/jpeg"
	}

	return &image, nil
}

func telegramFilePath(ctx context.Context, botAPI *tele.Bot, fileID string) (string, error) {
	if strings.TrimSpace(fileID) == "" {
		return "", errors.New("photo file id is empty")
	}

	payload := make(map[string]string, 1)
	payload["file_id"] = fileID

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, botAPI.URL+"/bot"+botAPI.Token+"/getFile", bytes.NewReader(payloadBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("telegram getFile returned status %s", resp.Status)
	}

	var telegramResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}

	err = json.NewDecoder(resp.Body).Decode(&telegramResp)
	if err != nil {
		return "", err
	}

	if !telegramResp.OK {
		if strings.TrimSpace(telegramResp.Description) != "" {
			return "", errors.New(telegramResp.Description)
		}

		return "", errors.New("telegram getFile request failed")
	}

	if strings.TrimSpace(telegramResp.Result.FilePath) == "" {
		return "", errors.New("telegram file path is empty")
	}

	return telegramResp.Result.FilePath, nil
}

func telegramFileBytes(ctx context.Context, botAPI *tele.Bot, filePath string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, botAPI.URL+"/file/bot"+botAPI.Token+"/"+filePath, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram file download returned status %s", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func normalizeIncomingMessage(c tele.Context, message string) (string, bool) {
	chat := c.Chat()
	if chat == nil || !isGroupChat(chat) {
		return message, true
	}

	if isDirectReplyToBot(c) {
		return message, true
	}

	botMention, ok := findBotMentionEntity(c)
	if !ok {
		return "", false
	}

	return trimLeadingBotMention(c, message, botMention), true
}

func isDirectReplyToBot(c tele.Context) bool {
	message := c.Message()
	if message == nil || message.ReplyTo == nil || message.ReplyTo.Sender == nil {
		return false
	}

	replySender := message.ReplyTo.Sender
	if !replySender.IsBot {
		return false
	}

	_, botID := resolveBotIdentity(c)
	if botID != 0 && replySender.ID == botID {
		return true
	}

	replyUsername := strings.TrimSpace(replySender.Username)
	botUsername, _ := resolveBotIdentity(c)
	if replyUsername != "" && botUsername != "" && strings.EqualFold(replyUsername, botUsername) {
		return true
	}

	return false
}

func isGroupChat(chat *tele.Chat) bool {
	if chat == nil {
		return false
	}

	return chat.Type == tele.ChatGroup || chat.Type == tele.ChatSuperGroup
}

func findBotMentionEntity(c tele.Context) (tele.MessageEntity, bool) {
	entities := c.Entities()
	if len(entities) == 0 {
		return tele.MessageEntity{}, false
	}

	botUsername, botID := resolveBotIdentity(c)
	message := c.Message()
	for idx := range entities {
		entity := entities[idx]
		switch entity.Type {
		case tele.EntityMention:
			if message == nil || botUsername == "" {
				continue
			}

			entityText := strings.TrimSpace(message.EntityText(entity))
			if strings.EqualFold(strings.TrimPrefix(entityText, "@"), botUsername) {
				return entity, true
			}
		case tele.EntityTMention:
			if entity.User != nil && botID != 0 && entity.User.ID == botID {
				return entity, true
			}
		case tele.EntityHashtag, tele.EntityCashtag, tele.EntityCommand, tele.EntityURL, tele.EntityEmail,
			tele.EntityPhone, tele.EntityBold, tele.EntityItalic, tele.EntityUnderline, tele.EntityStrikethrough,
			tele.EntityCode, tele.EntityCodeBlock, tele.EntityTextLink, tele.EntitySpoiler, tele.EntityCustomEmoji,
			tele.EntityBlockquote, tele.EntityEBlockquote:
			continue
		}
	}

	return tele.MessageEntity{}, false
}

func resolveBotIdentity(c tele.Context) (string, int64) {
	botAPI := c.Bot()
	if botAPI == nil {
		return "", 0
	}

	bot, ok := botAPI.(*tele.Bot)
	if !ok || bot.Me == nil {
		return "", 0
	}

	return strings.TrimSpace(bot.Me.Username), bot.Me.ID
}

func trimLeadingBotMention(c tele.Context, message string, mention tele.MessageEntity) string {
	trimmedMessage := strings.TrimSpace(message)
	currentMessage := c.Message()
	if currentMessage == nil {
		return trimmedMessage
	}

	mentionText := strings.TrimSpace(currentMessage.EntityText(mention))
	if mentionText == "" {
		return trimmedMessage
	}

	if !strings.HasPrefix(trimmedMessage, mentionText) {
		return trimmedMessage
	}

	trimmedMessage = strings.TrimPrefix(trimmedMessage, mentionText)
	trimmedMessage = strings.TrimLeft(trimmedMessage, " \t\r\n,.:;!-")

	return strings.TrimSpace(trimmedMessage)
}

func SetPlaceholderHooks(sender func(c tele.Context, text string) (*tele.Message, error), editor func(c tele.Context, msg tele.Editable, text string) error) func() {
	originalSender := placeholderSender
	originalEditor := placeholderEditor

	if sender != nil {
		placeholderSender = sender
	}

	if editor != nil {
		placeholderEditor = editor
	}

	return func() {
		placeholderSender = originalSender
		placeholderEditor = originalEditor
	}
}

func startTyping(c tele.Context) func() {
	err := c.Notify(tele.Typing)
	if err != nil {
		return func() {}
	}

	stopCh := make(chan struct{})
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				_ = c.Notify(tele.Typing)
			case <-stopCh:
				return
			}
		}
	}()

	return func() {
		close(stopCh)
	}
}

func resolveChatRequestTimeout(cfg config.Config) time.Duration {
	timeout := cfg.ResponsesTimeout
	if timeout <= 0 {
		return defaultChatRequestTimeout
	}

	return timeout
}
