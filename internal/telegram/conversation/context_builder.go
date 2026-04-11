package conversation

import (
	"fmt"
	"strings"

	tele "gopkg.in/telebot.v4"
)

type telegramContext struct {
	senderName    string
	replyAuthor   string
	replyText     string
	quoteText     string
	messageOrigin string
	forwardOrigin string
	senderChat    string
}

func buildLLMMessage(sender *tele.User, currentMessage *tele.Message, message string) string {
	message = strings.TrimSpace(message)

	context := buildTelegramContext(sender, currentMessage)

	var builder strings.Builder
	if context.hasData() {
		builder.WriteString("[telegram-context]\n")
		builder.WriteString(context.render())
		builder.WriteString("\n[/telegram-context]")
	}

	if message != "" || builder.Len() > 0 {
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString("[user-message]\n")
		builder.WriteString(message)
		builder.WriteString("\n[/user-message]")
	}

	return strings.TrimSpace(builder.String())
}

func buildTelegramContext(sender *tele.User, message *tele.Message) telegramContext {
	var context telegramContext
	context.senderName = resolveSenderName(sender)
	context.replyAuthor, context.replyText = resolveReplyReference(message)
	context.quoteText = resolveQuoteText(message)
	context.messageOrigin, context.forwardOrigin, context.senderChat = resolveCurrentMessageOrigin(message)
	return context
}

func (c telegramContext) hasData() bool {
	return c.senderName != "" ||
		c.replyAuthor != "" ||
		c.replyText != "" ||
		c.quoteText != "" ||
		c.messageOrigin != "" ||
		c.forwardOrigin != "" ||
		c.senderChat != ""
}

func (c telegramContext) render() string {
	lines := make([]string, 0, 7)

	if c.senderName != "" {
		lines = append(lines, "sender: "+c.senderName)
	}
	if c.replyAuthor != "" {
		lines = append(lines, "reply_to_author: "+c.replyAuthor)
	}
	if c.replyText != "" {
		lines = append(lines, "reply_to_text: "+c.replyText)
	}
	if c.quoteText != "" {
		lines = append(lines, "quote_text: "+c.quoteText)
	}
	if c.messageOrigin != "" {
		lines = append(lines, "message_origin: "+c.messageOrigin)
	}
	if c.forwardOrigin != "" {
		lines = append(lines, "forward_origin: "+c.forwardOrigin)
	}
	if c.senderChat != "" {
		lines = append(lines, "sender_chat: "+c.senderChat)
	}

	return strings.Join(lines, "\n")
}

func resolveCurrentMessageOrigin(message *tele.Message) (string, string, string) {
	if isForwardedMessage(message) {
		return "forwarded", describeForwardOrigin(message), ""
	}

	if message != nil && message.SenderChat != nil {
		return "sender_chat", "", resolveChatName(message.SenderChat)
	}

	return "direct", "", ""
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
