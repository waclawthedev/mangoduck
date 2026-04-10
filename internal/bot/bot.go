package bot

import (
	"context"
	"database/sql"
	"strings"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v4"

	"mangoduck/internal/config"
	"mangoduck/internal/cronjobs"
	"mangoduck/internal/llm/chat"
	"mangoduck/internal/llm/responses"
	openairesponses "mangoduck/internal/llm/responses/openai"
	portkeyresponses "mangoduck/internal/llm/responses/portkey"
	xairesponses "mangoduck/internal/llm/responses/xai"
	"mangoduck/internal/llm/searchx"
	"mangoduck/internal/llm/websearch"
	"mangoduck/internal/logging"
	"mangoduck/internal/mcpbridge"
	"mangoduck/internal/repo"
	"mangoduck/internal/telegram/chats"
	"mangoduck/internal/telegram/conversation"
	"mangoduck/internal/telegram/start"
)

type Runtime struct {
	bot               *tele.Bot
	scheduler         *cronjobs.Service
	schedulerStopFunc context.CancelFunc
	logger            *zap.Logger
}

type toolRuntimeFactoryAdapter struct {
	bridge *mcpbridge.Bridge
}

func New(cfg config.Config, db *sql.DB, logger *zap.Logger) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	logger = logging.WithComponent(logger, "bot")

	b, err := tele.NewBot(tele.Settings{
		Token: cfg.TelegramToken,
		Poller: &tele.LongPoller{
			Timeout: cfg.PollTimeout,
		},
		OnError: func(err error, c tele.Context) {
			handlerLogger := logger.Named("telegram")
			if c == nil {
				handlerLogger.Error("telegram handler failed", zap.Error(err))
				return
			}

			var chatID int64
			chat := c.Chat()
			if chat != nil {
				chatID = chat.ID
			}

			var senderID int64
			sender := c.Sender()
			if sender != nil {
				senderID = sender.ID
			}

			handlerLogger.Error(
				"telegram handler failed",
				zap.Error(err),
				zap.Int64("chat_id", chatID),
				zap.Int64("sender_id", senderID),
				zap.String("text", strings.TrimSpace(c.Text())),
			)

			notifyTelegramHandlerError(handlerLogger, c, err)
		},
	})
	if err != nil {
		return nil, err
	}

	commandSyncer := NewCommandSyncer(b)
	approvalNotifier := NewChatApprovalNotifier(b)
	responsesClient, err := portkeyresponses.NewClient(portkeyresponses.Config{
		Provider:       cfg.PortkeyProvider,
		ProviderAPIKey: cfg.PortkeyProviderAPIKey,
		BaseURL:        cfg.PortkeyBaseURL,
		Timeout:        cfg.ResponsesTimeout,
	})
	if err != nil {
		return nil, err
	}

	var openAIWebSearchClient responses.Client
	var webSearchService *websearch.Service
	if cfg.OpenAIWebSearchEnabled() {
		openAIWebSearchClient, err = openairesponses.NewClient(openairesponses.Config{
			APIKey:  cfg.OpenAIWebSearchAPIKey,
			BaseURL: cfg.OpenAIWebSearchBaseURL,
			Timeout: cfg.OpenAIWebSearchTimeout,
		})
		if err != nil {
			return nil, err
		}

		webSearchService, err = websearch.NewService(openAIWebSearchClient, cfg.OpenAIWebSearchModel, websearch.WithLogger(logger))
		if err != nil {
			return nil, err
		}
	}

	var xaiClient responses.Client
	var xSearchService *searchx.Service
	if cfg.XSearchEnabled() {
		xaiClient, err = xairesponses.NewClient(xairesponses.Config{
			APIKey:  cfg.XAIAPIKey,
			BaseURL: cfg.XAIBaseURL,
			Timeout: cfg.XAITimeout,
		}, xairesponses.WithLogger(logger))
		if err != nil {
			return nil, err
		}

		xSearchService, err = searchx.NewService(xaiClient, cfg.XAIModel, searchx.WithLogger(logger))
		if err != nil {
			return nil, err
		}
	}

	chatsRepo := repo.NewChatsRepo(db)
	inputsOutputsRepo := repo.NewInputsOutputsRepo(db)
	cronTasksRepo := repo.NewCronTasksRepo(db)

	toolBridge, err := mcpbridge.New(cfg.MCP)
	if err != nil {
		return nil, err
	}

	scheduler, err := cronjobs.NewService(cronTasksRepo, nil, NewTelegramSender(b), cronjobs.WithLogger(logger))
	if err != nil {
		return nil, err
	}

	err = runStartupPreflight(context.Background(), cfg, logger, responsesClient, xaiClient, openAIWebSearchClient, toolBridge)
	if err != nil {
		return nil, combineStartupError("startup preflight failed", err)
	}

	chatService, err := chat.NewService(
		responsesClient,
		xSearchService,
		webSearchService,
		inputsOutputsRepo,
		cronTasksRepo,
		scheduler,
		cfg.MainModel,
		chat.WithLogger(logger),
		chat.WithXSearchEnabled(cfg.XSearchEnabled()),
		chat.WithWebSearchEnabled(cfg.OpenAIWebSearchEnabled()),
		chat.WithMemoryStore(chatsRepo),
		chat.WithToolRuntimeFactory(toolRuntimeFactoryAdapter{bridge: toolBridge}),
	)
	if err != nil {
		return nil, err
	}
	scheduler.SetExecutor(chatService)

	err = commandSyncer.SyncDefaults()
	if err != nil {
		return nil, err
	}

	err = commandSyncer.SyncGroups()
	if err != nil {
		return nil, err
	}

	err = commandSyncer.SyncAdmins(cfg.AdminTGIDs)
	if err != nil {
		return nil, err
	}

	registerHandlers(b, cfg, chatsRepo, db, chatService, approvalNotifier)

	var runtime Runtime
	runtime.bot = b
	runtime.scheduler = scheduler
	runtime.logger = logger

	return &runtime, nil
}

