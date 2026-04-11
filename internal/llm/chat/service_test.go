package chat_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"mangoduck/internal/llm/chat"
	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/searchx"
	"mangoduck/internal/llm/websearch"
	"mangoduck/internal/repo"

	"github.com/stretchr/testify/require"
)

type stubClient struct {
	requests []*responses.CreateResponseRequest
	replies  []*responses.Response
	err      error
}

func (c *stubClient) CreateResponse(_ context.Context, request *responses.CreateResponseRequest) (*responses.Response, error) {
	c.requests = append(c.requests, request)
	if c.err != nil {
		return nil, c.err
	}

	if len(c.replies) == 0 {
		return &responses.Response{}, nil
	}

	reply := c.replies[0]
	c.replies = c.replies[1:]

	return reply, nil
}

type stubXSearchExecutor struct {
	requests []*searchx.SearchRequest
	result   *searchx.SearchResult
	err      error
}

func (s *stubXSearchExecutor) Search(_ context.Context, request *searchx.SearchRequest) (*searchx.SearchResult, error) {
	s.requests = append(s.requests, request)
	if s.err != nil {
		return nil, s.err
	}

	if s.result != nil {
		return s.result, nil
	}

	return &searchx.SearchResult{}, nil
}

type stubWebSearchExecutor struct {
	requests []*websearch.SearchRequest
	result   *websearch.SearchResult
	err      error
}

func (s *stubWebSearchExecutor) Search(_ context.Context, request *websearch.SearchRequest) (*websearch.SearchResult, error) {
	s.requests = append(s.requests, request)
	if s.err != nil {
		return nil, s.err
	}

	if s.result != nil {
		return s.result, nil
	}

	return &websearch.SearchResult{}, nil
}

type stubHistoryStore struct {
	listItems   []json.RawMessage
	appendCalls [][]json.RawMessage
	listErr     error
	appendErr   error
}

func (s *stubHistoryStore) Append(_ context.Context, _ int64, items []json.RawMessage) error {
	if s.appendErr != nil {
		return s.appendErr
	}

	s.appendCalls = append(s.appendCalls, cloneRaw(items))
	s.listItems = append(s.listItems, cloneRaw(items)...)
	return nil
}

func (s *stubHistoryStore) List(_ context.Context, _ int64) ([]json.RawMessage, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	return cloneRaw(s.listItems), nil
}

type stubMemoryStore struct {
	getCalls []int64
	setCalls []stubMemorySetCall
	memory   map[int64]string
	getErr   error
	setErr   error
}

type stubMemorySetCall struct {
	chatID int64
	text   string
}

func (s *stubMemoryStore) GetMemory(_ context.Context, chatID int64) (string, error) {
	s.getCalls = append(s.getCalls, chatID)
	if s.getErr != nil {
		return "", s.getErr
	}
	if s.memory == nil {
		return "", nil
	}

	return s.memory[chatID], nil
}

func (s *stubMemoryStore) SetMemory(_ context.Context, chatID int64, memoryText string) error {
	s.setCalls = append(s.setCalls, stubMemorySetCall{chatID: chatID, text: memoryText})
	if s.setErr != nil {
		return s.setErr
	}
	if s.memory == nil {
		s.memory = make(map[int64]string)
	}

	s.memory[chatID] = memoryText

	return nil
}

type stubCronTaskStore struct {
	createCalls  []*repo.CronTask
	listCalls    []int64
	getCalls     []int64
	deleteCalls  []int64
	createResult *repo.CronTask
	listResult   []*repo.CronTask
	getResult    *repo.CronTask
	deleteResult *repo.CronTask
	createErr    error
	listErr      error
	getErr       error
	deleteErr    error
}

func (s *stubCronTaskStore) Create(_ context.Context, chatID int64, createdByTGID int64, schedule string, prompt string) (*repo.CronTask, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}

	var task repo.CronTask
	if s.createResult != nil {
		task = *s.createResult
	}
	task.ChatID = chatID
	task.CreatedByTGID = createdByTGID
	task.Schedule = schedule
	task.Prompt = prompt
	if task.ID == 0 {
		task.ID = int64(len(s.createCalls) + 1)
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = time.Unix(0, 0)
	}

	s.createCalls = append(s.createCalls, &task)

	return &task, nil
}

