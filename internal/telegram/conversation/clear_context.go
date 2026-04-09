package conversation

import (
	"context"
	"errors"
	"fmt"

	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/config"
	"mangoduck/internal/telegram/chats"
	"mangoduck/internal/telegram/shared"
)

type HistoryClearer interface {
	Clear(ctx context.Context, chatID int64) error
}

func ClearContext(_ config.Config, chatsRepo chats.Repository, historyClearer HistoryClearer) func(tele.Context) error {
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

		err = historyClearer.Clear(context.Background(), c.Chat().ID)
		if err != nil {
			return fmt.Errorf("clearing chat context: %w", err)
		}

		return c.Send("Context cleared.")
	}
}
