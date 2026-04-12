package openai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/responses/openai"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testOpenAIModel = "gpt-5-mini"

func TestNewClientRequiresAPIKey(t *testing.T) {
	t.Parallel()

	client, err := openai.NewClient(openai.Config{})
	require.Nil(t, client)
	require.ErrorIs(t, err, responses.ErrMissingAPIKey)
}

func TestCreateResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, http.MethodPost, r.Method) {
			return
		}
		if !assert.Equal(t, "/v1/responses", r.URL.Path) {
			return
		}
		assert.Equal(t, "Bearer openai-key", r.Header.Get("Authorization"))

		var payload responses.CreateResponseRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Equal(t, testOpenAIModel, payload.Model)
		assert.Equal(t, "hello", payload.Input)

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"id":"resp_123","object":"response","model":"gpt-5-mini","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Hi there"}]}]}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := openai.NewClient(openai.Config{
		APIKey:  "openai-key",
		BaseURL: server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = testOpenAIModel
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Equal(t, "resp_123", response.ID)
	require.Equal(t, "Hi there", response.OutputText())
}

func TestCreateResponseReturnsAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)

		_, err := w.Write([]byte(`{"error":{"message":"invalid api key","type":"invalid_request_error"}}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := openai.NewClient(openai.Config{
		APIKey:  "openai-key",
		BaseURL: server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = testOpenAIModel
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.Nil(t, response)

	var apiErr *responses.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "openai", apiErr.Provider)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, "invalid api key", apiErr.Message)
}

func TestCreateResponsePreservesToolsAndInputItems(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload responses.CreateResponseRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Len(t, payload.Tools, 1)
		assert.Equal(t, "function", payload.Tools[0].Type)
		assert.Equal(t, "remember", payload.Tools[0].Name)
		assert.Equal(t, "required", payload.ToolChoice)

		inputItems, ok := payload.Input.([]any)
		assert.True(t, ok)
		assert.Len(t, inputItems, 1)

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"id":"resp_tools","object":"response","model":"gpt-5-mini","status":"completed","output":[{"type":"function_call","call_id":"call_123","name":"remember","arguments":"{\"text\":\"hello\"}"}]}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := openai.NewClient(openai.Config{
		APIKey:  "openai-key",
		BaseURL: server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = testOpenAIModel
	request.Input = []any{
		map[string]any{
			"type":    "function_call_output",
			"call_id": "call_prev",
			"output":  "ok",
		},
	}
	request.Tools = []*responses.Tool{
		{
			Type:        "function",
			Name:        "remember",
			Description: "Remember text",
			Parameters: map[string]any{
				"type": "object",
			},
		},
	}
	request.ToolChoice = "required"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Len(t, response.FunctionCalls(), 1)
	require.Equal(t, "remember", response.FunctionCalls()[0].Name)
}