func (s *stubCronTaskStore) ListByChatID(_ context.Context, chatID int64) ([]*repo.CronTask, error) {
	s.listCalls = append(s.listCalls, chatID)
	if s.listErr != nil {
		return nil, s.listErr
	}

	if s.listResult != nil {
		return s.listResult, nil
	}

	return nil, nil
}

func (s *stubCronTaskStore) GetByID(_ context.Context, id int64) (*repo.CronTask, error) {
	s.getCalls = append(s.getCalls, id)
	if s.getErr != nil {
		return nil, s.getErr
	}

	if s.getResult != nil {
		return s.getResult, nil
	}

	var task repo.CronTask
	task.ID = id

	return &task, nil
}

func (s *stubCronTaskStore) DeleteByID(_ context.Context, id int64) (*repo.CronTask, error) {
	s.deleteCalls = append(s.deleteCalls, id)
	if s.deleteErr != nil {
		return nil, s.deleteErr
	}

	if s.deleteResult != nil {
		return s.deleteResult, nil
	}

	var task repo.CronTask
	task.ID = id

	return &task, nil
}

type stubCronTaskManager struct {
	addCalls    []*repo.CronTask
	removeCalls []int64
	addErr      error
}

func (s *stubCronTaskManager) AddTask(task *repo.CronTask) error {
	if s.addErr != nil {
		return s.addErr
	}

	s.addCalls = append(s.addCalls, task)
	return nil
}

func (s *stubCronTaskManager) RemoveTask(taskID int64) {
	s.removeCalls = append(s.removeCalls, taskID)
}

type stubToolRuntimeFactory struct {
	runtime chat.ToolRuntime
	err     error
	opens   int
}

func (f *stubToolRuntimeFactory) OpenSession(_ context.Context) (chat.ToolRuntime, error) {
	f.opens++
	if f.err != nil {
		return nil, f.err
	}

	if f.runtime != nil {
		return f.runtime, nil
	}

	return &stubToolRuntime{}, nil
}

type stubToolRuntime struct {
	tools      []*responses.Tool
	executions []stubToolExecution
	result     string
	handled    bool
	err        error
	closeCalls int
}

type stubToolExecution struct {
	name      string
	arguments string
}

func newTestService(
	deps chat.Dependencies,
	options ...chat.Option,
) (*chat.Service, error) {
	return chat.NewService(deps, options...)
}

func (r *stubToolRuntime) Tools() []*responses.Tool {
	return r.tools
}

func (r *stubToolRuntime) Execute(_ context.Context, name string, arguments string) (string, bool, error) {
	r.executions = append(r.executions, stubToolExecution{name: name, arguments: arguments})
	return r.result, r.handled, r.err
}

func (r *stubToolRuntime) Close() error {
	r.closeCalls++
	return nil
}

