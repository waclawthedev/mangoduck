package chat_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"mangoduck/internal/llm/chat"
	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/websearch"
)

func TestReplyReturnsFallbackTextWhenToolLoopEndsWithoutFinalAnswer(t *testing.T) {
	var client stubClient
	client.replies = []*responses.Response{
		buildResponse(`{"type":"function_call","call_id":"call_1","name":"web-search","arguments":"{\"query\":\"golang\",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
		buildResponse(`{"type":"function_call","call_id":"call_2","name":"web-search","arguments":"{\"query\":\"golang\",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
		buildResponse(`{"type":"function_call","call_id":"call_3","name":"web-search","arguments":"{\"query\":\"golang\",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
		buildResponse(`{"type":"function_call","call_id":"call_4","name":"web-search","arguments":"{\"query\":\"golang\",\"allowed_domains\":null,\"excluded_domains\":null,\"enable_image_understanding\":false}"}`),
	}

	var xSearcher stubXSearchExecutor
	var webSearcher stubWebSearchExecutor
	webSearcher.result = &websearch.SearchResult{Text: "search result"}
	var historyStore stubHistoryStore
	var cronTaskStore stubCronTaskStore
	var cronTaskManager stubCronTaskManager

	service, err := chat.NewService(&client, &xSearcher, &webSearcher, &historyStore, &cronTaskStore, &cronTaskManager, "gpt-5")
	require.NoError(t, err)

	reply, err := service.Reply(context.Background(), &chat.Request{
		ChatID:  7,
		Message: "find golang",
	})
	require.NoError(t, err)
	require.Equal(t, chat.DefaultNoResponseText, reply.Text)
	require.True(t, reply.UsedTool)
}
