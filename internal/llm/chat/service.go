package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/searchx"
	"mangoduck/internal/llm/websearch"
	"mangoduck/internal/logging"
	"mangoduck/internal/repo"
)

const (
	DefaultModel                = "gpt-5-mini"
	DefaultNoResponseText       = "I don't have a response yet."
	defaultMaxSteps             = 4
	xSearchFunctionToolName     = "x-search"
	webSearchFunctionToolName   = "web-search"
	memoryGetFunctionToolName   = "memory-get"
	memorySetFunctionToolName   = "memory-set"
	listCronTasksToolName       = "list-cron-tasks"
	addCronTaskFunctionToolName = "add-cron-task"
	deleteCronTaskToolName      = "delete-cron-task"
	telegramHTMLPrompt          = "You are replying for Telegram in agent mode. Return only Telegram-compatible HTML. Use only tags supported by Telegram HTML parse mode. Do not use unsupported block tags such as <p>, <div>, <span>, <ul>, <ol>, or <li>; use plain text line breaks instead. Escape plain text so it is valid HTML, but when you intentionally use Telegram formatting tags such as <b>, <i>, <u>, <s>, <code>, <pre>, <blockquote>, <a>, <tg-spoiler>, <tg-emoji>, or <tg-time>, emit them as real tags and do not escape them as text. Do not use Markdown. Work step by step: call at most one function in each assistant turn, then analyze the function result before deciding what to do next. If more information is needed, make another single function call on the next turn. Produce the final user-facing answer only when the collected tool results are sufficient."
	searchQueryRequiredToolText = "Search tool input rejected: the query is empty. Do not call a search tool again yet. Ask the user one short clarifying question to collect the missing topic, entity, or keywords, then wait for their reply."
	cronTaskHTMLInstruction     = "Return the final user-facing answer only as Telegram-compatible HTML. Use only tags supported by Telegram HTML parse mode. Do not use Markdown or unsupported block tags such as <p>, <div>, <span>, <ul>, <ol>, or <li>."
)

var (
	ErrMissingMessage             = errors.New("chat message is required")
	ErrMissingSearchExecutor      = errors.New("chat X search executor is required")
	ErrMissingWebSearchExecutor   = errors.New("chat web search executor is required")
	ErrMissingHistoryStore        = errors.New("chat history store is required")
	ErrMissingCronTaskStore       = errors.New("chat cron task store is required")
	ErrMissingCronTaskManager     = errors.New("chat cron task manager is required")
	ErrTooManyFunctionCallsInStep = errors.New("chat response contains more than one function call in a step")
)

type XSearchExecutor interface {
	Search(ctx context.Context, request *searchx.SearchRequest) (*searchx.SearchResult, error)
}

type WebSearchExecutor interface {
	Search(ctx context.Context, request *websearch.SearchRequest) (*websearch.SearchResult, error)
}

type HistoryStore interface {
	Append(ctx context.Context, chatID int64, items []json.RawMessage) error
	List(ctx context.Context, chatID int64) ([]json.RawMessage, error)
}

type MemoryStore interface {
	GetMemory(ctx context.Context, chatID int64) (string, error)
	SetMemory(ctx context.Context, chatID int64, memoryText string) error
}

type CronTaskStore interface {
	Create(ctx context.Context, chatID int64, createdByTGID int64, schedule string, prompt string) (*repo.CronTask, error)
	GetByID(ctx context.Context, id int64) (*repo.CronTask, error)
	ListByChatID(ctx context.Context, chatID int64) ([]*repo.CronTask, error)
	DeleteByID(ctx context.Context, id int64) (*repo.CronTask, error)
}

type CronTaskManager interface {
	AddTask(task *repo.CronTask) error
	RemoveTask(taskID int64)
}

type ToolCallNotifier func(statusText string) error

type ToolRuntimeFactory interface {
	OpenSession(ctx context.Context) (ToolRuntime, error)
}