func TestNewServiceUsesDefaultModel(t *testing.T) {
	var client stubClient
	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "I don't have a response yet.", reply.Text)
	require.Len(t, client.requests, 1)
	require.Equal(t, chat.DefaultModel, client.requests[0].Model)
	require.False(t, *client.requests[0].Store)
	require.False(t, *client.requests[0].ParallelToolCalls)
	require.Len(t, client.requests[0].Tools, 7)
	require.Equal(t, "function", client.requests[0].Tools[0].Type)
	require.Equal(t, "x-search", client.requests[0].Tools[0].Name)
	require.True(t, client.requests[0].Tools[0].Strict)
	require.Equal(t, "function", client.requests[0].Tools[1].Type)
	require.Equal(t, "web-search", client.requests[0].Tools[1].Name)
	require.True(t, client.requests[0].Tools[1].Strict)
	require.Equal(t, "memory-get", client.requests[0].Tools[2].Name)
	require.Equal(t, "memory-set", client.requests[0].Tools[3].Name)
	require.Equal(t, "list-cron-tasks", client.requests[0].Tools[4].Name)
	require.Equal(t, "add-cron-task", client.requests[0].Tools[5].Name)
	require.Equal(t, "delete-cron-task", client.requests[0].Tools[6].Name)

	inputItems, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, inputItems, 2)
	require.Contains(t, string(inputItems[0]), "The current user is not an admin.")
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}`, string(inputItems[1]))
}

func TestReplyOmitsDisabledBuiltInSearchTools(t *testing.T) {
	var client stubClient
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(
		chat.Dependencies{
			Client:          &client,
			HistoryStore:    &historyStore,
			CronTaskStore:   &cronTaskStore,
			CronTaskManager: &cronTaskManager,
		},
		chat.WithXSearchEnabled(false),
		chat.WithWebSearchEnabled(false),
	)
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "I don't have a response yet.", reply.Text)
	require.Len(t, client.requests, 1)
	require.Len(t, client.requests[0].Tools, 5)
	require.Equal(t, "memory-get", client.requests[0].Tools[0].Name)
	require.Equal(t, "memory-set", client.requests[0].Tools[1].Name)
	require.Equal(t, "list-cron-tasks", client.requests[0].Tools[2].Name)
	require.Equal(t, "add-cron-task", client.requests[0].Tools[3].Name)
	require.Equal(t, "delete-cron-task", client.requests[0].Tools[4].Name)
}

func TestReplyRequiresMessage(t *testing.T) {
	var client stubClient
	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "   ",
	})
	require.Nil(t, reply)
	require.ErrorIs(t, err, chat.ErrMissingMessage)
}

func TestReplyAllowsImageOnlyInput(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"I can see the image"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID: 7,
		Image: &chat.InputImage{
			MIMEType:   "image/png",
			DataBase64: "aGVsbG8=",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "I can see the image", reply.Text)

	inputItems, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}`, string(inputItems[1]))
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,aGVsbG8="}]}`, string(historyStore.appendCalls[0][0]))
}

func TestReplyBuildsCaptionAndImageUserItem(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Done"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "look at this",
		Image: &chat.InputImage{
			MIMEType:   "image/jpeg",
			DataBase64: "YmFzZTY0",
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Done", reply.Text)

	inputItems, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"look at this"},{"type":"input_image","image_url":"data:image/jpeg;base64,YmFzZTY0"}]}`, string(inputItems[1]))
}

func TestReplyIncludesAndExecutesRuntimeTools(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"github__search","arguments":"{\"query\":\"hello\"}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Final answer"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager
	runtime := &stubToolRuntime{
		tools: []*responses.Tool{
			{
				Type:        "function",
				Name:        "github__search",
				Description: "MCP tool",
			},
		},
		result:  "MCP result",
		handled: true,
	}
	factory := &stubToolRuntimeFactory{runtime: runtime}

	service, err := newTestService(
		chat.Dependencies{
			Client:          &client,
			XSearcher:       &xSearcher,
			WebSearcher:     &webSearcher,
			HistoryStore:    &historyStore,
			CronTaskStore:   &cronTaskStore,
			CronTaskManager: &cronTaskManager,
			Model:           "gpt-5",
		},
		chat.WithToolRuntimeFactory(factory),
	)
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "Final answer", reply.Text)
	require.Equal(t, 1, factory.opens)
	require.Len(t, client.requests[0].Tools, 8)
	require.Equal(t, "github__search", client.requests[0].Tools[7].Name)
	require.Len(t, runtime.executions, 1)
	require.Equal(t, "github__search", runtime.executions[0].name)
	require.JSONEq(t, `{"type":"function_call_output","call_id":"call_1","output":"MCP result"}`, string(historyStore.appendCalls[2][0]))
	require.Equal(t, 1, runtime.closeCalls)
}

func TestReplyReturnsOutputTextWithoutFunctionCall(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Hi from model"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "Hi from model", reply.Text)
	require.Equal(t, "gpt-5", client.requests[0].Model)
	require.False(t, reply.UsedTool)
	require.Len(t, historyStore.appendCalls, 2)
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}`, string(historyStore.appendCalls[0][0]))
	require.JSONEq(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hi from model"}]}`, string(historyStore.appendCalls[1][0]))
}

