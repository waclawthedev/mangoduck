package bot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"mangoduck/internal/config"
	"mangoduck/internal/llm/responses"
)

const testOpenAIWebSearchModel = "gpt-5.4-nano"

type preflightResponsesClientStub struct {
	err      error
	requests []*responses.CreateResponseRequest
}

func (s *preflightResponsesClientStub) CreateResponse(_ context.Context, request *responses.CreateResponseRequest) (*responses.Response, error) {
	s.requests = append(s.requests, request)
	if s.err != nil {
		return nil, s.err
	}

	var response responses.Response
	response.ID = "resp_preflight"

	return &response, nil
}

type preflightMCPCheckerStub struct {
	err        error
	callCount  int
	lastCalled bool
}

func (s *preflightMCPCheckerStub) Preflight(_ context.Context) error {
	s.callCount++
	s.lastCalled = true

	return s.err
}

func TestRunStartupPreflightChecksResponsesXAIAndMCP(t *testing.T) {
	var mainClient preflightResponsesClientStub
	var xaiClient preflightResponsesClientStub
	var openAIWebSearchClient preflightResponsesClientStub
	var mcpChecker preflightMCPCheckerStub
	var cfg config.Config
	cfg.MainModel = "gpt-5"
	cfg.XAIModel = "grok-4-fast-search"
	cfg.OpenAIWebSearchModel = testOpenAIWebSearchModel
	cfg.ResponsesTimeout = 3 * time.Second
	cfg.XAITimeout = 4 * time.Second
	cfg.OpenAIWebSearchTimeout = 5 * time.Second

	err := runStartupPreflight(context.Background(), cfg, zap.NewNop(), &mainClient, &xaiClient, &openAIWebSearchClient, &mcpChecker)
	require.NoError(t, err)

	require.Len(t, mainClient.requests, 1)
	require.Equal(t, "gpt-5", mainClient.requests[0].Model)
	require.Equal(t, startupPreflightPrompt, mainClient.requests[0].Input)
	require.NotNil(t, mainClient.requests[0].Store)
	require.False(t, *mainClient.requests[0].Store)

	require.Len(t, xaiClient.requests, 1)
	require.Equal(t, "grok-4-fast-search", xaiClient.requests[0].Model)
	require.Equal(t, startupPreflightPrompt, xaiClient.requests[0].Input)

	require.Len(t, openAIWebSearchClient.requests, 1)
	require.Equal(t, testOpenAIWebSearchModel, openAIWebSearchClient.requests[0].Model)
	require.Equal(t, startupPreflightPrompt, openAIWebSearchClient.requests[0].Input)

	require.Equal(t, 1, mcpChecker.callCount)
	require.True(t, mcpChecker.lastCalled)
}

func TestRunStartupPreflightSkipsDisabledBuiltInSearchTools(t *testing.T) {
	var mainClient preflightResponsesClientStub
	var xaiClient preflightResponsesClientStub
	var openAIWebSearchClient preflightResponsesClientStub
	var mcpChecker preflightMCPCheckerStub
	var cfg config.Config
	enableXSearch := false
	enableOpenAIWebSearch := false
	cfg.EnableXSearch = &enableXSearch
	cfg.EnableOpenAIWebSearch = &enableOpenAIWebSearch

	err := runStartupPreflight(context.Background(), cfg, zap.NewNop(), &mainClient, &xaiClient, &openAIWebSearchClient, &mcpChecker)
	require.NoError(t, err)

	require.Len(t, mainClient.requests, 1)
	require.Empty(t, xaiClient.requests)
	require.Empty(t, openAIWebSearchClient.requests)
	require.Equal(t, 1, mcpChecker.callCount)
}

func TestRunStartupPreflightUsesDefaultModels(t *testing.T) {
	var mainClient preflightResponsesClientStub
	var xaiClient preflightResponsesClientStub
	var openAIWebSearchClient preflightResponsesClientStub

	err := runStartupPreflight(context.Background(), config.Config{}, zap.NewNop(), &mainClient, &xaiClient, &openAIWebSearchClient, nil)
	require.NoError(t, err)

	require.Len(t, mainClient.requests, 1)
	require.Equal(t, "gpt-5-mini", mainClient.requests[0].Model)

	require.Len(t, xaiClient.requests, 1)
	require.Equal(t, "grok-4-1-fast-reasoning", xaiClient.requests[0].Model)

	require.Len(t, openAIWebSearchClient.requests, 1)
	require.Equal(t, testOpenAIWebSearchModel, openAIWebSearchClient.requests[0].Model)
}

func TestRunStartupPreflightFailsFastOnResponsesAPI(t *testing.T) {
	var mainClient preflightResponsesClientStub
	var xaiClient preflightResponsesClientStub
	var openAIWebSearchClient preflightResponsesClientStub
	var mcpChecker preflightMCPCheckerStub
	mainClient.err = errors.New("dial tcp: connection refused")

	err := runStartupPreflight(context.Background(), config.Config{}, zap.NewNop(), &mainClient, &xaiClient, &openAIWebSearchClient, &mcpChecker)
	require.Error(t, err)
	require.ErrorContains(t, err, "responses api preflight request failed")

	require.Empty(t, xaiClient.requests)
	require.Empty(t, openAIWebSearchClient.requests)
	require.Equal(t, 0, mcpChecker.callCount)
}

func TestRunStartupPreflightFailsFastOnOpenAIWebSearchAPI(t *testing.T) {
	var mainClient preflightResponsesClientStub
	var xaiClient preflightResponsesClientStub
	var openAIWebSearchClient preflightResponsesClientStub
	var mcpChecker preflightMCPCheckerStub
	openAIWebSearchClient.err = errors.New("invalid api key")

	err := runStartupPreflight(context.Background(), config.Config{}, zap.NewNop(), &mainClient, &xaiClient, &openAIWebSearchClient, &mcpChecker)
	require.Error(t, err)
	require.ErrorContains(t, err, "openai web search api preflight request failed")

	require.Len(t, mainClient.requests, 1)
	require.Len(t, xaiClient.requests, 1)
	require.Equal(t, 0, mcpChecker.callCount)
}

func TestRunStartupPreflightReturnsMCPError(t *testing.T) {
	var mainClient preflightResponsesClientStub
	var xaiClient preflightResponsesClientStub
	var openAIWebSearchClient preflightResponsesClientStub
	var mcpChecker preflightMCPCheckerStub
	mcpChecker.err = errors.New("mcp server \"github\" connect: unauthorized")

	err := runStartupPreflight(context.Background(), config.Config{}, zap.NewNop(), &mainClient, &xaiClient, &openAIWebSearchClient, &mcpChecker)
	require.Error(t, err)
	require.ErrorContains(t, err, "mcp preflight failed")
	require.ErrorContains(t, err, "github")
}