type ToolRuntime interface {
	Tools() []*responses.Tool
	Execute(ctx context.Context, name string, arguments string) (string, bool, error)
	Close() error
}

type Option func(*Service)

type InputImage struct {
	MIMEType   string
	DataBase64 string
}

type Request struct {
	ChatID         int64
	UserTGID       int64
	Message        string
	Image          *InputImage
	IsAdmin        bool
	NotifyToolCall ToolCallNotifier
}

type Result struct {
	Text              string
	UsedTool          bool
	PlaceholderNeeded bool
}

type executionRequest struct {
	ChatID          int64
	UserTGID        int64
	Message         string
	Image           *InputImage
	IsAdmin         bool
	PersistHistory  bool
	EnableCronTools bool
	IsScheduled     bool
	NotifyToolCall  ToolCallNotifier
}

type Service struct {
	client           responses.Client
	xSearcher        XSearchExecutor
	webSearcher      WebSearchExecutor
	historyStore     HistoryStore
	memoryStore      MemoryStore
	cronTaskStore    CronTaskStore
	cronTaskManager  CronTaskManager
	toolRuntime      ToolRuntimeFactory
	model            string
	maxSteps         int
	xSearchEnabled   bool
	webSearchEnabled bool
	logger           *zap.Logger
}

func NewService(
	client responses.Client,
	xSearcher XSearchExecutor,
	webSearcher WebSearchExecutor,
	historyStore HistoryStore,
	cronTaskStore CronTaskStore,
	cronTaskManager CronTaskManager,
	model string,
	options ...Option,
) (*Service, error) {
	if client == nil {
		return nil, errors.New("chat client is required")
	}

	model = strings.TrimSpace(model)
	if model == "" {
		model = DefaultModel
	}

	var service Service
	service.client = client
	service.xSearcher = xSearcher
	service.webSearcher = webSearcher
	service.historyStore = historyStore
	service.memoryStore = noopMemoryStore{}
	service.cronTaskStore = cronTaskStore
	service.cronTaskManager = cronTaskManager
	service.model = model
	service.maxSteps = defaultMaxSteps
	service.xSearchEnabled = true
	service.webSearchEnabled = true
	service.toolRuntime = noopToolRuntimeFactory{}
	service.logger = zap.NewNop()

	for _, option := range options {
		if option == nil {
			continue
		}
		option(&service)
	}

	if service.xSearchEnabled && xSearcher == nil {
		return nil, ErrMissingSearchExecutor
	}

	if service.webSearchEnabled && webSearcher == nil {
		return nil, ErrMissingWebSearchExecutor
	}

	if historyStore == nil {
		return nil, ErrMissingHistoryStore
	}

	if cronTaskStore == nil {
		return nil, ErrMissingCronTaskStore
	}

	if cronTaskManager == nil {
		return nil, ErrMissingCronTaskManager
	}

	return &service, nil
}

func WithToolRuntimeFactory(factory ToolRuntimeFactory) Option {
	return func(service *Service) {
		if service == nil || factory == nil {
			return
		}

		service.toolRuntime = factory
	}
}

func WithMemoryStore(store MemoryStore) Option {
	return func(service *Service) {
		if service == nil || store == nil {
			return
		}

		service.memoryStore = store
	}
}

func WithLogger(logger *zap.Logger) Option {
	return func(service *Service) {
		if service == nil {
			return
		}

		service.logger = logging.WithComponent(logger, "chat")
	}
}

func WithXSearchEnabled(enabled bool) Option {
	return func(service *Service) {
		if service == nil {
			return
		}

		service.xSearchEnabled = enabled
	}
}

func WithWebSearchEnabled(enabled bool) Option {
	return func(service *Service) {
		if service == nil {
			return
		}

		service.webSearchEnabled = enabled
	}
}