func TestReplyUsesPersistedHistoryAndStoresFunctionLoopItems(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(
			`{"type":"reasoning","id":"rs_1","encrypted_content":"enc_1"}`,
			`{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{\"query\":\"latest xAI announcements\",\"allowed_x_handles\":[\"xai\"]}"}`,
		),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Final answer"}]}`),
	}

	var xSearcher stubXSearchExecutor
	xSearcher.result = &searchx.SearchResult{Text: "Search result"}
	var webSearcher stubWebSearchExecutor

	var historyStore stubHistoryStore
	historyStore.listItems = []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"Earlier question"}]}`),
		json.RawMessage(`{"type":"message","content":[{"type":"output_text","text":"Earlier answer"}]}`),
	}
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	var toolStatuses []string
	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  77,
		Message: "hello",
		NotifyToolCall: func(statusText string) error {
			toolStatuses = append(toolStatuses, statusText)
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"Searching on X for: latest xAI announcements"}, toolStatuses)
	require.Equal(t, "Final answer", reply.Text)
	require.True(t, reply.UsedTool)
	require.True(t, reply.PlaceholderNeeded)
	require.Len(t, xSearcher.requests, 1)
	require.Empty(t, webSearcher.requests)
	require.Len(t, client.requests, 2)

	firstInput, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, firstInput, 4)
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}`, string(firstInput[3]))

	secondInput, ok := client.requests[1].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, secondInput, 6)
	require.JSONEq(t, `{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{\"query\":\"latest xAI announcements\",\"allowed_x_handles\":[\"xai\"]}"}`, string(secondInput[4]))
	require.JSONEq(t, `{"type":"function_call_output","call_id":"call_1","output":"Search result"}`, string(secondInput[5]))
	require.Empty(t, client.requests[1].PreviousResponseID)

	require.Len(t, historyStore.appendCalls, 4)
	require.JSONEq(t, `{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{\"query\":\"latest xAI announcements\",\"allowed_x_handles\":[\"xai\"]}"}`, string(historyStore.appendCalls[1][0]))
	require.JSONEq(t, `{"type":"function_call_output","call_id":"call_1","output":"Search result"}`, string(historyStore.appendCalls[2][0]))
}

func TestReplyReplaysPersistedImageHistory(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Final answer"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	historyStore.listItems = []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"Earlier question"},{"type":"input_image","image_url":"data:image/png;base64,b2xk"}]}`),
		json.RawMessage(`{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Earlier answer"}]}`),
	}
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  77,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "Final answer", reply.Text)

	firstInput, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, firstInput, 4)
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"Earlier question"},{"type":"input_image","image_url":"data:image/png;base64,b2xk"}]}`, string(firstInput[1]))
	require.JSONEq(t, `{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]}`, string(firstInput[3]))
}

func TestReplyNotifiesEveryToolStepWithSpecificStatus(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{\"query\":\"golang release\",\"allowed_x_handles\":null,\"excluded_x_handles\":null,\"from_date\":null,\"to_date\":null,\"enable_image_understanding\":false,\"enable_video_understanding\":false}"}`),
		buildResponse(`{"type":"function_call","call_id":"call_2","name":"web-search","arguments":"{\"query\":\"golang 1.25 release notes\",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Final answer"}]}`),
	}

	var xSearcher stubXSearchExecutor
	xSearcher.result = &searchx.SearchResult{Text: "X result"}
	var webSearcher stubWebSearchExecutor
	webSearcher.result = &websearch.SearchResult{Text: "Web result"}
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	var toolStatuses []string
	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  77,
		Message: "hello",
		NotifyToolCall: func(statusText string) error {
			toolStatuses = append(toolStatuses, statusText)
			return nil
		},
	})
	require.NoError(t, err)
	require.Equal(t, "Final answer", reply.Text)
	require.Equal(t, []string{
		"Searching on X for: golang release",
		"Searching the web for: golang 1.25 release notes",
	}, toolStatuses)
}

