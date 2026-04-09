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
		assert.Equal(t, "gpt-5-mini", payload.Model)
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
	request.Model = "gpt-5-mini"
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
	request.Model = "gpt-5-mini"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.Nil(t, response)

	var apiErr *responses.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "openai", apiErr.Provider)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, "invalid api key", apiErr.Message)
}
