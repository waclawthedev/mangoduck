package portkey

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"mangoduck/internal/llm/responses"

	"resty.dev/v3"
)

const (
	defaultBaseURL  = "http://127.0.0.1:8787"
	defaultProvider = "openai"
)

type Config struct {
	APIKey         string
	Provider       string
	ProviderAPIKey string
	VirtualKey     string
	ConfigID       string
	CustomHost     string
	BaseURL        string
	Timeout        time.Duration
}

type Client struct {
	httpClient    *resty.Client
	errorProvider string
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	client := resty.New()
	client.SetBaseURL(baseURL)
	client.SetTimeout(cfg.Timeout)
	client.SetHeader("Content-Type", "application/json")
	client.SetHeader("Accept", "application/json")

	providerAPIKey := strings.TrimSpace(cfg.ProviderAPIKey)
	portkeyAPIKey := strings.TrimSpace(cfg.APIKey)
	if portkeyAPIKey != "" {
		client.SetHeader("x-portkey-api-key", portkeyAPIKey)
	}

	if cfg.Timeout > 0 {
		client.SetHeader("x-portkey-request-timeout", strconv.FormatInt(cfg.Timeout.Milliseconds(), 10))
	}

	virtualKey := strings.TrimSpace(cfg.VirtualKey)
	configID := strings.TrimSpace(cfg.ConfigID)
	provider := strings.TrimSpace(cfg.Provider)
	hasProviderHeader := false

	if provider == "" {
		provider = defaultProvider
	}

	errorProvider := "portkey"

	switch {
	case virtualKey != "":
		client.SetHeader("x-portkey-virtual-key", virtualKey)
	case configID != "":
		client.SetHeader("x-portkey-config", configID)
	default:
		if providerAPIKey == "" {
			return nil, responses.ErrMissingProviderAuth
		}

		client.SetHeader("x-portkey-provider", provider)
		client.SetHeader("Authorization", "Bearer "+providerAPIKey)
		hasProviderHeader = true
		errorProvider = "portkey/" + provider
	}

	customHost := strings.TrimSpace(cfg.CustomHost)
	if customHost != "" {
		client.SetHeader("x-portkey-custom-host", customHost)

		if !hasProviderHeader {
			client.SetHeader("x-portkey-provider", provider)
		}

		errorProvider = "portkey/" + provider
	}

	var wrapper Client
	wrapper.httpClient = client
	wrapper.errorProvider = errorProvider

	return &wrapper, nil
}

func (c *Client) CreateResponse(ctx context.Context, request *responses.CreateResponseRequest) (*responses.Response, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}

	var apiResponse responses.Response
	var apiErrBody struct {
		Error *responses.ResponseError `json:"error"`
	}

	httpResponse, err := c.httpClient.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResponse).
		SetError(&apiErrBody).
		Post("/v1/responses")
	if err != nil {
		return nil, fmt.Errorf("responses create response: %w", err)
	}

	if httpResponse.StatusCode() >= http.StatusBadRequest {
		return nil, responses.BuildAPIError(c.errorProvider, httpResponse.StatusCode(), apiErrBody.Error)
	}

	return &apiResponse, nil
}