func TestReplyStoresNormalizedAssistantTextOnly(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(
			`{"type":"reasoning","id":"rs_1","encrypted_content":"enc_1"}`,
			`{"type":"message","content":[{"type":"output_text","text":"Hello"},{"type":"output_text","text":" world"}]}`,
		),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  9,
		Message: "new",
	})
	require.NoError(t, err)
	require.Equal(t, "Hello world", reply.Text)
	require.Len(t, historyStore.appendCalls, 2)
	require.JSONEq(t, `{"type":"message","role":"assistant","content":[{"type":"output_text","text":"Hello world"}]}`, string(historyStore.appendCalls[1][0]))
}

func TestReplyRejectsMultipleFunctionCallsInSingleStep(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(
			`{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{\"query\":\"first\"}"}`,
			`{"type":"function_call","call_id":"call_2","name":"x-search","arguments":"{\"query\":\"second\"}"}`,
		),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager
	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.Nil(t, reply)
	require.ErrorIs(t, err, chat.ErrTooManyFunctionCallsInStep)
}

func TestReplyRejectsInvalidFunctionArguments(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{invalid"}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager
	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.Nil(t, reply)
	require.ErrorContains(t, err, "parse x-search function arguments")
}

func TestReplyRoutesWebSearchFunctionCall(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"web-search","arguments":"{\"query\":\"latest xAI announcements\",\"allowed_domains\":[\"x.ai\"],\"excluded_domains\":null,\"enable_image_understanding\":true}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Final answer"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	webSearcher.result = &websearch.SearchResult{Text: "Web search result"}
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "Final answer", reply.Text)
	require.Empty(t, xSearcher.requests)
	require.Len(t, webSearcher.requests, 1)
	require.Equal(t, "latest xAI announcements", webSearcher.requests[0].Query)
	require.Equal(t, []string{"x.ai"}, webSearcher.requests[0].AllowedDomains)
	require.True(t, webSearcher.requests[0].EnableImageUnderstanding)
}

func TestReplyDoesNotFailOnEmptyXSearchQuery(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"x-search","arguments":"{\"query\":\"\",\"allowed_x_handles\":null,\"excluded_x_handles\":null,\"from_date\":null,\"to_date\":null,\"enable_image_understanding\":false,\"enable_video_understanding\":false}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"What exactly should I search for on X?"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "search something",
	})
	require.NoError(t, err)
	require.Equal(t, "What exactly should I search for on X?", reply.Text)
	require.Empty(t, xSearcher.requests)
	require.Len(t, historyStore.appendCalls, 4)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Search tool input rejected: the query is empty.")
}

func TestReplyDoesNotFailOnEmptyWebSearchQuery(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"web-search","arguments":"{\"query\":\"   \",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"What should I look up on the web?"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "search the web",
	})
	require.NoError(t, err)
	require.Equal(t, "What should I look up on the web?", reply.Text)
	require.Empty(t, webSearcher.requests)
	require.Len(t, historyStore.appendCalls, 4)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Search tool input rejected: the query is empty.")
}

func TestReplyAddsCronTaskForAnyUser(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"add-cron-task","arguments":"{\"schedule\":\"0 9 * * *\",\"prompt\":\"send daily update\"}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Scheduled"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	cronTaskStore.createResult = &repo.CronTask{
		ID:            42,
		ChatID:        99,
		CreatedByTGID: 7,
		Schedule:      "0 9 * * *",
		Prompt:        "send daily update",
		CreatedAt:     time.Unix(0, 0),
	}
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:   99,
		UserTGID: 7,
		Message:  "Create daily schedule",
	})
	require.NoError(t, err)
	require.Equal(t, "Scheduled", reply.Text)
	require.Len(t, cronTaskStore.createCalls, 1)
	require.Len(t, cronTaskManager.addCalls, 1)
	require.Equal(t, int64(42), cronTaskManager.addCalls[0].ID)
	require.Contains(t, cronTaskStore.createCalls[0].Prompt, "Return the final user-facing answer only as Telegram-compatible HTML.")
	require.Len(t, historyStore.appendCalls, 4)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Cron task created.")
}

