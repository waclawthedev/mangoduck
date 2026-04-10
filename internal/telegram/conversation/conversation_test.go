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
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

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
	ctx.On("Message").Return(&tele.Message{Text: "Hello bot"})
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
			require.Equal(t, "@boss here. Hello bot", request.Message)
			require.True(t, request.IsAdmin)
			err := request.NotifyToolCall("Searching the web for: @boss here. Hello bot")
			require.NoError(t, err)
			return &llmchat.Result{Text: "Final answer", UsedTool: true, PlaceholderNeeded: true}, nil
		},
	}

	handler := Chat(cfg, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
	require.Equal(t, "Searching the web for: @boss here. Hello bot", sentText)
	require.Equal(t, "Final answer", editedText)
}

func TestChat_NormalizesEscapedTelegramHTMLInPlaceholderEdit(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	var editedText string
	restoreHooks := SetPlaceholderHooks(
		func(c tele.Context, text string) (*tele.Message, error) {
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
	ctx.On("Message").Return(&tele.Message{Text: "Hello bot"})
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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			err := request.NotifyToolCall("Running a tool...")
			require.NoError(t, err)
			return &llmchat.Result{Text: `Done. &lt;b&gt;file.txt&lt;/b&gt;`, UsedTool: true, PlaceholderNeeded: true}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
	require.Equal(t, "Done. <b>file.txt</b>", editedText)
}

func TestChat_NormalizesEscapedTelegramHTMLInDirectReply(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

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
	ctx.On("Send", "Done. <b>file.txt</b>", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			return &llmchat.Result{Text: `Done. &lt;b&gt;file.txt&lt;/b&gt;`}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_BlocksInactiveChat(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

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
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 7
	sender.Username = "member"
	ctx.On("Sender").Return(&sender)
	ctx.On("Text").Return("Hello everyone")
	ctx.On("Message").Return(&tele.Message{Text: "Hello everyone"})
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

func TestChat_AcceptsDirectReplyToBotWithoutMentionInGroup(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	ctx.On("Text").Return("help me with this")
	var bot tele.Bot
	bot.Me = &tele.User{ID: 100, Username: "mangoduck", IsBot: true}
	ctx.On("Bot").Return(&bot)
	ctx.On("Message").Return(&tele.Message{
		Text: "help me with this",
		ReplyTo: &tele.Message{
			Text:   "Original bot answer",
			Sender: &tele.User{ID: 100, Username: "mangoduck", IsBot: true},
		},
	})
	var currentChat tele.Chat
	currentChat.ID = -1001
	currentChat.Type = "group"
	currentChat.Title = "Mango Duck"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Send", "Sure", []any{tele.ModeHTML}).Return(nil)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusActive
			chatRecord.CreatedAt = time.Now()
			return &chatRecord, nil
		},
	}

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "In reply to @mangoduck: Original bot answer\n\n@boss here. help me with this", request.Message)
			return &llmchat.Result{Text: "Sure"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_TrimsLeadingBotMentionInGroupMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

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
			require.Equal(t, "@boss here. help me", request.Message)
			return &llmchat.Result{Text: "Hello human"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IncludesReplyContextInRequestMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("What did they mean?")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "What did they mean?",
		ReplyTo: &tele.Message{
			Text: "Ship it today",
			Sender: &tele.User{
				ID:        77,
				FirstName: "Alice",
				Username:  "alice",
			},
		},
	})
	ctx.On("Send", "I can help", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "In reply to Alice (@alice): Ship it today\n\n@boss here. What did they mean?", request.Message)
			return &llmchat.Result{Text: "I can help"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IncludesForwardContextInRequestMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Release is live")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "Release is live",
		OriginalSender: &tele.User{
			ID:        77,
			FirstName: "Alice",
			Username:  "alice",
		},
	})
	ctx.On("Send", "Noted", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "Forwarded message from Alice (@alice). Shared by @boss. Release is live", request.Message)
			return &llmchat.Result{Text: "Noted"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IncludesForwardedChannelPostContextInRequestMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Breaking: rollout paused")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "Breaking: rollout paused",
		OriginalChat: &tele.Chat{
			ID:       -100777,
			Type:     tele.ChatChannel,
			Title:    "Deploy News",
			Username: "deploynews",
		},
		OriginalMessageID: 91,
	})
	ctx.On("Send", "Captured", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "Forwarded message from channel Deploy News (@deploynews). Shared by @boss. Breaking: rollout paused", request.Message)
			return &llmchat.Result{Text: "Captured"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_UsesForwardOriginWhenReplyingToForwardedMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Is this accurate?")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "Is this accurate?",
		ReplyTo: &tele.Message{
			Text: "The outage is resolved",
			OriginalSender: &tele.User{
				ID:        77,
				FirstName: "Alice",
				Username:  "alice",
			},
		},
	})
	ctx.On("Send", "Checking", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "In reply to Alice (@alice): The outage is resolved\n\n@boss here. Is this accurate?", request.Message)
			return &llmchat.Result{Text: "Checking"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IncludesSenderChatReplyContextInRequestMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Does this need action?")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "Does this need action?",
		ReplyTo: &tele.Message{
			Text: "Maintenance starts in 10 minutes",
			SenderChat: &tele.Chat{
				ID:       -100222,
				Type:     tele.ChatChannel,
				Title:    "Ops Alerts",
				Username: "opsalerts",
			},
		},
	})
	ctx.On("Send", "Looking", []any{tele.ModeHTML}).Return(nil)

	repoStub := &testChatsRepo{
		getByTGIDFunc: func(ctx context.Context, tgID int64) (*repo.Chat, error) {
			var chatRecord repo.Chat
			chatRecord.TGID = tgID
			chatRecord.Type = "group"
			chatRecord.Status = repo.ChatStatusActive
			chatRecord.CreatedAt = time.Now()
			return &chatRecord, nil
		},
	}

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "In reply to channel post from Ops Alerts (@opsalerts): Maintenance starts in 10 minutes\n\n@boss here. Does this need action?", request.Message)
			return &llmchat.Result{Text: "Looking"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IncludesQuotedReplyContextInRequestMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Can you interpret this?")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "Can you interpret this?",
		ReplyTo: &tele.Message{
			Text: "Ship it today, but only after QA signs off.",
			Sender: &tele.User{
				ID:        77,
				FirstName: "Alice",
				Username:  "alice",
			},
		},
		Quote: &tele.TextQuote{
			Text: "only after QA signs off",
		},
	})
	ctx.On("Send", "Sure", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "In reply to Alice (@alice): Ship it today, but only after QA signs off. Quoted part: only after QA signs off\n\n@boss here. Can you interpret this?", request.Message)
			return &llmchat.Result{Text: "Sure"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_IncludesExternalReplyContextInRequestMessage(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = buildRequestImage
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	sender.Username = "boss"
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("Can you summarize it?")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Text: "Can you summarize it?",
		ExternalReply: &tele.ExternalReply{
			Origin: &tele.MessageOrigin{
				Sender: &tele.User{
					ID:        77,
					FirstName: "Alice",
					Username:  "alice",
				},
			},
			Document: &tele.Document{},
		},
		Quote: &tele.TextQuote{
			Text: "budget-v3.pdf",
		},
	})
	ctx.On("Send", "On it", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "In reply to Alice (@alice): [document] Quoted part: budget-v3.pdf\n\n@boss here. Can you summarize it?", request.Message)
			return &llmchat.Result{Text: "On it"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_BuildsPhotoCaptionRequest(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = func(ctx context.Context, c tele.Context, photo *tele.Photo) (*llmchat.InputImage, error) {
		require.NotNil(t, photo)

		var image llmchat.InputImage
		image.MIMEType = "image/jpeg"
		image.DataBase64 = "ZmFrZQ=="
		return &image, nil
	}
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("look at this")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Caption: "look at this",
		Photo:   &tele.Photo{},
	})
	ctx.On("Send", "Looks good", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "Telegram user 42 here. look at this", request.Message)
			require.NotNil(t, request.Image)
			require.Equal(t, "image/jpeg", request.Image.MIMEType)
			require.Equal(t, "ZmFrZQ==", request.Image.DataBase64)
			return &llmchat.Result{Text: "Looks good"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_BuildsPhotoOnlyRequest(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = func(ctx context.Context, c tele.Context, photo *tele.Photo) (*llmchat.InputImage, error) {
		require.NotNil(t, photo)

		var image llmchat.InputImage
		image.MIMEType = "image/png"
		image.DataBase64 = "aW1hZ2U="
		return &image, nil
	}
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Photo: &tele.Photo{},
	})
	ctx.On("Send", "Image received", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "Telegram user 42 here.", request.Message)
			require.NotNil(t, request.Image)
			require.Equal(t, "image/png", request.Image.MIMEType)
			return &llmchat.Result{Text: "Image received"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_AppliesTimeoutBeforeBuildingPhotoRequest(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = func(ctx context.Context, c tele.Context, photo *tele.Photo) (*llmchat.InputImage, error) {
		require.NotNil(t, photo)

		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.WithinDuration(t, time.Now().Add(20*time.Millisecond), deadline, 20*time.Millisecond)

		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Photo: &tele.Photo{},
	})

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			t.Fatal("Reply should not be called after photo builder timeout")
			return nil, nil
		},
	}

	var cfg config.Config
	cfg.ResponsesTimeout = 20 * time.Millisecond

	handler := Chat(cfg, repoStub, responder)
	err := handler(ctx)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.EqualError(t, err, "reading telegram photo: context deadline exceeded")
}

