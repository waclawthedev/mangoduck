package responses

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

var (
	ErrMissingAPIKey       = errors.New("responses api key is required")
	ErrMissingProviderAuth = errors.New("responses provider authentication is required")
	ErrMissingModel        = errors.New("responses model is required")
	ErrMissingInput        = errors.New("responses input is required")
)

type ResponseCreator interface {
	CreateResponse(ctx context.Context, request *CreateResponseRequest) (*Response, error)
}

type CreateResponseRequest struct {
	Model              string  `json:"model"`
	Input              any     `json:"input"`
	PreviousResponseID string  `json:"previous_response_id,omitempty"`
	Store              *bool   `json:"store,omitempty"`
	ParallelToolCalls  *bool   `json:"parallel_tool_calls,omitempty"`
	Tools              []*Tool `json:"tools,omitempty"`
	ToolChoice         any     `json:"tool_choice,omitempty"`
}

func (r *CreateResponseRequest) Validate() error {
	if r == nil {
		return ErrMissingInput
	}

	if strings.TrimSpace(r.Model) == "" {
		return ErrMissingModel
	}

	if r.Input == nil {
		return ErrMissingInput
	}

	return nil
}

type Response struct {
	ID     string         `json:"id"`
	Object string         `json:"object"`
	Model  string         `json:"model"`
	Status string         `json:"status"`
	Output []*OutputItem  `json:"output,omitempty"`
	Error  *ResponseError `json:"error,omitempty"`
}

func (r *Response) OutputText() string {
	if r == nil {
		return ""
	}

	var builder strings.Builder

	for _, item := range r.Output {
		if item == nil {
			continue
		}

		if item.Type == "output_text" && item.Text != "" {
			builder.WriteString(item.Text)
		}

		for _, content := range item.Content {
			if content == nil || content.Text == "" {
				continue
			}

			builder.WriteString(content.Text)
		}
	}

	return builder.String()
}

type OutputItem struct {
	ID        string          `json:"id,omitempty"`
	Type      string          `json:"type"`
	Role      string          `json:"role,omitempty"`
	Status    string          `json:"status,omitempty"`
	Text      string          `json:"text,omitempty"`
	Content   []*OutputText   `json:"content,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Arguments string          `json:"arguments,omitempty"`
	Raw       json.RawMessage `json:"-"`
}

func (o *OutputItem) UnmarshalJSON(data []byte) error {
	type alias OutputItem

	var payload alias
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	*o = OutputItem(payload)
	o.Raw = append(o.Raw[:0], data...)

	return nil
}

type OutputText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type Tool struct {
	Type                     string   `json:"type"`
	Name                     string   `json:"name,omitempty"`
	Description              string   `json:"description,omitempty"`
	Parameters               any      `json:"parameters,omitempty"`
	Strict                   bool     `json:"strict,omitempty"`
	Filters                  *Filters `json:"filters,omitempty"`
	AllowedXHandles          []string `json:"allowed_x_handles,omitempty"`
	ExcludedXHandles         []string `json:"excluded_x_handles,omitempty"`
	FromDate                 string   `json:"from_date,omitempty"`
	ToDate                   string   `json:"to_date,omitempty"`
	EnableImageUnderstanding bool     `json:"enable_image_understanding,omitempty"`
	EnableVideoUnderstanding bool     `json:"enable_video_understanding,omitempty"`
}

type Filters struct {
	AllowedDomains  []string `json:"allowed_domains,omitempty"`
	ExcludedDomains []string `json:"excluded_domains,omitempty"`
}

type InputItem struct {
	Type    string `json:"type"`
	CallID  string `json:"call_id,omitempty"`
	Output  string `json:"output,omitempty"`
	Role    string `json:"role,omitempty"`
	Content any    `json:"content,omitempty"`
}

type FunctionCall struct {
	CallID    string
	Name      string
	Arguments string
}

func (r *Response) FunctionCalls() []*FunctionCall {
	if r == nil {
		return nil
	}

	calls := make([]*FunctionCall, 0, len(r.Output))
	for _, item := range r.Output {
		if item == nil || item.Type != "function_call" {
			continue
		}

		var call FunctionCall
		call.CallID = strings.TrimSpace(item.CallID)
		call.Name = strings.TrimSpace(item.Name)
		call.Arguments = strings.TrimSpace(item.Arguments)
		calls = append(calls, &call)
	}

	if len(calls) == 0 {
		return nil
	}

	return calls
}

type ResponseError struct {
	Message string `json:"message"`
	Type    string `json:"type,omitempty"`
	Code    any    `json:"code,omitempty"`
}

type APIError struct {
	StatusCode int
	Provider   string
	Message    string
	Type       string
	Code       any
}

func (e *APIError) Error() string {
	if e == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(e.Provider)
	builder.WriteString(" responses api error")

	if e.StatusCode > 0 {
		builder.WriteString(" (status ")
		builder.WriteString(strconv.Itoa(e.StatusCode))
		builder.WriteString(")")
	}

	if e.Message != "" {
		builder.WriteString(": ")
		builder.WriteString(e.Message)
	}

	return builder.String()
}

func BuildAPIError(provider string, statusCode int, responseError *ResponseError) error {
	var apiErr APIError
	apiErr.StatusCode = statusCode
	apiErr.Provider = provider

	if responseError != nil {
		apiErr.Message = responseError.Message
		apiErr.Type = responseError.Type
		apiErr.Code = responseError.Code
	}

	return &apiErr
}
