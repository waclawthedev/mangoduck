package bot

import (
	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/repo"
)

type ChatApprovalNotifier struct {
	bot *tele.Bot
}

func NewChatApprovalNotifier(bot *tele.Bot) *ChatApprovalNotifier {
	return &ChatApprovalNotifier{bot: bot}
}

func (n *ChatApprovalNotifier) NotifyApproved(chatRecord *repo.Chat) error {
	var recipient tele.Chat
	recipient.ID = chatRecord.TGID
	recipient.Type = tele.ChatType(chatRecord.Type)

	_, err := n.bot.Send(&recipient, "Welcome! Your access has been approved.")
	return err
}
