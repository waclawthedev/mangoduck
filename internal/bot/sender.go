package bot

import (
	"errors"
	"html"

	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/telegram/tgerr"
	"mangoduck/internal/telegram/tghtml"
)

type TelegramSender struct {
	bot *tele.Bot
}

func NewTelegramSender(bot *tele.Bot) *TelegramSender {
	return &TelegramSender{bot: bot}
}

func (s *TelegramSender) Send(chatID int64, text string) error {
	if s == nil || s.bot == nil {
		return errors.New("telegram sender bot is nil")
	}

	recipient := &tele.Chat{ID: chatID}
	normalizedText := tghtml.Normalize(text)
	_, err := s.bot.Send(recipient, normalizedText, tele.ModeHTML)
	if err == nil {
		return nil
	}

	if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
		return err
	}

	sanitizedText := tghtml.Sanitize(normalizedText)
	if sanitizedText != "" && sanitizedText != normalizedText {
		_, err = s.bot.Send(recipient, sanitizedText, tele.ModeHTML)
		if err == nil {
			return nil
		}

		if !errors.Is(tgerr.Normalize(err), tgerr.ErrEntityParse) {
			return err
		}
	}

	escapedText := html.EscapeString(normalizedText)
	if escapedText == normalizedText {
		return err
	}

	_, err = s.bot.Send(recipient, escapedText, tele.ModeHTML)

	return err
}