func TestReplyListsCronTasksForAnyUser(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"list-cron-tasks","arguments":"{}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Listed"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	cronTaskStore.listResult = []*repo.CronTask{
		{
			ID:       11,
			ChatID:   99,
			Schedule: "0 9 * * *",
			Prompt:   "send daily update",
		},
		{
			ID:       12,
			ChatID:   99,
			Schedule: "@daily",
			Prompt:   "send digest",
		},
	}
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:   99,
		UserTGID: 7,
		Message:  "List schedules",
	})
	require.NoError(t, err)
	require.Equal(t, "Listed", reply.Text)
	require.Equal(t, []int64{99}, cronTaskStore.listCalls)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Cron tasks for this chat:")
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Task ID: 11")
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Task ID: 12")
}

func TestReplyReturnsEmptyCronTaskListForAnyUser(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"list-cron-tasks","arguments":"{}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Empty"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:   99,
		UserTGID: 7,
		Message:  "List schedules",
	})
	require.NoError(t, err)
	require.Equal(t, "Empty", reply.Text)
	require.Equal(t, []int64{99}, cronTaskStore.listCalls)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "No cron tasks found for this chat.")
}

func TestReplyListsCronTasksForNonAdmin(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"list-cron-tasks","arguments":"{}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Listed"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	cronTaskStore.listResult = []*repo.CronTask{
		{
			ID:       21,
			ChatID:   99,
			Schedule: "*/15 * * * *",
			Prompt:   "check the web for event updates",
		},
	}
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  99,
		Message: "List schedules",
	})
	require.NoError(t, err)
	require.Equal(t, "Listed", reply.Text)
	require.Equal(t, []int64{99}, cronTaskStore.listCalls)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Task ID: 21")
}

func TestReplyAddsCronTaskForNonAdmin(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"add-cron-task","arguments":"{\"schedule\":\"0 9 * * *\",\"prompt\":\"send daily update\"}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Scheduled"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	cronTaskStore.createResult = &repo.CronTask{
		ID:            42,
		ChatID:        99,
		CreatedByTGID: 0,
		Schedule:      "0 9 * * *",
		Prompt:        "send daily update",
		CreatedAt:     time.Unix(0, 0),
	}
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  99,
		Message: "Create daily schedule",
	})
	require.NoError(t, err)
	require.Equal(t, "Scheduled", reply.Text)
	require.Len(t, cronTaskStore.createCalls, 1)
	require.Len(t, cronTaskManager.addCalls, 1)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Cron task created.")
}

func TestReplyRejectsDeletingCronTaskFromAnotherChat(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"delete-cron-task","arguments":"{\"task_id\":42}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Denied"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	cronTaskStore.getResult = &repo.CronTask{
		ID:       42,
		ChatID:   777,
		Schedule: "0 9 * * *",
		Prompt:   "send daily update",
	}
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:   99,
		UserTGID: 7,
		Message:  "Delete schedule",
	})
	require.NoError(t, err)
	require.Equal(t, "Denied", reply.Text)
	require.Len(t, cronTaskStore.getCalls, 1)
	require.Empty(t, cronTaskStore.deleteCalls)
	require.Empty(t, cronTaskManager.removeCalls)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "does not belong to this chat")
}

