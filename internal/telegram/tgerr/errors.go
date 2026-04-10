package tgerr

import (
	"errors"
	"fmt"
	"html"
	"regexp"
	"strings"

	tele "gopkg.in/telebot.v4"
)

var (
	ErrMessageNotModified = errors.New("telegram message not modified")
	ErrEntityParse        = errors.New("telegram entity parse")
	ErrForbidden          = errors.New("telegram forbidden")
	ErrRateLimited        = errors.New("telegram rate limited")
)

var sensitiveErrorPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(bearer)\s+[^\s"']+`),
	regexp.MustCompile(`(?i)\b(api[_ -]?key|token|secret|password|authorization|credential)s?\b\s*[:=]\s*("[^"]*"|'[^']*'|[^\s,;]+)`),
	regexp.MustCompile(`(?i)\b(api[_ -]?key|token|secret|password|authorization|credential)s?\b\s+("[^"]*"|'[^']*'|[^\s,;]+)`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]+\b`),
	regexp.MustCompile(`\bpk-[A-Za-z0-9_-]+\b`),
	regexp.MustCompile(`\b\d{6,}:[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?i)(https?://)([^/\s:@]+):([^/\s@]+)@`),
}

type Error struct {
	Code        int
	Description string
	RetryAfter  int
	Err         error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}

	return e.Description
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.Err
}

func Normalize(err error) error {
	if err == nil {
		return nil
	}

	message := strings.TrimSpace(err.Error())
	if isEntityParseDescription(message) {
		var normalized Error
		normalized.Code = 400
		normalized.Description = message
		normalized.Err = ErrEntityParse

		return &normalized
	}

	if errors.Is(err, tele.ErrMessageNotModified) || errors.Is(err, tele.ErrSameMessageContent) {
		return fmt.Errorf("%w: %w", ErrMessageNotModified, err)
	}

	var floodErr tele.FloodError
	if errors.As(err, &floodErr) {
		var normalized Error
		normalized.Code = 429
		normalized.Description = err.Error()
		normalized.RetryAfter = floodErr.RetryAfter
		normalized.Err = ErrRateLimited

		return &normalized
	}

	var teleErr *tele.Error
	if !errors.As(err, &teleErr) {
		return err
	}

	if teleErr.Code == 403 {
		var normalized Error
		normalized.Code = teleErr.Code
		normalized.Description = teleErr.Description
		normalized.Err = ErrForbidden

		return &normalized
	}

	if teleErr.Code == 400 && isEntityParseDescription(teleErr.Description) {
		var normalized Error
		normalized.Code = teleErr.Code
		normalized.Description = teleErr.Description
		normalized.Err = ErrEntityParse

		return &normalized
	}

	return err
}

func isEntityParseDescription(description string) bool {
	return strings.Contains(strings.ToLower(description), "can't parse entities")
}

func UserMessage(err error) string {
	if err == nil {
		return ""
	}

	normalizedErr := Normalize(err)
	switch {
	case errors.Is(normalizedErr, ErrRateLimited):
		return "An error occurred while processing your request.\n<code>Telegram rate limit reached. Please try again in a few seconds.</code>"
	case errors.Is(normalizedErr, ErrEntityParse):
		return "An error occurred while sending the reply.\n<code>Telegram rejected the message formatting.</code>"
	case errors.Is(normalizedErr, ErrForbidden):
		return "An error occurred while sending the reply.\n<code>Telegram rejected delivery to this chat.</code>"
	}

	summary := redactSensitiveDetails(err.Error())
	if summary == "" {
		return "An error occurred while processing your request."
	}

	return fmt.Sprintf(
		"An error occurred while processing your request.\n<code>%s</code>",
		html.EscapeString(summary),
	)
}

func redactSensitiveDetails(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}

	message = strings.Join(strings.Fields(message), " ")
	for _, pattern := range sensitiveErrorPatterns {
		switch pattern.String() {
		case `(?i)(https?://)([^/\s:@]+):([^/\s@]+)@`:
			message = pattern.ReplaceAllString(message, "${1}[REDACTED]@")
		case `(?i)\b(bearer)\s+[^\s"']+`:
			message = pattern.ReplaceAllString(message, "$1 [REDACTED]")
		default:
			message = pattern.ReplaceAllString(message, "[REDACTED]")
		}
	}

	const maxLen = 300
	if len(message) > maxLen {
		message = strings.TrimSpace(message[:maxLen-3]) + "..."
	}

	return message
}
