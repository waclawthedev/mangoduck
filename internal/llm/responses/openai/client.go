package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mangoduck/internal/llm/responses"

	"resty.dev/v3"
)

const (
	defaultBaseURL = "https://api.openai.com"
	providerName   = "openai"
)

type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

type Client struct {
	httpClient *resty.Client
}

func NewClient(cfg Config) (*Client, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, responses.ErrMissingAPIKey
	}

	client := resty.New()
	client.SetBaseURL(baseURL)
	client.SetTimeout(cfg.Timeout)
	client.SetHeader("Content-Type", "application/json")
	client.SetHeader("Accept", "application/json")
	client.SetHeader("Authorization", "Bearer "+apiKey)

	var wrapper Client
	wrapper.httpClient = client

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
		return nil, responses.BuildAPIError(providerName, httpResponse.StatusCode(), apiErrBody.Error)
	}

	return &apiResponse, nil
}
