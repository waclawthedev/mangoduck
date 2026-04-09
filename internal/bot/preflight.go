package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"mangoduck/internal/config"
	"mangoduck/internal/llm/chat"
	"mangoduck/internal/llm/responses"
	"mangoduck/internal/llm/searchx"
	"mangoduck/internal/llm/websearch"
	"mangoduck/internal/logging"
)

const (
	startupPreflightPrompt     = "Reply with OK only."
	maxStartupPreflightTimeout = 15 * time.Second
)

type startupMCPChecker interface {
	Preflight(ctx context.Context) error
}

func runStartupPreflight(
	ctx context.Context,
	cfg config.Config,
	logger *zap.Logger,
	mainClient responses.Client,
	xaiClient responses.Client,
	openAIWebSearchClient responses.Client,
	mcpChecker startupMCPChecker,
) error {
	logger = logging.WithComponent(logger, "preflight")
	logger.Info("startup preflight started")

	mainModel := resolveMainModel(cfg.MainModel)
	err := checkResponsesAPI(ctx, mainClient, "responses api", mainModel, cfg.ResponsesTimeout)
	if err != nil {
		return err
	}
	logger.Info("responses api preflight passed", zap.String("model", mainModel))

	if cfg.XSearchEnabled() {
		xaiModel := resolveXAIModel(cfg.XAIModel)
		err = checkResponsesAPI(ctx, xaiClient, "xai api", xaiModel, cfg.XAITimeout)
		if err != nil {
			return err
		}
		logger.Info("xai api preflight passed", zap.String("model", xaiModel))
	} else {
		logger.Info("xai api preflight skipped because x-search is disabled")
	}

	if cfg.OpenAIWebSearchEnabled() {
		webSearchModel := resolveOpenAIWebSearchModel(cfg.OpenAIWebSearchModel)
		err = checkResponsesAPI(ctx, openAIWebSearchClient, "openai web search api", webSearchModel, cfg.OpenAIWebSearchTimeout)
		if err != nil {
			return err
		}
		logger.Info("openai web search api preflight passed", zap.String("model", webSearchModel))
	} else {
		logger.Info("openai web search api preflight skipped because web-search is disabled")
	}

	if mcpChecker != nil {
		mcpCtx, cancel := context.WithTimeout(ctx, clampStartupPreflightTimeout(cfg.ResponsesTimeout))
		defer cancel()

		err = mcpChecker.Preflight(mcpCtx)
		if err != nil {
			return fmt.Errorf("mcp preflight failed: %w", err)
		}

		logger.Info("mcp preflight passed")
	}

	logger.Info("startup preflight completed")

	return nil
}

func checkResponsesAPI(ctx context.Context, client responses.Client, target string, model string, timeout time.Duration) error {
	if client == nil {
		return fmt.Errorf("%s client is required", target)
	}

	checkCtx, cancel := context.WithTimeout(ctx, clampStartupPreflightTimeout(timeout))
	defer cancel()

	var request responses.CreateResponseRequest
	request.Model = model
	request.Input = startupPreflightPrompt

	storeDisabled := false
	request.Store = &storeDisabled

	response, err := client.CreateResponse(checkCtx, &request)
	if err != nil {
		return fmt.Errorf("%s preflight request failed for model %q: %w", target, model, err)
	}
	if response == nil {
		return fmt.Errorf("%s preflight returned nil response for model %q", target, model)
	}

	return nil
}

func resolveMainModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return chat.DefaultModel
	}

	return model
}

func resolveXAIModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return searchx.DefaultModel
	}

	return model
}

func resolveOpenAIWebSearchModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return websearch.DefaultModel
	}

	return model
}

func clampStartupPreflightTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return maxStartupPreflightTimeout
	}
	if timeout > maxStartupPreflightTimeout {
		return maxStartupPreflightTimeout
	}

	return timeout
}

func combineStartupError(base string, err error) error {
	if err == nil {
		return errors.New(base)
	}

	return fmt.Errorf("%s: %w", base, err)
}