func TestExecuteScheduledDoesNotUseHistoryAndDisablesCronTools(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"web-search","arguments":"{\"query\":\"latest news\",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Scheduled answer"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	webSearcher.result = &websearch.SearchResult{Text: "Web result"}
	var historyStore stubHistoryStore
	historyStore.listItems = []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":[{"type":"input_text","text":"old"}]}`),
	}
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	text, err := service.ExecuteScheduled(context.Background(), 55, "run task")
	require.NoError(t, err)
	require.Equal(t, "Scheduled answer", text)
	require.Empty(t, historyStore.appendCalls)
	require.Len(t, client.requests, 2)

	firstInput, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, firstInput, 2)
	require.Contains(t, string(firstInput[0]), "The current user is not an admin.")
	require.Contains(t, string(firstInput[0]), "This request is a scheduled cron execution.")
	require.Contains(t, string(firstInput[0]), "Do not explain scheduling limitations")

	require.Len(t, client.requests[0].Tools, 4)
	require.Equal(t, "x-search", client.requests[0].Tools[0].Name)
	require.Equal(t, "web-search", client.requests[0].Tools[1].Name)
	require.Equal(t, "memory-get", client.requests[0].Tools[2].Name)
	require.Equal(t, "memory-set", client.requests[0].Tools[3].Name)
}

func TestReplySystemPromptGuidesCronPromptAuthoring(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"ok"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
		Model:           "gpt-5",
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:   99,
		UserTGID: 7,
		Message:  "Create daily schedule",
		IsAdmin:  true,
	})
	require.NoError(t, err)
	require.Equal(t, "ok", reply.Text)

	inputItems, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, inputItems, 2)
	require.Contains(t, string(inputItems[0]), "When creating a cron task, write the saved prompt as a standalone execution instruction for future runs")
	require.Contains(t, string(inputItems[0]), "For any cron task that plans or executes multi-step work, require the saved prompt to spell out a clear step-by-step execution plan")
	require.Contains(t, string(inputItems[0]), "For normal chat, if the latest user request is ambiguous, underspecified, or missing a critical detail, ask one short clarifying question")
	require.Contains(t, string(inputItems[0]), "Never call x-search or web-search with an empty, placeholder, or overly vague query")
	require.Contains(t, string(inputItems[0]), "Ensure the saved prompt explicitly requires the final answer to be Telegram-compatible HTML")
	require.Contains(t, string(inputItems[0]), "If the user asks to monitor, remind, notify, check, search for, or report something on a repeating cadence")
	require.Contains(t, string(inputItems[0]), "treat that as a cron task request, not as chat memory")
	require.Contains(t, string(inputItems[0]), "Do not store schedules, recurring reminders, or recurring monitoring instructions in memory")
	require.Contains(t, string(inputItems[0]), "first call memory-get, then call memory-set with the full updated memory text")
}

func TestReplyPromptForRecurringRequestMakesCronToolsTheRightAction(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"add-cron-task","arguments":"{\"schedule\":\"*/15 * * * *\",\"prompt\":\"Every run, check the web for the requested event and send only new relevant updates as Telegram-compatible HTML.\"}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Scheduled"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	cronTaskStore.createResult = &repo.CronTask{
		ID:            77,
		ChatID:        99,
		CreatedByTGID: 7,
		Schedule:      "*/15 * * * *",
		Prompt:        "Every run, check the web for the requested event and send only new relevant updates as Telegram-compatible HTML.",
		CreatedAt:     time.Unix(0, 0),
	}
	var cronTaskManager stubCronTaskManager
	var memoryStore stubMemoryStore

	service, err := newTestService(
		chat.Dependencies{
			Client:          &client,
			XSearcher:       &xSearcher,
			WebSearcher:     &webSearcher,
			HistoryStore:    &historyStore,
			CronTaskStore:   &cronTaskStore,
			CronTaskManager: &cronTaskManager,
			Model:           "gpt-5",
		},
		chat.WithMemoryStore(&memoryStore),
	)
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:   99,
		UserTGID: 7,
		Message:  "Кожні 15 хвилин повідомляй про певну подію в інтернеті",
	})
	require.NoError(t, err)
	require.Equal(t, "Scheduled", reply.Text)
	require.Len(t, cronTaskStore.createCalls, 1)
	require.Empty(t, memoryStore.setCalls)
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Cron task created.")
}

