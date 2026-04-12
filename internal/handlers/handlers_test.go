package handlers_test

import (
	"context"
	"testing"
	"time"

	"mangoduck/internal/config"
	"mangoduck/internal/handlers"
	handlermocks "mangoduck/internal/handlers/mocks"
	"mangoduck/internal/llm/chat"
	"mangoduck/internal/repo"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	tele "gopkg.in/telebot.v4"
)

const (
	testModelReply = "Hello human"
	testGroupTitle = "Mango Duck"
)

type chatsRepoStub struct {
	getByTGIDFunc     func(ctx context.Context, tgID int64) (*repo.Chat, error)
	listFunc          func(ctx context.Context) ([]*repo.Chat, error)
	createFunc        func(ctx context.Context, tgID int64, title string, username string, chatType string, status repo.ChatStatus) (*repo.Chat, error)
	updateProfileFunc func(ctx context.Context, tgID int64, title string, username string, chatType string) error
	updateStatusFunc  func(ctx context.Context, tgID int64, status repo.ChatStatus) error
}

func (s *chatsRepoStub) Create(ctx context.Context, tgID int64, title string, username string, chatType string, status repo.ChatStatus) (*repo.Chat, error) {
	if s.createFunc == nil {
		return nil, repo.ErrChatNotFound
	}

	return s.createFunc(ctx, tgID, title, username, chatType, status)
}

func (s *chatsRepoStub) GetByTGID(ctx context.Context, tgID int64) (*repo.Chat, error) {
	if s.getByTGIDFunc == nil {
		return nil, repo.ErrChatNotFound
	}

	return s.getByTGIDFunc(ctx, tgID)
}

func (s *chatsRepoStub) List(ctx context.Context) ([]*repo.Chat, error) {
	if s.listFunc == nil {
		return nil, nil
	}

	return s.listFunc(ctx)
}

func (s *chatsRepoStub) UpdateProfile(ctx context.Context, tgID int64, title string, username string, chatType string) error {
	if s.updateProfileFunc == nil {
		return nil
	}

	return s.updateProfileFunc(ctx, tgID, title, username, chatType)
}

func (s *chatsRepoStub) UpdateStatus(ctx context.Context, tgID int64, status repo.ChatStatus) error {
	if s.updateStatusFunc == nil {
		return nil
	}

	return s.updateStatusFunc(ctx, tgID, status)
}

type chatResponderStub struct {
	replyFunc func(ctx context.Context, request *chat.Request) (*chat.Result, error)
}

type approvalNotifierStub struct {
	notifyApprovedFunc func(chatRecord *repo.Chat) error
}

func (s *approvalNotifierStub) NotifyApproved(chatRecord *repo.Chat) error {
	if s.notifyApprovedFunc == nil {
		return nil
	}

	return s.notifyApprovedFunc(chatRecord)
}

func (s *chatResponderStub) Reply(ctx context.Context, request *chat.Request) (*chat.Result, error) {
	return s.replyFunc(ctx, request)
}

