package bot

import (
	"errors"
	"testing"

	handlermocks "mangoduck/internal/handlers/mocks"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v4"
)

func TestNotifyTelegramHandlerError_SendsSanitizedMessage(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)
	ctx.On("Send", "An error occurred while processing your request.\n<code>provider failed: [REDACTED]</code>", []any{tele.ModeHTML}).Return(nil)

	notifyTelegramHandlerError(zap.NewNop(), ctx, errors.New("provider failed: api_key=sk-secret123"))
}

func TestNotifyTelegramHandlerError_SkipsForbiddenErrors(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)

	notifyTelegramHandlerError(zap.NewNop(), ctx, tele.ErrBlockedByUser)
}
