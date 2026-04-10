package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	tele "gopkg.in/telebot.v4"
)

func TestScopeForChat_UsesChatScope(t *testing.T) {
	const chatID int64 = 584719545

	scope := scopeForChat(chatID)

	assert.Equal(t, tele.CommandScopeChat, scope.Type)
	assert.Equal(t, chatID, scope.ChatID)
	assert.Zero(t, scope.UserID)
}

func TestScopeForAllPrivateChats_UsesPrivateChatsScope(t *testing.T) {
	scope := scopeForAllPrivateChats()

	assert.Equal(t, tele.CommandScopeAllPrivateChats, scope.Type)
	assert.Zero(t, scope.ChatID)
	assert.Zero(t, scope.UserID)
}

func TestScopeForAllGroupChats_UsesGroupChatsScope(t *testing.T) {
	scope := scopeForAllGroupChats()

	assert.Equal(t, tele.CommandScopeAllGroupChats, scope.Type)
	assert.Zero(t, scope.ChatID)
	assert.Zero(t, scope.UserID)
}

func TestCommandsForAdmin_IncludesChats(t *testing.T) {
	commands := commandsForAdmin()

	assert.Len(t, commands, 3)
	assert.Equal(t, "start", commands[0].Text)
	assert.Equal(t, "clear_context", commands[1].Text)
	assert.Equal(t, "chats", commands[2].Text)
}

func TestBaseCommands_ContainsUserCommands(t *testing.T) {
	commands := baseCommands()

	assert.Len(t, commands, 2)
	assert.Equal(t, "start", commands[0].Text)
	assert.Equal(t, "clear_context", commands[1].Text)
}
