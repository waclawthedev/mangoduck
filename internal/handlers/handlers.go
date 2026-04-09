package handlers

import (
	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/config"
	"mangoduck/internal/telegram/chats"
	"mangoduck/internal/telegram/conversation"
	"mangoduck/internal/telegram/start"
)

type (
	ChatsRepository      = chats.Repository
	ChatApprovalNotifier = chats.ApprovalNotifier
	ChatResponder        = conversation.Responder
)

func Start(cfg config.Config, chatsRepo ChatsRepository) func(tele.Context) error {
	return start.Start(cfg, chatsRepo)
}

func Chats(cfg config.Config, chatsRepo ChatsRepository) func(tele.Context) error {
	return chats.Chats(cfg, chatsRepo)
}

func ToggleChatStatus(cfg config.Config, chatsRepo ChatsRepository, approvalNotifier ChatApprovalNotifier) func(tele.Context) error {
	return chats.ToggleChatStatus(cfg, chatsRepo, approvalNotifier)
}

func Chat(cfg config.Config, chatsRepo ChatsRepository, responder ChatResponder) func(tele.Context) error {
	return conversation.Chat(cfg, chatsRepo, responder)
}