func (r *Runtime) Start() {
	if r == nil || r.bot == nil {
		return
	}

	if r.scheduler != nil {
		schedulerCtx, cancel := context.WithCancel(context.Background())
		r.schedulerStopFunc = cancel
		r.logger.Debug("starting scheduler")
		err := r.scheduler.Start(schedulerCtx)
		if err != nil {
			cancel()
			r.logger.Error("scheduler start failed", zap.Error(err))
		}
	}

	r.logger.Debug("starting telegram bot loop")
	r.bot.Start()
	r.logger.Debug("telegram bot loop stopped")
}

func (r *Runtime) Stop() {
	if r == nil {
		return
	}

	r.logger.Debug("stopping runtime")
	stopRuntime(r.schedulerStopFunc, func() {
		if r.bot != nil {
			r.bot.Stop()
		}
	})
	r.logger.Debug("runtime stopped")
}

func stopRuntime(stopScheduler context.CancelFunc, stopBot func()) {
	if stopScheduler != nil {
		stopScheduler()
	}

	if stopBot != nil {
		stopBot()
	}
}

func (a toolRuntimeFactoryAdapter) OpenSession(ctx context.Context) (chat.ToolRuntime, error) {
	if a.bridge == nil {
		return nil, nil
	}

	return a.bridge.OpenSession(ctx)
}

func registerHandlers(b *tele.Bot, cfg config.Config, chatsRepo *repo.ChatsRepo, db *sql.DB, chatResponder conversation.Responder, approvalNotifier *ChatApprovalNotifier) {
	toggleChatStatusButton := (&tele.ReplyMarkup{}).Data("", chats.ToggleChatStatusButtonUnique())
	chatsPageButton := (&tele.ReplyMarkup{}).Data("", chats.ChatsPageButtonUnique())
	chatHandler := conversation.Chat(cfg, chatsRepo, chatResponder)

	b.Handle("/start", start.Start(cfg, chatsRepo))
	b.Handle("/clear_context", conversation.ClearContext(cfg, chatsRepo, repo.NewInputsOutputsRepo(db)))
	b.Handle("/chats", chats.Chats(cfg, chatsRepo))
	b.Handle(&toggleChatStatusButton, chats.ToggleChatStatus(cfg, chatsRepo, approvalNotifier))
	b.Handle(&chatsPageButton, chats.ChatsPage(cfg, chatsRepo))
	b.Handle(tele.OnText, chatHandler)
	b.Handle(tele.OnPhoto, chatHandler)
}
