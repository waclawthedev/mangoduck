package start

import (
	"errors"

	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/config"
	"mangoduck/internal/telegram/chats"
	"mangoduck/internal/telegram/shared"
)

func Start(_ config.Config, chatsRepo chats.Repository) func(tele.Context) error {
	return func(c tele.Context) error {
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

		return c.Send("Hi!")
	}
}