func (s *Service) Reply(ctx context.Context, request *Request) (*Result, error) {
	if request == nil {
		return nil, ErrMissingMessage
	}

	var execution executionRequest
	execution.ChatID = request.ChatID
	execution.UserTGID = request.UserTGID
	execution.Message = request.Message
	execution.Image = cloneInputImage(request.Image)
	execution.IsAdmin = request.IsAdmin
	execution.PersistHistory = true
	execution.EnableCronTools = true
	execution.NotifyToolCall = request.NotifyToolCall

	return s.run(ctx, &execution)
}

func (s *Service) ExecuteScheduled(ctx context.Context, chatID int64, prompt string) (string, error) {
	var request executionRequest
	request.ChatID = chatID
	request.Message = prompt
	request.PersistHistory = false
	request.EnableCronTools = false
	request.IsScheduled = true

	result, err := s.run(ctx, &request)
	if err != nil {
		return "", err
	}
	if result == nil {
		return "", nil
	}

	return strings.TrimSpace(result.Text), nil
}

func (s *Service) run(ctx context.Context, request *executionRequest) (*Result, error) {
	if request == nil {
		return nil, ErrMissingMessage
	}

	message := strings.TrimSpace(request.Message)
	if message == "" && !hasInputImage(request.Image) {
		return nil, ErrMissingMessage
	}

	s.logger.Debug("chat execution started", zap.Int64("chat_id", request.ChatID), zap.Int64("user_tg_id", request.UserTGID), zap.Bool("persist_history", request.PersistHistory), zap.Bool("enable_cron_tools", request.EnableCronTools), zap.Bool("is_scheduled", request.IsScheduled), zap.Int("message_len", len(message)))

	var history []json.RawMessage
	var err error
	if request.PersistHistory {
		history, err = s.historyStore.List(ctx, request.ChatID)
		if err != nil {
			return nil, fmt.Errorf("listing chat history: %w", err)
		}
	}

	memoryText, err := s.memoryStore.GetMemory(ctx, request.ChatID)
	if errors.Is(err, repo.ErrChatNotFound) {
		memoryText = ""
	} else if err != nil {
		return nil, fmt.Errorf("getting chat memory: %w", err)
	}

	userItem, err := buildUserMessageItem(message, request.Image)
	if err != nil {
		return nil, err
	}

	systemItem, err := buildSystemMessageItem(buildSystemPrompt(request.IsAdmin, request.EnableCronTools, request.IsScheduled, memoryText))
	if err != nil {
		return nil, err
	}

	input := append([]json.RawMessage{systemItem}, cloneRawMessages(history)...)
	input = append(input, userItem)

	toolRuntime, err := s.toolRuntime.OpenSession(ctx)
	if err != nil {
		s.logger.Error("failed to open tool runtime", zap.Error(err))
		return nil, fmt.Errorf("open tool runtime: %w", err)
	}
	defer func() {
		if toolRuntime != nil {
			_ = toolRuntime.Close()
		}
	}()

	tools := buildTools(request.EnableCronTools, s.xSearchEnabled, s.webSearchEnabled, toolRuntime)

	var (
		usedTool          bool
		placeholderNeeded bool
		userItemStored    bool
		storeDisabled     = false
		parallelToolCalls = false
	)

	for step := range s.maxSteps {
		s.logger.Debug("chat step started", zap.Int64("chat_id", request.ChatID), zap.Int("step", step+1), zap.Int("input_items", len(input)), zap.Int("tools", len(tools)))
		response, err := s.client.CreateResponse(ctx, &responses.CreateResponseRequest{
			Model:             s.model,
			Input:             input,
			Store:             &storeDisabled,
			ParallelToolCalls: &parallelToolCalls,
			Tools:             tools,
			ToolChoice:        "auto",
		})
		if err != nil {
			s.logger.Error("chat response request failed", zap.Int64("chat_id", request.ChatID), zap.Int("step", step+1), zap.Error(err))
			return nil, err
		}

		if request.PersistHistory && !userItemStored {
			err = s.historyStore.Append(ctx, request.ChatID, []json.RawMessage{userItem})
			if err != nil {
				return nil, fmt.Errorf("appending user item to chat history: %w", err)
			}

			userItemStored = true
		}

		historyItems, err := buildHistoryItems(response)
		if err != nil {
			return nil, err
		}

		if len(historyItems) > 0 {
			if request.PersistHistory {
				err = s.historyStore.Append(ctx, request.ChatID, historyItems)
				if err != nil {
					return nil, fmt.Errorf("appending response items to chat history: %w", err)
				}
			}

			input = append(input, cloneRawMessages(historyItems)...)
		}

		functionCalls := response.FunctionCalls()
		if len(functionCalls) > 1 {
			s.logger.Error("chat response returned too many function calls", zap.Int64("chat_id", request.ChatID), zap.Int("step", step+1), zap.Int("function_calls", len(functionCalls)))
			return nil, ErrTooManyFunctionCallsInStep
		}

		if len(functionCalls) == 0 {
			text := strings.TrimSpace(response.OutputText())
			if text == "" && !usedTool {
				text = DefaultNoResponseText
			}

			s.logger.Debug("chat execution finished without further tool calls", zap.Int64("chat_id", request.ChatID), zap.Int("step", step+1), zap.Bool("used_tool", usedTool), zap.Int("text_len", len(text)))

			return &Result{
				Text:              text,
				UsedTool:          usedTool,
				PlaceholderNeeded: placeholderNeeded,
			}, nil
		}

		call := functionCalls[0]
		if call == nil {
			s.logger.Error("chat response returned nil function call", zap.Int64("chat_id", request.ChatID), zap.Int("step", step+1))
			return nil, errors.New("chat function call is nil")
		}

		if request.NotifyToolCall != nil {
			err := request.NotifyToolCall(buildToolCallStatusText(call))
			if err != nil {
				return nil, fmt.Errorf("notifying chat tool call: %w", err)
			}
		}

		placeholderNeeded = true
		usedTool = true
		s.logger.Debug("executing chat tool call", zap.Int64("chat_id", request.ChatID), zap.Int("step", step+1), zap.String("tool_name", strings.TrimSpace(call.Name)), zap.String("call_id", strings.TrimSpace(call.CallID)))

		toolResultText, err := s.executeToolCall(ctx, call, request, toolRuntime)
		if err != nil {
			s.logger.Error("chat tool call failed", zap.Int64("chat_id", request.ChatID), zap.String("tool_name", strings.TrimSpace(call.Name)), zap.String("call_id", strings.TrimSpace(call.CallID)), zap.Error(err))
			return nil, err
		}
		s.logger.Debug("chat tool call finished", zap.Int64("chat_id", request.ChatID), zap.String("tool_name", strings.TrimSpace(call.Name)), zap.String("call_id", strings.TrimSpace(call.CallID)), zap.Int("output_len", len(strings.TrimSpace(toolResultText))))

		functionCallOutput, err := buildFunctionCallOutputItem(strings.TrimSpace(call.CallID), strings.TrimSpace(toolResultText))
		if err != nil {
			return nil, err
		}

		if request.PersistHistory {
			err = s.historyStore.Append(ctx, request.ChatID, []json.RawMessage{functionCallOutput})
			if err != nil {
				return nil, fmt.Errorf("appending function call output to chat history: %w", err)
			}
		}

		input = append(input, functionCallOutput)
	}

	text := ""
	if usedTool {
		text = DefaultNoResponseText
	}

	s.logger.Debug("chat execution reached max steps", zap.Int64("chat_id", request.ChatID), zap.Int("max_steps", s.maxSteps), zap.Bool("used_tool", usedTool), zap.Int("text_len", len(text)))

	return &Result{
		Text:              text,
		UsedTool:          usedTool,
		PlaceholderNeeded: placeholderNeeded,
	}, nil
}

