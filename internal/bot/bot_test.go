package bot

import (
	"testing"

	"mangoduck/internal/config"
	openairesponses "mangoduck/internal/llm/responses/openai"
	xairesponses "mangoduck/internal/llm/responses/xai"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestStopRuntimeStopsSchedulerBeforeBot(t *testing.T) {
	order := make([]string, 0, 2)

	stopRuntime(func() {
		order = append(order, "scheduler")
	}, func() {
		order = append(order, "bot")
	})

	require.Equal(t, []string{"scheduler", "bot"}, order)
}

func TestNewResponsesClientSelectsOpenAI(t *testing.T) {
	var cfg config.Config
	cfg.ResponsesProvider = "openai"
	cfg.ResponsesProviderAPIKey = "openai-key"

	client, err := newResponsesClient(cfg, zap.NewNop())
	require.NoError(t, err)

	_, ok := client.(*openairesponses.Client)
	require.True(t, ok)
}

func TestNewResponsesClientSelectsXAI(t *testing.T) {
	var cfg config.Config
	cfg.ResponsesProvider = "xai"
	cfg.ResponsesProviderAPIKey = "xai-key"

	client, err := newResponsesClient(cfg, zap.NewNop())
	require.NoError(t, err)

	_, ok := client.(*xairesponses.Client)
	require.True(t, ok)
}
