package xai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/logging"

	"go.uber.org/zap"
	"resty.dev/v3"
)

const defaultBaseURL = "https://api.x.ai"

type Config struct {
	APIKey  string
	BaseURL string
	Timeout time.Duration
}

type Client struct {
	httpClient *resty.Client
	logger     *zap.Logger
}

type Option func(*Client)

func NewClient(cfg Config, options ...Option) (*Client, error) {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil, responses.ErrMissingAPIKey
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	client := resty.New()
	client.SetBaseURL(baseURL)
	client.SetTimeout(cfg.Timeout)
	client.SetHeader("Authorization", "Bearer "+cfg.APIKey)
	client.SetHeader("Content-Type", "application/json")
	client.SetHeader("Accept", "application/json")

	var wrapper Client
	wrapper.httpClient = client
	wrapper.logger = zap.NewNop()

	for _, option := range options {
		if option == nil {
			continue
		}

		option(&wrapper)
	}

	return &wrapper, nil
}

func WithLogger(logger *zap.Logger) Option {
	return func(client *Client) {
		if client == nil {
			return
		}

		client.logger = logging.WithComponent(logger, "xai")
	}
}

func (c *Client) CreateResponse(ctx context.Context, request *responses.CreateResponseRequest) (*responses.Response, error) {
	if err := request.Validate(); err != nil {
		c.logger.Error("xAI request validation failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("sending xAI responses request", zap.String("model", request.Model), zap.String("input_type", fmt.Sprintf("%T", request.Input)), zap.Int("input_len", inputTextLen(request.Input)), zap.Int("tools", len(request.Tools)))

	var apiResponse responses.Response
	var apiErrBody responseErrorEnvelope

	httpResponse, err := c.httpClient.R().
		SetContext(ctx).
		SetBody(request).
		SetResult(&apiResponse).
		SetError(&apiErrBody).
		Post("/v1/responses")
	if err != nil {
		c.logger.Error("xAI request failed", zap.Error(err))
		return nil, fmt.Errorf("xai create response: %w", err)
	}

	if httpResponse.StatusCode() >= http.StatusBadRequest {
		c.logger.Error("xAI API returned error", zap.Int("status", httpResponse.StatusCode()), zap.String("message", strings.TrimSpace(apiErrBody.Message)))
		return nil, buildAPIError(httpResponse.StatusCode(), apiErrBody.AsResponseError())
	}

	c.logger.Debug("xAI response received", zap.Int("status", httpResponse.StatusCode()), zap.String("response_id", apiResponse.ID), zap.String("model", apiResponse.Model))

	return &apiResponse, nil
}

type responseErrorEnvelope struct {
	Error   any    `json:"error"`
	Message string `json:"message,omitempty"`
}

func (e *responseErrorEnvelope) AsResponseError() *responses.ResponseError {
	if e == nil {
		return nil
	}

	switch value := e.Error.(type) {
	case nil:
		if strings.TrimSpace(e.Message) == "" {
			return nil
		}

		var responseError responses.ResponseError
		responseError.Message = strings.TrimSpace(e.Message)

		return &responseError
	case string:
		var responseError responses.ResponseError
		responseError.Message = strings.TrimSpace(value)

		return &responseError
	case map[string]any:
		payload, err := json.Marshal(value)
		if err != nil {
			break
		}

		var responseError responses.ResponseError
		err = json.Unmarshal(payload, &responseError)
		if err != nil {
			break
		}

		return &responseError
	}

	if strings.TrimSpace(e.Message) == "" {
		return nil
	}

	var responseError responses.ResponseError
	responseError.Message = strings.TrimSpace(e.Message)

	return &responseError
}

func buildAPIError(statusCode int, responseError *responses.ResponseError) error {
	return responses.BuildAPIError("xai", statusCode, responseError)
}

func inputTextLen(value any) int {
	input, ok := value.(string)
	if !ok {
		return 0
	}

	return len(input)
}