func hasInputImage(image *InputImage) bool {
	return image != nil && strings.TrimSpace(image.MIMEType) != "" && strings.TrimSpace(image.DataBase64) != ""
}

func cloneInputImage(image *InputImage) *InputImage {
	if image == nil {
		return nil
	}

	var cloned InputImage
	cloned.MIMEType = strings.TrimSpace(image.MIMEType)
	cloned.DataBase64 = strings.TrimSpace(image.DataBase64)

	if cloned.MIMEType == "" || cloned.DataBase64 == "" {
		return nil
	}

	return &cloned
}

func (s *Service) executeToolCall(ctx context.Context, call *responses.FunctionCall, request *executionRequest, runtime ToolRuntime) (string, error) {
	if call == nil {
		return "", errors.New("chat function call is nil")
	}

	switch strings.TrimSpace(call.Name) {
	case xSearchFunctionToolName:
		searchRequest, err := parseXSearchRequest(call.Arguments)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(searchRequest.Query) == "" {
			return searchQueryRequiredToolText, nil
		}

		searchResult, err := s.xSearcher.Search(ctx, searchRequest)
		if err != nil {
			return "", err
		}

		return strings.TrimSpace(searchResult.Text), nil
	case webSearchFunctionToolName:
		searchRequest, err := parseWebSearchRequest(call.Arguments)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(searchRequest.Query) == "" {
			return searchQueryRequiredToolText, nil
		}

		searchResult, err := s.webSearcher.Search(ctx, searchRequest)
		if err != nil {
			return "", err
		}

		return strings.TrimSpace(searchResult.Text), nil
	case memoryGetFunctionToolName:
		return s.getMemory(ctx, request)
	case memorySetFunctionToolName:
		return s.setMemory(ctx, request, call.Arguments)
	case listCronTasksToolName:
		return s.listCronTasks(ctx, request)
	case addCronTaskFunctionToolName:
		return s.addCronTask(ctx, request, call.Arguments)
	case deleteCronTaskToolName:
		return s.deleteCronTask(ctx, request, call.Arguments)
	default:
		if runtime != nil {
			output, handled, err := runtime.Execute(ctx, call.Name, call.Arguments)
			if handled || err != nil {
				return output, err
			}
		}

		return "", fmt.Errorf("chat received unknown function call: %s", call.Name)
	}
}

