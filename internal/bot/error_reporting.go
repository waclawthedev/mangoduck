package bot

import (
	"errors"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/telegram/tgerr"
)

func notifyTelegramHandlerError(logger *zap.Logger, c tele.Context, err error) {
	if logger == nil || c == nil || err == nil {
		return
	}

	if errors.Is(tgerr.Normalize(err), tgerr.ErrForbidden) {
		return
	}

	message := tgerr.UserMessage(err)
	if message == "" {
		return
	}

	sendErr := c.Send(message, tele.ModeHTML)
	if sendErr == nil {
		return
	}

	logger.Warn("failed to send sanitized telegram error", zap.Error(sendErr))
}
