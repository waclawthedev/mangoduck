package tgerr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	tele "gopkg.in/telebot.v4"
)

func TestNormalize_MessageNotModified(t *testing.T) {
	t.Parallel()

	err := Normalize(tele.ErrMessageNotModified)

	require.ErrorIs(t, err, ErrMessageNotModified)
}

func TestNormalize_SameMessageContent(t *testing.T) {
	t.Parallel()

	err := Normalize(tele.ErrSameMessageContent)

	require.ErrorIs(t, err, ErrMessageNotModified)
}

func TestNormalize_EntityParse(t *testing.T) {
	t.Parallel()

	var teleErr tele.Error
	teleErr.Code = 400
	teleErr.Description = "Bad Request: can't parse entities: unsupported start tag"

	err := Normalize(&teleErr)

	require.ErrorIs(t, err, ErrEntityParse)

	var normalized *Error
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, 400, normalized.Code)
}

func TestNormalize_EntityParseFromGenericErrorString(t *testing.T) {
	t.Parallel()

	err := Normalize(errors.New("telegram: Bad Request: can't parse entities: unsupported start tag (400)"))

	require.ErrorIs(t, err, ErrEntityParse)

	var normalized *Error
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, 400, normalized.Code)
}

func TestNormalize_Forbidden(t *testing.T) {
	t.Parallel()

	err := Normalize(tele.ErrBlockedByUser)

	require.ErrorIs(t, err, ErrForbidden)

	var normalized *Error
	require.ErrorAs(t, err, &normalized)
	require.Equal(t, 403, normalized.Code)
}

func TestNormalize_PassesUnknownErrorThrough(t *testing.T) {
	t.Parallel()

	originalErr := errors.New("boom")

	err := Normalize(originalErr)

	require.Same(t, originalErr, err)
}

func TestUserMessage_RedactsSensitiveDetails(t *testing.T) {
	t.Parallel()

	err := errors.New(`provider failed: api_key=sk-secret123 Bearer topsecret token 123456:ABCdefGhijklmnopqrstuvwxyz https://alice:hunter2@example.com/path`)

	message := UserMessage(err)

	require.Contains(t, message, "An error occurred while processing your request.")
	require.Contains(t, message, "[REDACTED]")
	require.NotContains(t, message, "sk-secret123")
	require.NotContains(t, message, "topsecret")
	require.NotContains(t, message, "123456:ABCdefGhijklmnopqrstuvwxyz")
	require.NotContains(t, message, "hunter2")
}

func TestUserMessage_FormatsTelegramRateLimit(t *testing.T) {
	t.Parallel()

	var err Error
	err.Code = 429
	err.Description = "telegram: Too Many Requests (429)"
	err.RetryAfter = 3
	err.Err = ErrRateLimited

	message := UserMessage(&err)

	require.Contains(t, message, "Telegram rate limit reached")
}
