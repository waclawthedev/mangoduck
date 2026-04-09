package websearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"mangoduck/internal/llm/responses"
	"mangoduck/internal/logging"

	"go.uber.org/zap"
)

const (
	DefaultModel  = "gpt-5.4-nano"
	webSearchType = "web_search"
	maxDomains    = 5
)

var (
	ErrMissingQuery            = errors.New("web search query is required")
	ErrConflictingDomainFilter = errors.New("web search cannot set both allowed_domains and excluded_domains")
)

type Service struct {
	client responses.Client
	model  string
	logger *zap.Logger
}

type Option func(*Service)

type SearchRequest struct {
	Query                    string
	AllowedDomains           []string
	ExcludedDomains          []string
	EnableImageUnderstanding bool
}

type SearchResult struct {
	ResponseID string
	Model      string
	Text       string
}

func NewService(client responses.Client, model string, options ...Option) (*Service, error) {
	if client == nil {
		return nil, errors.New("web search client is required")
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

		service.logger = logging.WithComponent(logger, "websearch")
	}
}

func (s *Service) Search(ctx context.Context, request *SearchRequest) (*SearchResult, error) {
	s.logger.Debug("validating web search request")
	if err := validateRequest(request); err != nil {
		s.logger.Error("web search request validation failed", zap.Error(err))
		return nil, err
	}

	s.logger.Debug("web search request accepted", zap.String("query", strings.TrimSpace(request.Query)), zap.Int("allowed_domains", len(request.AllowedDomains)), zap.Int("excluded_domains", len(request.ExcludedDomains)), zap.Bool("image_understanding", request.EnableImageUnderstanding))

	var responseRequest responses.CreateResponseRequest
	responseRequest.Model = s.model
	s.logger.Debug("building web search prompt")
	responseRequest.Input = buildPrompt(request)
	s.logger.Debug("building web search tool payload")
	responseRequest.Tools = []*responses.Tool{
		buildTool(request),
	}
	responseRequest.ToolChoice = "required"

	s.logger.Debug("sending web search request to responses API", zap.String("model", responseRequest.Model), zap.String("input_type", fmt.Sprintf("%T", responseRequest.Input)), zap.Int("input_len", inputTextLen(responseRequest.Input)), zap.Int("tools", len(responseRequest.Tools)))
	response, err := s.client.CreateResponse(ctx, &responseRequest)
	if err != nil {
		s.logger.Error("web search create response failed", zap.Error(err))
		return nil, fmt.Errorf("web search create response: %w", err)
	}

	var result SearchResult
	result.ResponseID = response.ID
	result.Model = response.Model
	result.Text = strings.TrimSpace(response.OutputText())

	s.logger.Debug("web search response received", zap.String("response_id", result.ResponseID), zap.String("model", result.Model), zap.Int("text_len", len(result.Text)))

	return &result, nil
}

func validateRequest(request *SearchRequest) error {
	if request == nil {
		return ErrMissingQuery
	}

	if strings.TrimSpace(request.Query) == "" {
		return ErrMissingQuery
	}

	allowedDomains := cloneStrings(request.AllowedDomains)
	excludedDomains := cloneStrings(request.ExcludedDomains)
	if len(allowedDomains) > 0 && len(excludedDomains) > 0 {
		return ErrConflictingDomainFilter
	}

	if len(allowedDomains) > maxDomains {
		return fmt.Errorf("web search allowed_domains supports at most %d domains", maxDomains)
	}

	if len(excludedDomains) > maxDomains {
		return fmt.Errorf("web search excluded_domains supports at most %d domains", maxDomains)
	}

	return nil
}

func buildPrompt(request *SearchRequest) string {
	var builder strings.Builder
	builder.WriteString("Search the web for the user's request and return a concise summary with inline citations when available.\n\n")
	builder.WriteString("User query: ")
	builder.WriteString(strings.TrimSpace(request.Query))

	return builder.String()
}

func buildTool(request *SearchRequest) *responses.Tool {
	var tool responses.Tool
	tool.Type = webSearchType
	tool.EnableImageUnderstanding = request.EnableImageUnderstanding

	allowedDomains := cloneStrings(request.AllowedDomains)
	excludedDomains := cloneStrings(request.ExcludedDomains)
	if len(allowedDomains) > 0 || len(excludedDomains) > 0 {
		var filters responses.Filters
		filters.AllowedDomains = allowedDomains
		filters.ExcludedDomains = excludedDomains
		tool.Filters = &filters
	}

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
