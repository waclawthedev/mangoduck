package bot

import (
	"errors"

	tele "gopkg.in/telebot.v4"
)

type CommandSyncer struct {
	bot *tele.Bot
}

func NewCommandSyncer(bot *tele.Bot) *CommandSyncer {
	return &CommandSyncer{bot: bot}
}

func (s *CommandSyncer) SyncDefaults() error {
	return s.bot.SetCommands(baseCommands(), scopeForAllPrivateChats())
}

func (s *CommandSyncer) SyncGroups() error {
	if s == nil || s.bot == nil {
		return errors.New("bot is nil")
	}

	return s.bot.SetCommands(baseCommands(), scopeForAllGroupChats())
}

func (s *CommandSyncer) SyncAdmins(adminTGIDs []int64) error {
	if s == nil || s.bot == nil {
		return errors.New("bot is nil")
	}

	seen := make(map[int64]struct{}, len(adminTGIDs))
	for _, adminTGID := range adminTGIDs {
		if adminTGID == 0 {
			continue
		}
		if _, exists := seen[adminTGID]; exists {
			continue
		}

		seen[adminTGID] = struct{}{}
		err := s.bot.SetCommands(commandsForAdmin(), scopeForChat(adminTGID))
		if err != nil {
			return err
		}
	}

	return nil
}

func commandsForAdmin() []tele.Command {
	commands := baseCommands()

	var chatsCmd tele.Command
	chatsCmd.Text = "chats"
	chatsCmd.Description = "Manage chats"
	commands = append(commands, chatsCmd)

	return commands
}

func baseCommands() []tele.Command {
	commands := make([]tele.Command, 0, 3)

	var startCmd tele.Command
	startCmd.Text = "start"
	startCmd.Description = "Start bot"
	commands = append(commands, startCmd)

	var clearContextCmd tele.Command
	clearContextCmd.Text = "clear_context"
	clearContextCmd.Description = "Clear chat context"
	commands = append(commands, clearContextCmd)

	return commands
}

func scopeForChat(chatID int64) tele.CommandScope {
	var scope tele.CommandScope
	scope.Type = tele.CommandScopeChat
	scope.ChatID = chatID

	return scope
}

func scopeForAllPrivateChats() tele.CommandScope {
	var scope tele.CommandScope
	scope.Type = tele.CommandScopeAllPrivateChats

	return scope
}

func scopeForAllGroupChats() tele.CommandScope {
	var scope tele.CommandScope
	scope.Type = tele.CommandScopeAllGroupChats

	return scope
}
