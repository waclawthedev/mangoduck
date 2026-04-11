package searchx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/logging"

	"go.uber.org/zap"
)

const (
	DefaultModel = "grok-4-1-fast-reasoning"
	xSearchType  = "x_search"
)

var ErrMissingQuery = errors.New("search x query is required")

type Service struct {
	client responses.ResponseCreator
	model  string
	logger *zap.Logger
}

type Option func(*Service)

type SearchRequest struct {
	Query                    string
	AllowedXHandles          []string
	ExcludedXHandles         []string
	FromDate                 string
	ToDate                   string
	EnableImageUnderstanding bool
	EnableVideoUnderstanding bool
}

type SearchResult struct {
	ResponseID string
	Model      string
	Text       string
}

func NewService(client responses.ResponseCreator, model string, options ...Option) (*Service, error) {
	if client == nil {
		return nil, errors.New("search x client is required")
	}

	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}

	var service Service
	service.client = client
	service.model = model
	service.logger = zap.NewNop()

	for _, option := range options {
		if option == nil {
			continue
		}

		option(&service)
	}

	return &service, nil
}

func WithLogger(logger *zap.Logger) Option {
	return func(service *Service) {
		if service == nil {
			return
		}

		service.logger = logging.WithComponent(logger, "searchx")
	}
}

func (s *Service) Search(ctx context.Context, request *SearchRequest) (*SearchResult, error) {
	s.logger.Debug("validating X search request")
	if err := validateRequest(request); err != nil {
		s.logger.Error("X search request validation failed", zap.Error(err))
		return nil, err
	}

	s.logger.Debug("X search request accepted", zap.String("query", strings.TrimSpace(request.Query)), zap.Int("allowed_handles", len(request.AllowedXHandles)), zap.Int("excluded_handles", len(request.ExcludedXHandles)), zap.String("from_date", strings.TrimSpace(request.FromDate)), zap.String("to_date", strings.TrimSpace(request.ToDate)))

	var responseRequest responses.CreateResponseRequest
	responseRequest.Model = s.model
	s.logger.Debug("building X search prompt")
	responseRequest.Input = buildPrompt(request)
	s.logger.Debug("building X search tool payload")
	responseRequest.Tools = []*responses.Tool{
		buildTool(request),
	}
	responseRequest.ToolChoice = "required"

	s.logger.Debug("sending X search request to responses API", zap.String("model", responseRequest.Model), zap.String("input_type", fmt.Sprintf("%T", responseRequest.Input)), zap.Int("input_len", inputTextLen(responseRequest.Input)), zap.Int("tools", len(responseRequest.Tools)))
	response, err := s.client.CreateResponse(ctx, &responseRequest)
	if err != nil {
		s.logger.Error("X search create response failed", zap.Error(err))
		return nil, fmt.Errorf("search x create response: %w", err)
	}

	var result SearchResult
	result.ResponseID = response.ID
	result.Model = response.Model
	result.Text = strings.TrimSpace(response.OutputText())

	s.logger.Debug("X search response received", zap.String("response_id", result.ResponseID), zap.String("model", result.Model), zap.Int("text_len", len(result.Text)))

	return &result, nil
}

func validateRequest(request *SearchRequest) error {
	if request == nil {
		return ErrMissingQuery
	}

	if strings.TrimSpace(request.Query) == "" {
		return ErrMissingQuery
	}

	if err := validateOptionalDate(request.FromDate); err != nil {
		return fmt.Errorf("search x from_date: %w", err)
	}

	if err := validateOptionalDate(request.ToDate); err != nil {
		return fmt.Errorf("search x to_date: %w", err)
	}

	return nil
}

func validateOptionalDate(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}

	_, dateErr := time.Parse(time.DateOnly, value)
	if dateErr == nil {
		return nil
	}

	_, dateTimeErr := time.Parse(time.RFC3339, value)
	if dateTimeErr == nil {
		return nil
	}

	return fmt.Errorf("must be ISO-8601 date or datetime: %q", value)
}

func buildPrompt(request *SearchRequest) string {
	var builder strings.Builder
	builder.WriteString("Search X for the user's request and return a concise summary with inline citations when available.\n\n")
	builder.WriteString("User query: ")
	builder.WriteString(strings.TrimSpace(request.Query))

	return builder.String()
}

func buildTool(request *SearchRequest) *responses.Tool {
	var tool responses.Tool
	tool.Type = xSearchType
	tool.AllowedXHandles = cloneStrings(request.AllowedXHandles)
	tool.ExcludedXHandles = cloneStrings(request.ExcludedXHandles)
	tool.FromDate = strings.TrimSpace(request.FromDate)
	tool.ToDate = strings.TrimSpace(request.ToDate)
	tool.EnableImageUnderstanding = request.EnableImageUnderstanding
	tool.EnableVideoUnderstanding = request.EnableVideoUnderstanding

	return &tool
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	cloned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		cloned = append(cloned, trimmed)
	}

	if len(cloned) == 0 {
		return nil
	}

	return cloned
}

func inputTextLen(value any) int {
	input, ok := value.(string)
	if !ok {
		return 0
	}

	return len(input)
}
