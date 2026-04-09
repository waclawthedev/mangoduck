package xai_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/responses/xai"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResponse(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.Equal(t, http.MethodPost, r.Method) {
			return
		}
		if !assert.Equal(t, "/v1/responses", r.URL.Path) {
			return
		}
		assert.Equal(t, "Bearer xai-key", r.Header.Get("Authorization"))

		var payload responses.CreateResponseRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Equal(t, "grok-4-fast", payload.Model)
		assert.Equal(t, "hello", payload.Input)

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"id":"resp_456","object":"response","model":"grok-4-fast","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Hi from xAI"}]}]}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := xai.NewClient(xai.Config{
		APIKey:  "xai-key",
		BaseURL: server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "grok-4-fast"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Equal(t, "resp_456", response.ID)
	require.Equal(t, "Hi from xAI", response.OutputText())
}

func TestNewClientRequiresAPIKey(t *testing.T) {
	t.Parallel()

	client, err := xai.NewClient(xai.Config{})
	require.Nil(t, client)
	require.ErrorIs(t, err, responses.ErrMissingAPIKey)
}

func TestCreateResponseReturnsStringAPIError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)

		_, err := w.Write([]byte(`{"error":"search x is not available for this model"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := xai.NewClient(xai.Config{
		APIKey:  "xai-key",
		BaseURL: server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "grok-4-fast"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.Nil(t, response)

	var apiErr *responses.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "xai", apiErr.Provider)
	require.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
	require.Equal(t, "search x is not available for this model", apiErr.Message)
}