func (s *Service) getMemory(ctx context.Context, request *executionRequest) (string, error) {
	memoryText, err := s.memoryStore.GetMemory(ctx, request.ChatID)
	if errors.Is(err, repo.ErrChatNotFound) {
		return "Memory is empty.", nil
	}
	if err != nil {
		return "", fmt.Errorf("getting chat memory: %w", err)
	}

	memoryText = strings.TrimSpace(memoryText)
	if memoryText == "" {
		return "Memory is empty.", nil
	}

	return "Current memory:\n" + memoryText, nil
}

func (s *Service) setMemory(ctx context.Context, request *executionRequest, arguments string) (string, error) {
	payload, err := parseSetMemoryArguments(arguments)
	if err != nil {
		return "", err
	}

	err = s.memoryStore.SetMemory(ctx, request.ChatID, payload.Text)
	if err != nil {
		return "", fmt.Errorf("setting chat memory: %w", err)
	}

	if payload.Text == "" {
		return "Memory cleared.", nil
	}

	return "Memory updated.\nCurrent memory:\n" + payload.Text, nil
}

func (s *Service) addCronTask(ctx context.Context, request *executionRequest, arguments string) (string, error) {
	payload, parseErr := parseAddCronTaskArguments(arguments)
	if parseErr != nil {
		return "", parseErr
	}

	if !isValidCronExpression(payload.Schedule) {
		return fmt.Sprintf("Cannot create cron task: invalid cron expression %q.", payload.Schedule), nil
	}

	task, err := s.cronTaskStore.Create(ctx, request.ChatID, request.UserTGID, payload.Schedule, payload.Prompt)
	if err != nil {
		return "", fmt.Errorf("creating cron task: %w", err)
	}

	err = s.cronTaskManager.AddTask(task)
	if err != nil {
		_, cleanupErr := s.cronTaskStore.DeleteByID(ctx, task.ID)
		if cleanupErr != nil {
			return "", fmt.Errorf("registering cron task: %w", errors.Join(err, fmt.Errorf("cleanup failed: %w", cleanupErr)))
		}

		return "", fmt.Errorf("registering cron task: %w", err)
	}

	s.logger.Debug("cron task created from chat tool", zap.Int64("chat_id", task.ChatID), zap.Int64("task_id", task.ID), zap.String("schedule", task.Schedule), zap.Int("prompt_len", len(task.Prompt)))

	return fmt.Sprintf(
		"Cron task created.\nTask ID: %d\nSchedule: %s\nTimezone: %s\nPrompt: %s",
		task.ID,
		task.Schedule,
		time.Now().Location().String(),
		task.Prompt,
	), nil
}