func TestStart_CreatesInactiveChatAndRequestsApproval(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	sender.Username = "member"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Send", "wait for chat approval\nChat ID: 7").Return(nil)

	repoStub := &chatsRepoStub{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			assert.Equal(t, int64(7), tgID)
			return nil, repo.ErrChatNotFound
		},
		createFunc: func(ctx context.Context, tgID int64, title string, username string, chatType string, status repo.ChatStatus) (*repo.Chat, error) {
			assert.Equal(t, int64(7), tgID)
			assert.Empty(t, title)
			assert.Equal(t, "member", username)
			assert.Equal(t, "private", chatType)
			assert.Equal(t, repo.ChatStatusInactive, status)

			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = chatType
			chatRecord.Username = username
			chatRecord.Status = status
			chatRecord.CreatedAt = time.Now()
			return &chatRecord, nil
		},
	}

	handler := handlers.Start(config.Config{}, repoStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestStart_SendsHiForActiveChat(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Send", "Hi!").Return(nil)

	repoStub := &chatsRepoStub{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "private"
			chatRecord.Status = repo.ChatStatusActive
			return &chatRecord, nil
		},
	}

	handler := handlers.Start(config.Config{}, repoStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestChat_SendsModelReplyForActiveChat(t *testing.T) {
	t.Parallel()

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
	ctx.On("Message").Return(&tele.Message{Text: "Hello bot"})
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Send", testModelReply, []any{tele.ModeHTML}).Return(nil)

	repoStub := &chatsRepoStub{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "private"
			chatRecord.Status = repo.ChatStatusActive
			return &chatRecord, nil
		},
	}

	responderStub := &chatResponderStub{
		replyFunc: func(ctx context.Context, request *chat.Request) (*chat.Result, error) {
			assert.Equal(t, int64(7), request.ChatID)
			assert.Equal(t, int64(42), request.UserTGID)
			assert.Equal(t, "[telegram-context]\nsender: @boss\nmessage_origin: direct\n[/telegram-context]\n\n[user-message]\nHello bot\n[/user-message]", request.Message)
			assert.True(t, request.IsAdmin)
			deadline, ok := ctx.Deadline()
			assert.True(t, ok)
			assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, time.Second)
			return &chat.Result{Text: testModelReply}, nil
		},
	}

	var cfg config.Config
	cfg.AdminTGIDs = []int64{42}
	cfg.AdminTGID = 42
	cfg.ResponsesTimeout = 5 * time.Second

	handler := handlers.Chat(cfg, repoStub, responderStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestChat_BlocksInactiveChat(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
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
	currentChat.Title = testGroupTitle
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Send", "wait for chat approval\nChat ID: -1001").Return(nil)

	repoStub := &chatsRepoStub{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "group"
			chatRecord.Title = testGroupTitle
			chatRecord.Status = repo.ChatStatusInactive
			return &chatRecord, nil
		},
	}

	responderStub := &chatResponderStub{
		replyFunc: func(ctx context.Context, request *chat.Request) (*chat.Result, error) {
			t.Fatal("Reply should not be called for inactive chat")
			return nil, nil
		},
	}

	handler := handlers.Chat(config.Config{}, repoStub, responderStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestChat_AllowsDirectReplyToBotWithoutMentionInGroup(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	ctx.On("Text").Return("help me")
	var bot tele.Bot
	bot.Me = &tele.User{ID: 100, Username: "mangoduck", IsBot: true}
	ctx.On("Bot").Return(&bot)
	ctx.On("Message").Return(&tele.Message{
		Text: "help me",
		ReplyTo: &tele.Message{
			Text:   "Original bot answer",
			Sender: &tele.User{ID: 100, Username: "mangoduck", IsBot: true},
		},
	})
	var currentChat tele.Chat
	currentChat.ID = -1001
	currentChat.Type = "group"
	currentChat.Title = testGroupTitle
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Send", testModelReply, []any{tele.ModeHTML}).Return(nil)

	repoStub := &chatsRepoStub{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusActive
			return &chatRecord, nil
		},
	}

	responderStub := &chatResponderStub{
		replyFunc: func(ctx context.Context, request *chat.Request) (*chat.Result, error) {
			assert.Equal(t, "[telegram-context]\nsender: @boss\nreply_to_author: @mangoduck\nreply_to_text: Original bot answer\nmessage_origin: direct\n[/telegram-context]\n\n[user-message]\nhelp me\n[/user-message]", request.Message)
			return &chat.Result{Text: testModelReply}, nil
		},
	}

	handler := handlers.Chat(config.Config{}, repoStub, responderStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestChats_SendsListForConfiguredAdmin(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	ctx.On("Chat").Return(&tele.Chat{ID: 42, Type: "private"})
	ctx.On("Send", mock.AnythingOfType("string"), mock.Anything).Return(nil)

	repoStub := &chatsRepoStub{
		listFunc: func(ctx context.Context) ([]*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = -1001
			chatRecord.Title = testGroupTitle
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusInactive
			return []*repo.Chat{&chatRecord}, nil
		},
	}

	var cfg config.Config
	cfg.AdminTGIDs = []int64{42}
	cfg.AdminTGID = 42

	handler := handlers.Chats(cfg, repoStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestChats_BlocksAdminPanelOutsidePrivateChat(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	ctx.On("Chat").Return(&tele.Chat{ID: -1001, Type: "group", Title: testGroupTitle})
	ctx.On("Send", "Open this admin panel in a private chat with the bot.").Return(nil)

	repoStub := &chatsRepoStub{
		listFunc: func(ctx context.Context) ([]*repo.Chat, error) {
			t.Fatal("List should not be called outside private chats")
			return nil, nil
		},
	}

	var cfg config.Config
	cfg.AdminTGIDs = []int64{42}
	cfg.AdminTGID = 42

	handler := handlers.Chats(cfg, repoStub)
	err := handler(ctx)

	assert.NoError(t, err)
}

func TestToggleChatStatus_UpdatesStatusAndRefreshesMessage(t *testing.T) {
	t.Parallel()

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	ctx.On("Chat").Return(&tele.Chat{ID: 42, Type: "private"})
	ctx.On("Args").Return([]string{"-1001", "active", "1"})
	ctx.On("Edit", mock.AnythingOfType("string"), mock.Anything).Return(nil)
	ctx.On("Respond").Return(nil)

	repoStub := &chatsRepoStub{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Title = testGroupTitle
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusInactive
			return &chatRecord, nil
		},
		listFunc: func(ctx context.Context) ([]*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = -1001
			chatRecord.Title = testGroupTitle
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusActive
			return []*repo.Chat{&chatRecord}, nil
		},
		updateStatusFunc: func(ctx context.Context, tgID int64, status repo.ChatStatus) error {
			assert.Equal(t, int64(-1001), tgID)
			assert.Equal(t, repo.ChatStatusActive, status)
			return nil
		},
	}

	notifierStub := &approvalNotifierStub{
		notifyApprovedFunc: func(chatRecord *repo.Chat) error {
			assert.Equal(t, int64(-1001), chatRecord.TGID)
			assert.Equal(t, repo.ChatStatusActive, chatRecord.Status)
			assert.Equal(t, "group", chatRecord.Type)
			return nil
		},
	}

	var cfg config.Config
	cfg.AdminTGIDs = []int64{42}
	cfg.AdminTGID = 42

	handler := handlers.ToggleChatStatus(cfg, repoStub, notifierStub)
	err := handler(ctx)

	assert.NoError(t, err)
}
