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

func TestCommandsForAdmin_IncludesChats(t *testing.T) {
	commands := commandsForAdmin()

	assert.Len(t, commands, 3)
	assert.Equal(t, "start", commands[0].Text)
	assert.Equal(t, "clear_context", commands[1].Text)
	assert.Equal(t, "chats", commands[2].Text)
}
