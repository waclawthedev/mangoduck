package bot

import (
	"testing"

	"github.com/stretchr/testify/require"
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