func (s *Service) listCronTasks(ctx context.Context, request *executionRequest) (string, error) {
	tasks, err := s.cronTaskStore.ListByChatID(ctx, request.ChatID)
	if err != nil {
		return "", fmt.Errorf("listing cron tasks: %w", err)
	}

	if len(tasks) == 0 {
		return "No cron tasks found for this chat.", nil
	}

	var builder strings.Builder
	builder.WriteString("Cron tasks for this chat:")
	for _, task := range tasks {
		if task == nil {
			continue
		}

		builder.WriteString("\n\n")
		_, _ = fmt.Fprintf(&builder, "Task ID: %d", task.ID)
		builder.WriteString("\nSchedule: ")
		builder.WriteString(strings.TrimSpace(task.Schedule))
		builder.WriteString("\nPrompt: ")
		builder.WriteString(strings.TrimSpace(task.Prompt))
	}

	return builder.String(), nil
}

func (s *Service) deleteCronTask(ctx context.Context, request *executionRequest, arguments string) (string, error) {
	taskID, err := parseDeleteCronTaskArguments(arguments)
	if err != nil {
		return "", err
	}

	task, err := s.cronTaskStore.GetByID(ctx, taskID)
	if errors.Is(err, repo.ErrCronTaskNotFound) {
		return fmt.Sprintf("Cron task %d was not found.", taskID), nil
	}
	if err != nil {
		return "", fmt.Errorf("getting cron task: %w", err)
	}

	if task.ChatID != request.ChatID {
		return fmt.Sprintf("Cron task %d does not belong to this chat and cannot be managed here.", taskID), nil
	}

	task, err = s.cronTaskStore.DeleteByID(ctx, taskID)
	if errors.Is(err, repo.ErrCronTaskNotFound) {
		return fmt.Sprintf("Cron task %d was not found.", taskID), nil
	}
	if err != nil {
		return "", fmt.Errorf("deleting cron task: %w", err)
	}

	s.cronTaskManager.RemoveTask(task.ID)
	s.logger.Debug("cron task deleted from chat tool", zap.Int64("chat_id", task.ChatID), zap.Int64("task_id", task.ID), zap.String("schedule", task.Schedule))

	return fmt.Sprintf(
		"Cron task deleted.\nTask ID: %d\nSchedule: %s\nPrompt: %s",
		task.ID,
		task.Schedule,
		task.Prompt,
	), nil
}

func isValidCronExpression(schedule string) bool {
	_, err := cron.ParseStandard(schedule)
	return err == nil
}

type noopToolRuntimeFactory struct{}

type noopToolRuntime struct{}

type noopMemoryStore struct{}

func (noopToolRuntimeFactory) OpenSession(context.Context) (ToolRuntime, error) {
	return noopToolRuntime{}, nil
}

func (noopToolRuntime) Tools() []*responses.Tool {
	return nil
}

func (noopToolRuntime) Execute(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

func (noopToolRuntime) Close() error {
	return nil
}

func (noopMemoryStore) GetMemory(context.Context, int64) (string, error) {
	return "", nil
}

func (noopMemoryStore) SetMemory(context.Context, int64, string) error {
	return nil
}
