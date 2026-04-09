package conversation

import (
	"context"
	"errors"
	"testing"
	"time"

	"mangoduck/internal/config"
	handlermocks "mangoduck/internal/handlers/mocks"
	llmchat "mangoduck/internal/llm/chat"
	"mangoduck/internal/repo"

	"github.com/stretchr/testify/require"
	tele "gopkg.in/telebot.v4"
)

type testChatsRepo struct {
	getByTGIDFunc     func(ctx context.Context, tgID int64) (*repo.Chat, error)
	createFunc        func(ctx context.Context, tgID int64, title string, username string, chatType string, status repo.ChatStatus) (*repo.Chat, error)
	updateProfileFunc func(ctx context.Context, tgID int64, title string, username string, chatType string) error
}

func (s *testChatsRepo) GetByTGID(ctx context.Context, tgID int64) (*repo.Chat, error) {
	return s.getByTGIDFunc(ctx, tgID)
}

func (s *testChatsRepo) Create(ctx context.Context, tgID int64, title string, username string, chatType string, status repo.ChatStatus) (*repo.Chat, error) {
	if s.createFunc == nil {
		return nil, errors.New("unexpected create call")
	}

	return s.createFunc(ctx, tgID, title, username, chatType, status)
}

func (s *testChatsRepo) List(ctx context.Context) ([]*repo.Chat, error) {
	return nil, errors.New("unexpected list call")
}

func (s *testChatsRepo) UpdateProfile(ctx context.Context, tgID int64, title string, username string, chatType string) error {
	if s.updateProfileFunc == nil {
		return nil
	}

	return s.updateProfileFunc(ctx, tgID, title, username, chatType)
}

func (s *testChatsRepo) UpdateStatus(ctx context.Context, tgID int64, status repo.ChatStatus) error {
	return errors.New("unexpected update status call")
}

type testResponder struct {
	replyFunc func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error)
}

func (s *testResponder) Reply(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
	return s.replyFunc(ctx, request)
}

type testHistoryClearer struct {
	clearFunc func(ctx context.Context, chatID int64) error
}

func (s *testHistoryClearer) Clear(ctx context.Context, chatID int64) error {
	return s.clearFunc(ctx, chatID)
}

func TestChat_SendsPlaceholderAndEditsWhenToolUsed(t *testing.T) {
	var sentText string
	var editedText string
	restoreHooks := SetPlaceholderHooks(
		func(c tele.Context, text string) (*tele.Message, error) {
			sentText = text
			return &tele.Message{ID: 99}, nil
		},
		func(c tele.Context, msg tele.Editable, text string) error {
			editedText = text
			require.NotNil(t, msg)
			return nil
		},
	)
	t.Cleanup(restoreHooks)

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Hello bot")
	ctx.On("Notify", tele.Typing).Return(nil)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "private"
			chatRecord.Status = repo.ChatStatusActive
			chatRecord.CreatedAt = time.Now()
			return &chatRecord, nil
		},
	}

	var cfg config.Config
	cfg.AdminTGIDs = []int64{42}
	cfg.AdminTGID = 42

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, int64(7), request.ChatID)
			require.Equal(t, int64(42), request.UserTGID)
			require.Equal(t, "Hello bot", request.Message)
			require.True(t, request.IsAdmin)
			err := request.NotifyToolCall("Searching the web for: Hello bot")
			require.NoError(t, err)
			return &llmchat.Result{Text: "Final answer", UsedTool: true, PlaceholderNeeded: true}, nil
		},
	}

	handler := Chat(cfg, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
	require.Equal(t, "Searching the web for: Hello bot", sentText)
	require.Equal(t, "Final answer", editedText)
}

func TestChat_BlocksInactiveChat(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	sender.Username = "member"
	ctx.On("Sender").Return(&sender)
	ctx.On("Text").Return("@mangoduck help")
	ctx.On("Entities").Return(tele.Entities{
		{
			Type:   tele.EntityMention,
			Offset: 0,
			Length: len("@mangoduck"),
		},
	})
	var bot tele.Bot
	bot.Me = &tele.User{ID: 100, Username: "mangoduck"}
	ctx.On("Bot").Return(&bot)
	var message tele.Message
	message.Text = "@mangoduck help"
	ctx.On("Message").Return(&message)
	var currentChat tele.Chat
	currentChat.ID = -1001
	currentChat.Type = "group"
	currentChat.Title = "Mango Duck"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Send", "wait for chat approval\nChat ID: -1001").Return(nil)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "group"
			chatRecord.Title = "Mango Duck"
			chatRecord.Status = repo.ChatStatusInactive
			return &chatRecord, nil
		},
	}

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			t.Fatal("Reply should not be called for inactive chat")
			return nil, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IgnoresGroupMessageWithoutBotMention(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	sender.Username = "member"
	ctx.On("Sender").Return(&sender)
	ctx.On("Text").Return("Hello everyone")
	ctx.On("Chat").Return(&tele.Chat{ID: -1001, Type: "group", Title: "Mango Duck"})
	ctx.On("Entities").Return(tele.Entities(nil))

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			t.Fatal("chat lookup should not happen without bot mention")
			return nil, nil
		},
	}

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			t.Fatal("Reply should not be called without bot mention")
			return nil, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_TrimsLeadingBotMentionInGroupMessage(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	ctx.On("Text").Return("@mangoduck, help me")
	ctx.On("Entities").Return(tele.Entities{
		{
			Type:   tele.EntityMention,
			Offset: 0,
			Length: len("@mangoduck"),
		},
	})
	var bot tele.Bot
	bot.Me = &tele.User{ID: 100, Username: "mangoduck"}
	ctx.On("Bot").Return(&bot)
	var message tele.Message
	message.Text = "@mangoduck, help me"
	ctx.On("Message").Return(&message)
	var currentChat tele.Chat
	currentChat.ID = -1001
	currentChat.Type = "group"
	currentChat.Title = "Mango Duck"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Send", "Hello human", []any{tele.ModeHTML}).Return(nil)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusActive
			return &chatRecord, nil
		},
	}

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "help me", request.Message)
			return &llmchat.Result{Text: "Hello human"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestClearContext_ClearsCurrentChatHistory(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	sender.Username = "member"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 77
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Send", "Context cleared.").Return(nil)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "private"
			chatRecord.Status = repo.ChatStatusActive
			chatRecord.CreatedAt = time.Now()
			return &chatRecord, nil
		},
	}

	clearer := &testHistoryClearer{
		clearFunc: func(ctx context.Context, chatID int64) error {
			require.Equal(t, int64(77), chatID)
			return nil
		},
	}

	handler := ClearContext(config.Config{}, repoStub, clearer)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestClearContext_ReturnsClearError(t *testing.T) {
	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	sender.Username = "member"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 77
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "private"
			chatRecord.Status = repo.ChatStatusActive
			chatRecord.CreatedAt = time.Now()
			return &chatRecord, nil
		},
	}

	clearer := &testHistoryClearer{
		clearFunc: func(ctx context.Context, chatID int64) error {
			return errors.New("boom")
		},
	}

	handler := ClearContext(config.Config{}, repoStub, clearer)
	err := handler(ctx)
	require.EqualError(t, err, "clearing chat context: boom")
}
