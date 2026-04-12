package chat

import (
	"fmt"
	"strings"

	"mangoduck/internal/llm/responses"
)

func buildTools(enableCronTools bool, enableXSearch bool, enableWebSearch bool, runtime ToolRuntime) []*responses.Tool {
	tools := make([]*responses.Tool, 0, 7)

	if enableXSearch {
		tools = append(tools, buildXSearchFunctionTool())
	}
	if enableWebSearch {
		tools = append(tools, buildWebSearchFunctionTool())
	}

	tools = append(tools, buildMemoryGetFunctionTool(), buildMemorySetFunctionTool())

	if enableCronTools {
		tools = append(tools, buildListCronTasksFunctionTool(), buildAddCronTaskFunctionTool(), buildDeleteCronTaskFunctionTool())
	}

	if runtime != nil {
		tools = append(tools, runtime.Tools()...)
	}

	return tools
}

func buildXSearchFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = xSearchFunctionToolName
	tool.Description = "Search X and return a concise summary with citations when available."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Concrete non-empty search query for X. Do not use an empty placeholder.",
				"minLength":   1,
			},
			"allowed_x_handles": map[string]any{
				"type":        []string{"array", "null"},
				"description": "Only search within these X handles.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"excluded_x_handles": map[string]any{
				"type":        []string{"array", "null"},
				"description": "Exclude these X handles from search.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"from_date": map[string]any{
				"type":        []string{"string", "null"},
				"description": "Optional ISO-8601 start date or datetime.",
			},
			"to_date": map[string]any{
				"type":        []string{"string", "null"},
				"description": "Optional ISO-8601 end date or datetime.",
			},
			"enable_image_understanding": map[string]any{
				"type":        []string{"boolean", "null"},
				"description": "Allow understanding images attached to X posts.",
			},
			"enable_video_understanding": map[string]any{
				"type":        []string{"boolean", "null"},
				"description": "Allow understanding videos attached to X posts.",
			},
		},
		"required": []string{
			"query",
			"allowed_x_handles",
			"excluded_x_handles",
			"from_date",
			"to_date",
			"enable_image_understanding",
			"enable_video_understanding",
		},
	}
	tool.Strict = true

	return &tool
}

func buildWebSearchFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = webSearchFunctionToolName
	tool.Description = "Search the web and return a concise summary with citations when available."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"query": map[string]any{
				"type":        "string",
				"description": "Concrete non-empty search query for the web. Do not use an empty placeholder.",
				"minLength":   1,
			},
			"allowed_domains": map[string]any{
				"type":        []string{"array", "null"},
				"description": "Only search within these domains. Cannot be combined with excluded_domains.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"excluded_domains": map[string]any{
				"type":        []string{"array", "null"},
				"description": "Exclude these domains from search. Cannot be combined with allowed_domains.",
				"items": map[string]any{
					"type": "string",
				},
			},
			"enable_image_understanding": map[string]any{
				"type":        []string{"boolean", "null"},
				"description": "Allow understanding images from web pages when supported.",
			},
		},
		"required": []string{
			"query",
			"allowed_domains",
			"excluded_domains",
			"enable_image_understanding",
		},
	}
	tool.Strict = true

	return &tool
}

func buildMemoryGetFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = memoryGetFunctionToolName
	tool.Description = "Read the persisted memory text for this chat. Use this before memory-set when the user asks to remember something, forget something, or change custom behavior."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
		"required":             []string{},
	}
	tool.Strict = true

	return &tool
}

func buildMemorySetFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = memorySetFunctionToolName
	tool.Description = "Replace the persisted memory text for this chat. Before calling this tool, first call memory-get to inspect the latest saved memory whenever the user asks to remember something, forget something, or change custom behavior. Write the full updated memory text, not a diff. Use an empty text string to clear memory."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"text": map[string]any{
				"type":        "string",
				"description": "The full memory text that should be persisted for this chat.",
			},
		},
		"required": []string{
			"text",
		},
	}
	tool.Strict = true

	return &tool
}

func buildListCronTasksFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = listCronTasksToolName
	tool.Description = "List persisted cron tasks for the current chat."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           map[string]any{},
		"required":             []string{},
	}
	tool.Strict = true

	return &tool
}

func buildAddCronTaskFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = addCronTaskFunctionToolName
	tool.Description = "Create a persisted cron task for the current chat. Use this for recurring reminders, monitoring, checks, and reports. The task runs the provided prompt later in the same chat without using chat history, but still in agentic mode. Cron expressions use the local timezone."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"schedule": map[string]any{
				"type":        "string",
				"description": "Cron expression in standard 5-field format or descriptor such as @daily.",
			},
			"prompt": map[string]any{
				"type":        "string",
				"description": "Prompt to execute on every scheduled run.",
			},
		},
		"required": []string{
			"schedule",
			"prompt",
		},
	}
	tool.Strict = true

	return &tool
}

func buildDeleteCronTaskFunctionTool() *responses.Tool {
	var tool responses.Tool
	tool.Type = "function"
	tool.Name = deleteCronTaskToolName
	tool.Description = "Delete a persisted cron task by ID for the current chat."
	tool.Parameters = map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"task_id": map[string]any{
				"type":        "integer",
				"description": "The cron task ID to delete.",
			},
		},
		"required": []string{
			"task_id",
		},
	}
	tool.Strict = true

	return &tool
}

func buildToolCallStatusText(call *responses.FunctionCall) string {
	if call == nil {
		return "Running a tool..."
	}

	name := strings.TrimSpace(call.Name)
	switch name {
	case xSearchFunctionToolName:
		return buildXSearchStatusText(call.Arguments)
	case webSearchFunctionToolName:
		return buildWebSearchStatusText(call.Arguments)
	case memoryGetFunctionToolName:
		return "Reading saved memory..."
	case memorySetFunctionToolName:
		return buildMemoryStatusText(call.Arguments)
	case listCronTasksToolName:
		return "Listing scheduled tasks..."
	case addCronTaskFunctionToolName:
		return buildAddCronTaskStatusText(call.Arguments)
	case deleteCronTaskToolName:
		return buildDeleteCronTaskStatusText(call.Arguments)
	default:
		return buildGenericToolStatusText(name)
	}
}

func buildXSearchStatusText(arguments string) string {
	request, err := parseXSearchRequest(arguments)
	if err != nil {
		return "Searching on X..."
	}

	query := summarizeStatusText(request.Query, 80)
	if query == "" {
		return "Searching on X..."
	}

	return "Searching on X for: " + query
}

func buildWebSearchStatusText(arguments string) string {
	request, err := parseWebSearchRequest(arguments)
	if err != nil {
		return "Searching the web..."
	}

	query := summarizeStatusText(request.Query, 80)
	if query == "" {
		return "Searching the web..."
	}

	return "Searching the web for: " + query
}

func buildMemoryStatusText(arguments string) string {
	payload, err := parseSetMemoryArguments(arguments)
	if err != nil {
		return "Updating saved memory..."
	}
	if payload.Text == "" {
		return "Clearing saved memory..."
	}

	return "Updating saved memory..."
}

func buildAddCronTaskStatusText(arguments string) string {
	payload, err := parseAddCronTaskArguments(arguments)
	if err != nil {
		return "Creating scheduled task..."
	}

	schedule := summarizeStatusText(payload.Schedule, 60)
	if schedule == "" {
		return "Creating scheduled task..."
	}

	return fmt.Sprintf("Creating scheduled task (%s)...", schedule)
}

func buildDeleteCronTaskStatusText(arguments string) string {
	taskID, err := parseDeleteCronTaskArguments(arguments)
	if err != nil {
		return "Deleting scheduled task..."
	}

	return fmt.Sprintf("Deleting scheduled task #%d...", taskID)
}

func buildGenericToolStatusText(name string) string {
	friendlyName := summarizeStatusText(humanizeToolName(name), 80)
	if friendlyName == "" {
		return "Running a tool..."
	}

	return fmt.Sprintf("Calling tool: %s...", friendlyName)
}

func humanizeToolName(name string) string {
	replacer := strings.NewReplacer("__", " ", "-", " ", "_", " ")
	name = replacer.Replace(strings.TrimSpace(name))
	name = strings.Join(strings.Fields(name), " ")

	return name
}

func summarizeStatusText(text string, maxRunes int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" || maxRunes <= 0 {
		return ""
	}

	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}

	if maxRunes <= 3 {
		return string(runes[:maxRunes])
	}

	return string(runes[:maxRunes-3]) + "..."
}
