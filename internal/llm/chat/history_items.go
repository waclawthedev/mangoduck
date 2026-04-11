package chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"mangoduck/internal/llm/responses"
)

func buildSystemMessageItem(message string) (json.RawMessage, error) {
	return buildMessageItem("system", message, nil)
}

func buildUserMessageItem(message string, image *InputImage) (json.RawMessage, error) {
	return buildMessageItem("user", message, image)
}

func buildMessageItem(role string, message string, image *InputImage) (json.RawMessage, error) {
	content := make([]map[string]any, 0, 2)

	message = strings.TrimSpace(message)
	if message != "" {
		content = append(content, map[string]any{
			"type": "input_text",
			"text": message,
		})
	}

	if hasInputImage(image) {
		content = append(content, map[string]any{
			"type":      "input_image",
			"image_url": buildImageDataURL(image),
		})
	}

	payload := map[string]any{
		"type":    "message",
		"role":    role,
		"content": content,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal user message item: %w", err)
	}

	return raw, nil
}

func buildImageDataURL(image *InputImage) string {
	if !hasInputImage(image) {
		return ""
	}

	return "data:" + strings.TrimSpace(image.MIMEType) + ";base64," + strings.TrimSpace(image.DataBase64)
}

func buildFunctionCallOutputItem(callID, output string) (json.RawMessage, error) {
	payload := map[string]any{
		"type":    "function_call_output",
		"call_id": callID,
		"output":  output,
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal function call output item: %w", err)
	}

	return raw, nil
}

func cloneRawMessages(items []json.RawMessage) []json.RawMessage {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]json.RawMessage, 0, len(items))
	for _, item := range items {
		cloned = append(cloned, append(json.RawMessage(nil), item...))
	}

	return cloned
}

func buildAssistantMessageItem(text string) (json.RawMessage, error) {
	payload := map[string]any{
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{
				"type": "output_text",
				"text": text,
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal assistant message item: %w", err)
	}

	return raw, nil
}

func buildFunctionCallItem(call *responses.FunctionCall) (json.RawMessage, error) {
	if call == nil {
		return nil, errors.New("chat function call is nil")
	}

	payload := map[string]any{
		"type":      "function_call",
		"call_id":   strings.TrimSpace(call.CallID),
		"name":      strings.TrimSpace(call.Name),
		"arguments": strings.TrimSpace(call.Arguments),
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal function call item: %w", err)
	}

	return raw, nil
}

func buildHistoryItems(response *responses.Response) ([]json.RawMessage, error) {
	if response == nil {
		return nil, nil
	}

	items := make([]json.RawMessage, 0, 2)

	text := strings.TrimSpace(response.OutputText())
	if text != "" {
		assistantItem, err := buildAssistantMessageItem(text)
		if err != nil {
			return nil, err
		}

		items = append(items, assistantItem)
	}

	functionCalls := response.FunctionCalls()
	if len(functionCalls) == 0 {
		return items, nil
	}

	for _, call := range functionCalls {
		functionCallItem, err := buildFunctionCallItem(call)
		if err != nil {
			return nil, err
		}

		items = append(items, functionCallItem)
	}

	return items, nil
}
