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
		photo := currentPhoto(c)
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

func buildLLMMessage(sender *tele.User, currentMessage *tele.Message, message string) string {
	sections := make([]string, 0, 3)

	replyContext := buildReplyContext(currentMessage)
	if replyContext != "" {
		sections = append(sections, replyContext)
	}

	currentSection := buildCurrentMessageContext(sender, currentMessage, message)
	if currentSection != "" {
		sections = append(sections, currentSection)
	}

	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func buildCurrentMessageContext(sender *tele.User, currentMessage *tele.Message, message string) string {
	message = strings.TrimSpace(message)
	senderName := resolveSenderName(sender)

	if isForwardedMessage(currentMessage) {
		forwardOrigin := describeForwardOrigin(currentMessage)
		switch {
		case forwardOrigin != "" && senderName != "" && message != "":
			return "Forwarded message from " + forwardOrigin + ". Shared by " + senderName + ". " + message
		case forwardOrigin != "" && senderName != "":
			return "Forwarded message from " + forwardOrigin + ". Shared by " + senderName + "."
		case forwardOrigin != "" && message != "":
			return "Forwarded message from " + forwardOrigin + ". " + message
		case forwardOrigin != "":
			return "Forwarded message from " + forwardOrigin + "."
		case senderName != "" && message != "":
			return senderName + " here with a forwarded message. " + message
		case senderName != "":
			return senderName + " here with a forwarded message."
		default:
			return message
		}
	}

	if currentMessage != nil && currentMessage.SenderChat != nil {
		senderChatName := resolveChatName(currentMessage.SenderChat)
		switch {
		case senderChatName != "" && senderName != "" && message != "":
			return "Message sent on behalf of " + senderChatName + ". Posted by " + senderName + ". " + message
		case senderChatName != "" && message != "":
			return "Message sent on behalf of " + senderChatName + ". " + message
		case senderChatName != "":
			return "Message sent on behalf of " + senderChatName + "."
		}
	}

	return buildAttributedUserMessage(sender, message)
}

func buildAttributedUserMessage(sender *tele.User, message string) string {
	message = strings.TrimSpace(message)

	senderName := resolveSenderName(sender)
	if senderName == "" {
		return message
	}

	if message == "" {
		return senderName + " here."
	}

	return senderName + " here. " + message
}

func buildReplyContext(message *tele.Message) string {
	if message == nil {
		return ""
	}

	hasReplyReference := message.ReplyTo != nil || message.ExternalReply != nil || message.Quote != nil
	if !hasReplyReference {
		return ""
	}

	originalAuthor, originalText := resolveReplyReference(message)
	quoteText := resolveQuoteText(message)

	switch {
	case originalAuthor != "" && originalText != "" && quoteText != "" && quoteText != originalText:
		return "In reply to " + originalAuthor + ": " + originalText + " Quoted part: " + quoteText
	case originalAuthor != "" && originalText != "":
		return "In reply to " + originalAuthor + ": " + originalText
	case originalAuthor != "" && quoteText != "":
		return "In reply to " + originalAuthor + ". Quoted part: " + quoteText
	case originalAuthor != "":
		return "In reply to " + originalAuthor + "."
	case originalText != "" && quoteText != "" && quoteText != originalText:
		return "In reply to this message: " + originalText + " Quoted part: " + quoteText
	case originalText != "":
		return "In reply to this message: " + originalText
	case quoteText != "":
		return "In reply to a message with quoted text: " + quoteText
	default:
		return "In reply to an earlier message."
	}
}

func resolveReplyReference(message *tele.Message) (string, string) {
	if message == nil {
		return "", ""
	}

	if message.ReplyTo != nil {
		replyTo := message.ReplyTo
		return resolveReplyAuthorName(replyTo), describeMessageContent(replyTo)
	}

	if message.ExternalReply != nil {
		return resolveExternalReplyAuthorName(message.ExternalReply), describeExternalReplyContent(message.ExternalReply)
	}

	return "", ""
}

func resolveSenderName(sender *tele.User) string {
	if sender == nil {
		return ""
	}

	firstName := strings.TrimSpace(sender.FirstName)
	lastName := strings.TrimSpace(sender.LastName)
	fullName := strings.TrimSpace(strings.Join([]string{firstName, lastName}, " "))
	username := strings.TrimSpace(sender.Username)

	switch {
	case fullName != "" && username != "":
		return fullName + " (@" + username + ")"
	case fullName != "":
		return fullName
	case username != "":
		return "@" + username
	case sender.ID != 0:
		return fmt.Sprintf("Telegram user %d", sender.ID)
	default:
		return ""
	}
}

func resolveReplyAuthorName(message *tele.Message) string {
	if message == nil {
		return ""
	}

	if isForwardedMessage(message) {
		origin := describeForwardOrigin(message)
		if origin != "" {
			return origin
		}
	}

	if message.Sender != nil {
		return resolveSenderName(message.Sender)
	}

	return describeSenderChatOrigin(message.SenderChat)
}

func resolveExternalReplyAuthorName(reply *tele.ExternalReply) string {
	if reply == nil || reply.Origin == nil {
		return ""
	}

	origin := reply.Origin
	if origin.Sender != nil {
		return resolveSenderName(origin.Sender)
	}

	senderUsername := strings.TrimSpace(origin.SenderUsername)
	if senderUsername != "" {
		return senderUsername
	}

	if origin.SenderChat != nil {
		return resolveChatName(origin.SenderChat)
	}

	if origin.Chat != nil {
		return resolveChatName(origin.Chat)
	}

	if reply.Chat != nil {
		return resolveChatName(reply.Chat)
	}

	return ""
}

func isForwardedMessage(message *tele.Message) bool {
	if message == nil {
		return false
	}

	if message.IsForwarded() {
		return true
	}

	if strings.TrimSpace(message.OriginalSenderName) != "" {
		return true
	}

	return message.Origin != nil
}

func describeForwardOrigin(message *tele.Message) string {
	if message == nil {
		return ""
	}

	if message.OriginalChat != nil {
		chatName := resolveChatName(message.OriginalChat)
		if chatName == "" {
			return ""
		}

		chatType := strings.TrimSpace(string(message.OriginalChat.Type))
		if chatType == string(tele.ChatChannel) || message.OriginalMessageID != 0 {
			return "channel " + chatName
		}

		return "chat " + chatName
	}

	if message.OriginalSender != nil {
		return resolveSenderName(message.OriginalSender)
	}

	originalSenderName := strings.TrimSpace(message.OriginalSenderName)
	if originalSenderName != "" {
		return originalSenderName
	}

	return describeMessageOrigin(message.Origin)
}

func describeMessageOrigin(origin *tele.MessageOrigin) string {
	if origin == nil {
		return ""
	}

	if origin.Sender != nil {
		return resolveSenderName(origin.Sender)
	}

	senderUsername := strings.TrimSpace(origin.SenderUsername)
	if senderUsername != "" {
		return senderUsername
	}

	if origin.SenderChat != nil {
		return resolveChatName(origin.SenderChat)
	}

	if origin.Chat != nil {
		chatName := resolveChatName(origin.Chat)
		if chatName == "" {
			return ""
		}

		if origin.MessageID != 0 || origin.Type == "channel" {
			return "channel " + chatName
		}

		return "chat " + chatName
	}

	signature := strings.TrimSpace(origin.Signature)
	if signature != "" {
		return signature
	}

	return ""
}

func resolveChatName(chat *tele.Chat) string {
	if chat == nil {
		return ""
	}

	title := strings.TrimSpace(chat.Title)
	firstName := strings.TrimSpace(chat.FirstName)
	lastName := strings.TrimSpace(chat.LastName)
	fullName := strings.TrimSpace(strings.Join([]string{firstName, lastName}, " "))
	username := strings.TrimSpace(chat.Username)

	switch {
	case title != "" && username != "":
		return title + " (@" + username + ")"
	case title != "":
		return title
	case fullName != "" && username != "":
		return fullName + " (@" + username + ")"
	case fullName != "":
		return fullName
	case username != "":
		return "@" + username
	case chat.ID != 0:
		return fmt.Sprintf("chat %d", chat.ID)
	default:
		return ""
	}
}

func describeSenderChatOrigin(chat *tele.Chat) string {
	if chat == nil {
		return ""
	}

	chatName := resolveChatName(chat)
	if chatName == "" {
		return ""
	}

	chatType := strings.TrimSpace(string(chat.Type))
	if chatType == string(tele.ChatChannel) {
		return "channel post from " + chatName
	}

	return "message from chat " + chatName
}

func describeMessageContent(message *tele.Message) string {
	if message == nil {
		return ""
	}

	text := strings.TrimSpace(message.Text)
	if text != "" {
		return text
	}

	caption := strings.TrimSpace(message.Caption)
	if caption != "" {
		return caption
	}

	switch {
	case message.Photo != nil:
		return "[photo]"
	case message.Document != nil:
		return "[document]"
	case message.Audio != nil:
		return "[audio]"
	case message.Voice != nil:
		return "[voice message]"
	case message.Video != nil:
		return "[video]"
	case message.Sticker != nil:
		return "[sticker]"
	default:
		return ""
	}
}

func describeExternalReplyContent(reply *tele.ExternalReply) string {
	if reply == nil {
		return ""
	}

	switch {
	case len(reply.Photo) > 0:
		return "[photo]"
	case reply.Document != nil:
		return "[document]"
	case reply.Audio != nil:
		return "[audio]"
	case reply.Voice != nil:
		return "[voice message]"
	case reply.Video != nil:
		return "[video]"
	case reply.Sticker != nil:
		return "[sticker]"
	case reply.Animation != nil:
		return "[animation]"
	case reply.Contact != nil:
		return "[contact]"
	case reply.Dice != nil:
		return "[dice]"
	case reply.Game != nil:
		return "[game]"
	case reply.Venue != nil:
		return "[venue]"
	case reply.Poll != nil:
		return "[poll]"
	case reply.Location != nil:
		return "[location]"
	case reply.Invoice != nil:
		return "[invoice]"
	case reply.Story != nil:
		return "[story]"
	case reply.Note != nil:
		return "[video note]"
	default:
		return ""
	}
}

func resolveQuoteText(message *tele.Message) string {
	if message == nil || message.Quote == nil {
		return ""
	}

	return strings.TrimSpace(message.Quote.Text)
}

func currentPhoto(c tele.Context) *tele.Photo {
	message := c.Message()
	if message == nil {
		return nil
	}

	return message.Photo
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

	botMention, ok := findBotMentionEntity(c)
	if !ok {
		return "", false
	}

	return trimLeadingBotMention(c, message, botMention), true
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
