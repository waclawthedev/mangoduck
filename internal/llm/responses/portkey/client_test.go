package portkey_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/responses/portkey"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClientRequiresProviderAuthWithoutVirtualKeyOrConfig(t *testing.T) {
	t.Parallel()

	client, err := portkey.NewClient(portkey.Config{})
	require.Nil(t, client)
	require.ErrorIs(t, err, responses.ErrMissingProviderAuth)
}

func TestNewClientAllowsProviderAuthWithoutPortkeyAPIKey(t *testing.T) {
	t.Parallel()

	client, err := portkey.NewClient(portkey.Config{
		ProviderAPIKey: "openai-key",
	})
	require.NoError(t, err)
	require.NotNil(t, client)
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
		assert.Empty(t, r.Header.Get("X-Portkey-Api-Key"))
		assert.Equal(t, "openai", r.Header.Get("X-Portkey-Provider"))
		assert.Equal(t, "Bearer openai-key", r.Header.Get("Authorization"))
		assert.Equal(t, "1000", r.Header.Get("X-Portkey-Request-Timeout"))

		var payload responses.CreateResponseRequest
		err := json.NewDecoder(r.Body).Decode(&payload)
		assert.NoError(t, err)
		assert.Equal(t, "gpt-5-mini", payload.Model)
		assert.Equal(t, "hello", payload.Input)
		assert.Nil(t, payload.Store)
		assert.Nil(t, payload.ParallelToolCalls)

		w.Header().Set("Content-Type", "application/json")
		_, err = w.Write([]byte(`{"id":"resp_123","object":"response","model":"gpt-5-mini","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"Hi there"}]}]}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := portkey.NewClient(portkey.Config{
		ProviderAPIKey: "openai-key",
		BaseURL:        server.URL,
		Timeout:        time.Second,
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

	client, err := portkey.NewClient(portkey.Config{
		ProviderAPIKey: "openai-key",
		BaseURL:        server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "gpt-5-mini"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.Nil(t, response)

	var apiErr *responses.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, "portkey/openai", apiErr.Provider)
	require.Equal(t, http.StatusUnauthorized, apiErr.StatusCode)
	require.Equal(t, "invalid api key", apiErr.Message)
}

func TestCreateResponseUsesVirtualKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("X-Portkey-Api-Key"))
		assert.Equal(t, "virtual-key", r.Header.Get("X-Portkey-Virtual-Key"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Portkey-Provider"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_999","object":"response","model":"gpt-5-mini","status":"completed"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := portkey.NewClient(portkey.Config{
		VirtualKey: "virtual-key",
		BaseURL:    server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "gpt-5-mini"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Equal(t, "resp_999", response.ID)
}

func TestCreateResponseIncludesPortkeyAPIKeyWhenProvided(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "portkey-key", r.Header.Get("X-Portkey-Api-Key"))
		assert.Equal(t, "openai", r.Header.Get("X-Portkey-Provider"))
		assert.Equal(t, "Bearer openai-key", r.Header.Get("Authorization"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_hosted","object":"response","model":"gpt-5-mini","status":"completed"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := portkey.NewClient(portkey.Config{
		APIKey:         "portkey-key",
		ProviderAPIKey: "openai-key",
		BaseURL:        server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "gpt-5-mini"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Equal(t, "resp_hosted", response.ID)
}

func TestCreateResponseUsesConfigID(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "config-id", r.Header.Get("X-Portkey-Config"))
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("X-Portkey-Provider"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_cfg","object":"response","model":"gpt-5-mini","status":"completed"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := portkey.NewClient(portkey.Config{
		ConfigID: "config-id",
		BaseURL:  server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "gpt-5-mini"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Equal(t, "resp_cfg", response.ID)
}

func TestCreateResponseUsesCustomHost(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "https://api.openai.com", r.Header.Get("X-Portkey-Custom-Host"))
		assert.Equal(t, "openai", r.Header.Get("X-Portkey-Provider"))

		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"id":"resp_custom_host","object":"response","model":"gpt-5-mini","status":"completed"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client, err := portkey.NewClient(portkey.Config{
		VirtualKey: "virtual-key",
		CustomHost: "https://api.openai.com",
		BaseURL:    server.URL,
	})
	require.NoError(t, err)

	var request responses.CreateResponseRequest
	request.Model = "gpt-5-mini"
	request.Input = "hello"

	response, err := client.CreateResponse(context.Background(), &request)
	require.NoError(t, err)
	require.Equal(t, "resp_custom_host", response.ID)
}
