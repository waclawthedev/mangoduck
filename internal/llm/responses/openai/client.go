package openai

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.uber.org/zap"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/logging"
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
	client  openaisdk.Client
	timeout time.Duration
	logger  *zap.Logger
}

type Option func(*Client)

func NewClient(cfg Config, options ...Option) (*Client, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, responses.ErrMissingAPIKey
	}

	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}

	client := openaisdk.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(baseURL+"/v1"),
	)

	var wrapper Client
	wrapper.client = client
	wrapper.timeout = cfg.Timeout
	wrapper.logger = zap.NewNop()

	for _, apply := range options {
		if apply == nil {
			continue
		}

		apply(&wrapper)
	}

	return &wrapper, nil
}

func WithLogger(logger *zap.Logger) Option {
	return func(client *Client) {
		if client == nil {
			return
		}

		client.logger = logging.WithComponent(logger, "openai")
	}
}

func (c *Client) CreateResponse(ctx context.Context, request *responses.CreateResponseRequest) (*responses.Response, error) {
	if err := request.Validate(); err != nil {
		c.logger.Error("OpenAI request validation failed", zap.Error(err))
		return nil, err
	}

	c.logger.Debug("sending OpenAI responses request", zap.String("model", request.Model), zap.String("input_type", fmt.Sprintf("%T", request.Input)), zap.Int("input_len", inputTextLen(request.Input)), zap.Int("tools", len(request.Tools)))

	var apiResponse responses.Response
	opts := make([]option.RequestOption, 0, 1)
	if c.timeout > 0 {
		opts = append(opts, option.WithRequestTimeout(c.timeout))
	}

	err := c.client.Post(ctx, "/responses", request, &apiResponse, opts...)
	if err != nil {
		var apiErr *openaisdk.Error
		if errors.As(err, &apiErr) {
			c.logger.Error("OpenAI API returned error", zap.Int("status", apiErr.StatusCode), zap.String("message", strings.TrimSpace(apiErr.Message)))

			var responseError responses.ResponseError
			responseError.Message = strings.TrimSpace(apiErr.Message)
			responseError.Type = strings.TrimSpace(apiErr.Type)
			responseError.Code = strings.TrimSpace(apiErr.Code)

			return nil, responses.BuildAPIError(providerName, apiErr.StatusCode, &responseError)
		}

		c.logger.Error("OpenAI request failed", zap.Error(err))
		return nil, fmt.Errorf("openai create response: %w", err)
	}

	c.logger.Debug("OpenAI response received", zap.String("response_id", apiResponse.ID), zap.String("model", apiResponse.Model))

	return &apiResponse, nil
}

func inputTextLen(value any) int {
	input, ok := value.(string)
	if !ok {
		return 0
	}

	return len(input)
}
