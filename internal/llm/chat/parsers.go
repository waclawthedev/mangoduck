package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mangoduck/internal/llm/searchx"
	"mangoduck/internal/llm/websearch"
)

func parseXSearchRequest(arguments string) (*searchx.SearchRequest, error) {
	var payload struct {
		Query                    string   `json:"query"`
		AllowedXHandles          []string `json:"allowed_x_handles"`
		ExcludedXHandles         []string `json:"excluded_x_handles"`
		FromDate                 string   `json:"from_date"`
		ToDate                   string   `json:"to_date"`
		EnableImageUnderstanding bool     `json:"enable_image_understanding"`
		EnableVideoUnderstanding bool     `json:"enable_video_understanding"`
	}

	err := json.Unmarshal([]byte(arguments), &payload)
	if err != nil {
		return nil, fmt.Errorf("parse x-search function arguments: %w", err)
	}

	var request searchx.SearchRequest
	request.Query = payload.Query
	request.AllowedXHandles = payload.AllowedXHandles
	request.ExcludedXHandles = payload.ExcludedXHandles
	request.FromDate = payload.FromDate
	request.ToDate = payload.ToDate
	request.EnableImageUnderstanding = payload.EnableImageUnderstanding
	request.EnableVideoUnderstanding = payload.EnableVideoUnderstanding

	return &request, nil
}

func parseWebSearchRequest(arguments string) (*websearch.SearchRequest, error) {
	var payload struct {
		Query                    string   `json:"query"`
		AllowedDomains           []string `json:"allowed_domains"`
		ExcludedDomains          []string `json:"excluded_domains"`
		EnableImageUnderstanding bool     `json:"enable_image_understanding"`
	}

	err := json.Unmarshal([]byte(arguments), &payload)
	if err != nil {
		return nil, fmt.Errorf("parse web-search function arguments: %w", err)
	}

	var request websearch.SearchRequest
	request.Query = payload.Query
	request.AllowedDomains = payload.AllowedDomains
	request.ExcludedDomains = payload.ExcludedDomains
	request.EnableImageUnderstanding = payload.EnableImageUnderstanding

	return &request, nil
}

type setMemoryArguments struct {
	Text string `json:"text"`
}

func parseSetMemoryArguments(arguments string) (*setMemoryArguments, error) {
	var payload setMemoryArguments
	err := json.Unmarshal([]byte(arguments), &payload)
	if err != nil {
		return nil, fmt.Errorf("parse memory-set function arguments: %w", err)
	}

	payload.Text = strings.TrimSpace(payload.Text)

	return &payload, nil
}

type addCronTaskArguments struct {
	Schedule string `json:"schedule"`
	Prompt   string `json:"prompt"`
}

func parseAddCronTaskArguments(arguments string) (*addCronTaskArguments, error) {
	var payload addCronTaskArguments
	err := json.Unmarshal([]byte(arguments), &payload)
	if err != nil {
		return nil, fmt.Errorf("parse add-cron-task function arguments: %w", err)
	}

	payload.Schedule = strings.TrimSpace(payload.Schedule)
	payload.Prompt = strings.TrimSpace(payload.Prompt)
	if payload.Schedule == "" {
		return nil, errors.New("add-cron-task schedule is required")
	}
	if payload.Prompt == "" {
		return nil, errors.New("add-cron-task prompt is required")
	}
	payload.Prompt = ensureCronTaskPromptHTML(payload.Prompt)

	return &payload, nil
}

func ensureCronTaskPromptHTML(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}

	if strings.Contains(strings.ToLower(prompt), strings.ToLower(cronTaskHTMLInstruction)) {
		return prompt
	}

	return prompt + "\n\n" + cronTaskHTMLInstruction
}

func parseDeleteCronTaskArguments(arguments string) (int64, error) {
	var payload struct {
		TaskID int64 `json:"task_id"`
	}

	err := json.Unmarshal([]byte(arguments), &payload)
	if err != nil {
		return 0, fmt.Errorf("parse delete-cron-task function arguments: %w", err)
	}
	if payload.TaskID <= 0 {
		return 0, errors.New("delete-cron-task task_id must be positive")
	}

	return payload.TaskID, nil
}