func TestChat_FallsBackToCaptionWhenPhotoBuildFails(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = func(ctx context.Context, c tele.Context, photo *tele.Photo) (*llmchat.InputImage, error) {
		require.NotNil(t, photo)
		return nil, errors.New("boom")
	}
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("look at this")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Caption: "look at this",
		Photo:   &tele.Photo{},
	})
	ctx.On("Send", "Caption handled", []any{tele.ModeHTML}).Return(nil)

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			require.Equal(t, "Telegram user 42 here. look at this", request.Message)
			require.Nil(t, request.Image)
			return &llmchat.Result{Text: "Caption handled"}, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.NoError(t, err)
}

func TestChat_FailsPhotoOnlyRequestWhenPhotoBuildFails(t *testing.T) {
	originalImageBuilder := requestImageBuilder
	requestImageBuilder = func(ctx context.Context, c tele.Context, photo *tele.Photo) (*llmchat.InputImage, error) {
		require.NotNil(t, photo)
		return nil, errors.New("boom")
	}
	t.Cleanup(func() {
		requestImageBuilder = originalImageBuilder
	})

	ctx := handlermocks.NewMockContext(t)
	var sender tele.User
	sender.ID = 42
	ctx.On("Sender").Return(&sender)
	var currentChat tele.Chat
	currentChat.ID = 7
	currentChat.Type = "private"
	ctx.On("Chat").Return(&currentChat)
	ctx.On("Text").Return("")
	ctx.On("Notify", tele.Typing).Return(nil)
	ctx.On("Message").Return(&tele.Message{
		Photo: &tele.Photo{},
	})

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

	responder := &testResponder{
		replyFunc: func(ctx context.Context, request *llmchat.Request) (*llmchat.Result, error) {
			t.Fatal("Reply should not be called for image-only photo build failure")
			return nil, nil
		},
	}

	handler := Chat(config.Config{}, repoStub, responder)
	err := handler(ctx)
	require.EqualError(t, err, "reading telegram photo: boom")
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

func TestBuildCurrentMessageContext_DetectsForwardedMessageByHiddenSenderName(t *testing.T) {
	var sender tele.User
	sender.ID = 42
	sender.Username = "forwarder"

	var currentMessage tele.Message
	currentMessage.OriginalSenderName = "Hidden Sender"

	result := buildCurrentMessageContext(&sender, &currentMessage, "Look at this")
	require.Equal(t, "Forwarded message from Hidden Sender. Shared by @forwarder. Look at this", result)
}

func TestResolveReplyAuthorName_PrefersForwardOriginOverForwarder(t *testing.T) {
	var forwarder tele.User
	forwarder.ID = 42
	forwarder.Username = "forwarder"

	var originSender tele.User
	originSender.ID = 99
	originSender.Username = "origin"

	var message tele.Message
	message.Sender = &forwarder
	message.Origin = &tele.MessageOrigin{Sender: &originSender}

	result := resolveReplyAuthorName(&message)
	require.Equal(t, "@origin", result)
}
