package chat

import (
	"strings"
	"time"
)

func buildSystemPrompt(isAdmin bool, enableCronTools bool, isScheduled bool, memoryText string) string {
	var builder strings.Builder
	builder.WriteString(telegramHTMLPrompt)
	appendSystemMemorySection(&builder, memoryText)
	builder.WriteString(" Current local time: ")
	builder.WriteString(time.Now().Format(time.RFC3339))
	builder.WriteString(". Local timezone: ")
	builder.WriteString(time.Now().Location().String())
	builder.WriteString(".")

	if isScheduled {
		builder.WriteString(" This request is a scheduled cron execution. Execute the saved task now using the available search tools when needed. Do not explain scheduling limitations, do not ask the user to set up cron, background jobs, or integrations, and do not ask clarifying follow-up questions. Treat the saved prompt as an instruction for what result to deliver in this run.")
	} else {
		builder.WriteString(" For normal chat, if the latest user request is ambiguous, underspecified, or missing a critical detail, ask one short clarifying question before taking action or making tool calls.")
		builder.WriteString(" Never call x-search or web-search with an empty, placeholder, or overly vague query; ask the user for the missing detail first.")
	}

	builder.WriteString(" If the user asks you to remember something, forget something, or change custom behavior for this chat, first call memory-get, then call memory-set with the full updated memory text.")

	if isAdmin {
		builder.WriteString(" The current user is an active admin.")
	} else {
		builder.WriteString(" The current user is not an admin.")
	}

	if enableCronTools {
		builder.WriteString(" Cron task tools are available. Use them when the user asks to list, create, or delete scheduled tasks. When creating a cron task, write the saved prompt as a standalone execution instruction for future runs, not as a restatement of the user's scheduling request. For any cron task that plans or executes multi-step work, require the saved prompt to spell out a clear step-by-step execution plan so each scheduled run can follow an obvious path to a concrete result with minimal ambiguity. Ensure the saved prompt explicitly requires the final answer to be Telegram-compatible HTML. Cron expressions use the local timezone shown above.")
	}

	return builder.String()
}

func appendSystemMemorySection(builder *strings.Builder, memoryText string) {
	if builder == nil {
		return
	}

	memoryText = strings.TrimSpace(memoryText)
	if memoryText == "" {
		return
	}

	builder.WriteString(" Persisted chat memory for this chat is provided below. Use it only when relevant and treat it as durable chat-specific preferences or facts.")
	builder.WriteString("\n[chat-memory]\n")
	builder.WriteString(memoryText)
	builder.WriteString("\n[/chat-memory]")
}