func TestReplyReturnsClientError(t *testing.T) {
	var client stubClient
	client.err = errors.New("boom")

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager
	service, err := newTestService(chat.Dependencies{
		Client:          &client,
		XSearcher:       &xSearcher,
		WebSearcher:     &webSearcher,
		HistoryStore:    &historyStore,
		CronTaskStore:   &cronTaskStore,
		CronTaskManager: &cronTaskManager,
	})
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "hello",
	})
	require.Nil(t, reply)
	require.EqualError(t, err, "boom")
}

func TestReplyInjectsPersistedMemoryIntoSystemPrompt(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"ok"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var memoryStore stubMemoryStore
	memoryStore.memory = map[int64]string{
		77: "Reply briefly. Remember the launch date is 2026-05-01.",
	}
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(
		chat.Dependencies{
			Client:          &client,
			XSearcher:       &xSearcher,
			WebSearcher:     &webSearcher,
			HistoryStore:    &historyStore,
			CronTaskStore:   &cronTaskStore,
			CronTaskManager: &cronTaskManager,
			Model:           "gpt-5",
		},
		chat.WithMemoryStore(&memoryStore),
	)
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  77,
		Message: "hello",
	})
	require.NoError(t, err)
	require.Equal(t, "ok", reply.Text)
	require.Equal(t, []int64{77}, memoryStore.getCalls)

	inputItems, ok := client.requests[0].Input.([]json.RawMessage)
	require.True(t, ok)
	require.Contains(t, string(inputItems[0]), "[chat-memory]")
	require.Contains(t, string(inputItems[0]), "Reply briefly. Remember the launch date is 2026-05-01.")
}

func TestReplySupportsMemoryGetAndSetTools(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"memory-get","arguments":"{}"}`),
		buildResponse(`{"type":"function_call","call_id":"call_2","name":"memory-set","arguments":"{\"text\":\"Reply in Ukrainian. Remember the project is MangoDuck.\"}"}`),
		buildResponse(`{"type":"message","content":[{"type":"output_text","text":"Saved"}]}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	var historyStore stubHistoryStore
	var memoryStore stubMemoryStore
	memoryStore.memory = map[int64]string{
		88: "Reply politely.",
	}
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := newTestService(
		chat.Dependencies{
			Client:          &client,
			XSearcher:       &xSearcher,
			WebSearcher:     &webSearcher,
			HistoryStore:    &historyStore,
			CronTaskStore:   &cronTaskStore,
			CronTaskManager: &cronTaskManager,
			Model:           "gpt-5",
		},
		chat.WithMemoryStore(&memoryStore),
	)
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  88,
		Message: "Запам'ятай нову поведінку",
	})
	require.NoError(t, err)
	require.Equal(t, "Saved", reply.Text)
	require.Equal(t, []int64{88, 88}, memoryStore.getCalls)
	require.Len(t, memoryStore.setCalls, 1)
	require.Equal(t, int64(88), memoryStore.setCalls[0].chatID)
	require.Equal(t, "Reply in Ukrainian. Remember the project is MangoDuck.", memoryStore.setCalls[0].text)
	require.Equal(t, "Reply in Ukrainian. Remember the project is MangoDuck.", memoryStore.memory[88])
	require.Contains(t, string(historyStore.appendCalls[2][0]), "Current memory:")
	require.Contains(t, string(historyStore.appendCalls[4][0]), "Memory updated.")
}

func buildResponse(items ...string) *responses.Response {
	response := &responses.Response{
		Output: make([]*responses.OutputItem, 0, len(items)),
	}

	for _, item := range items {
		var outputItem responses.OutputItem
		err := json.Unmarshal([]byte(item), &outputItem)
		if err != nil {
			panic(err)
		}

		response.Output = append(response.Output, &outputItem)
	}

	return response
}

func cloneRaw(items []json.RawMessage) []json.RawMessage {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, append(json.RawMessage(nil), item...))
	}

	return cloned
}
