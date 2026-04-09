package conversation

import (
	"context"
	"errors"
	"fmt"
	"html"
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
)

func sendReply(c tele.Context, text string) error {
	err := c.Send(text, tele.ModeHTML)
	if err == nil {
		return nil
	}

	if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
		return err
	}

	sanitizedText := tghtml.Sanitize(text)
	if sanitizedText != "" && sanitizedText != text {
		err = c.Send(sanitizedText, tele.ModeHTML)
		if err == nil {
			return nil
		}

		if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
			return err
		}
	}

	escapedText := html.EscapeString(text)
	if escapedText == text {
		return err
	}

	return c.Send(escapedText, tele.ModeHTML)
}

func editPlaceholderReply(c tele.Context, msg tele.Editable, text string) error {
	err := placeholderEditor(c, msg, text)
	if err == nil {
		return nil
	}

	if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
		return err
	}

	sanitizedText := tghtml.Sanitize(text)
	if sanitizedText != "" && sanitizedText != text {
		err = placeholderEditor(c, msg, sanitizedText)
		if err == nil {
			return nil
		}

		if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
			return err
		}
	}

	escapedText := html.EscapeString(text)
	if escapedText == text {
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
		if message == "" {
			return nil
		}

		message, shouldRespond := normalizeIncomingMessage(c, message)
		if !shouldRespond || message == "" {
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

		var placeholderMessage *tele.Message
		var request llmchat.Request
		request.ChatID = c.Chat().ID
		request.UserTGID = sender.ID
		request.Message = message
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

		replyCtx, cancel := context.WithTimeout(context.Background(), resolveChatRequestTimeout(cfg))
		defer cancel()

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
