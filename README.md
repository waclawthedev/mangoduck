<p align="center">
  <img src="./logo.png" alt="Mangoduck logo" width="380">
</p>

<h1 align="center">Mangoduck</h1>

<p align="center">
  Agentic Telegram bot for private chats and group chats, with OpenAI Responses API, search tools, MCP integration, chat-scoped memory, and scheduled tasks.
</p>

> [!WARNING]
> This project is provided for research and experimental purposes only. It may contain critical bugs, security flaws, or other defects, and you run, use, deploy, and rely on it entirely at your own risk.

## Overview

Mangoduck is a Go Telegram bot built for a single user or small teams that collaborate in Telegram chats. It keeps access control simple: every conversation target is treated as a Telegram `chat`, and access is granted or denied only by `chat_id`.

Admin privileges are not stored in the database. They are derived directly from `config.yaml` under `admin.tg_ids`; every other user is treated as a regular user.

The bot runs in an agentic loop on top of the Responses API, can call built-in tools such as web search and memory management, and can expose external MCP tools during a chat turn. It stores normalized conversation items locally in SQLite and replays them per `chat_id` on each request, while keeping the provider side stateless for the main chat flow.

## Features

- Agentic Telegram chat flow with OpenAI-compatible Responses API.
- Works in private chats, groups, and supergroups.
- Mentions-only behavior in group chats to avoid noise.
- Chat-level access control managed by admins via `/chats`.
- Local SQLite persistence for chat history, per-chat memory, and cron tasks.
- Built-in tools for `x-search`, `web-search`, `memory-get`, `memory-set`, `list-cron-tasks`, `add-cron-task`, and `delete-cron-task`.
- Optional MCP integration for both `streamable_http` and local `stdio` servers.
- Startup preflight that validates the main Responses API and, when enabled, the xAI search API, the OpenAI web search API, and enabled MCP servers before polling starts.
- Scheduled prompts executed in the same agentic mode without replaying chat history.
- Sanitized runtime errors are reported back into Telegram chats without exposing credentials.

## How It Works

1. A Telegram update arrives.
2. The bot resolves the current Telegram `chat_id` and creates or refreshes a local chat record.
3. In groups and supergroups, the bot replies only if the message mentions the bot.
4. If the chat is inactive, the bot asks for approval and shows the chat ID.
5. When an admin activates a chat through `/chats`, the bot sends an approval message into that same Telegram chat.
6. If the chat is active, the bot replays locally stored normalized Responses items for that `chat_id`, injects per-chat memory, and sends a fresh stateless request to the model.
7. The model may answer directly or call exactly one tool in a step.
8. Tool results are stored locally as normalized items and fed back into the next model step.
9. The final assistant response is sent back to the same Telegram chat as Telegram-compatible HTML.
10. If a handler fails, the bot logs the full error server-side and sends a sanitized error summary back to Telegram.

## Commands

### User commands

- `/start` creates or refreshes the current chat record, checks whether the chat is approved, and replies with `Hi!` when access is active.
- `/clear_context` removes the locally persisted chat context for the current chat.

### Admin commands

- `/chats` opens the chat management panel with activate/deactivate actions, and it works only in a private chat with the bot.
- Activating an inactive chat sends an approval message into that chat.

Admins are defined only in `config.yaml` under `admin.tg_ids`.

## Architecture

### Main pieces

- `cmd/mangoduck` starts the process, loads config, opens SQLite, and runs the bot.
- `internal/bot` wires Telegram, startup preflight, command sync, and runtime services.
- `internal/llm/chat` contains the main agentic chat loop and tool orchestration.
- `internal/llm/responses` wraps the Responses API client.
- `internal/llm/searchx` provides the xAI-backed X search tool, and `internal/llm/websearch` provides the direct OpenAI-backed web search tool.
- `internal/mcpbridge` exposes MCP tools to the model during a run.
- `internal/cronjobs` restores and executes scheduled prompts.
- `internal/repo` stores chats, normalized input/output items, and cron tasks.
- `internal/db` opens SQLite and applies embedded migrations on startup.

### User message flow

```mermaid
flowchart TD
    A["Telegram user sends message"] --> B["telebot handler<br/>internal/telegram/conversation"]
    B --> C{"Private chat?"}
    C -- "Yes" --> E["Normalize input"]
    C -- "No, group/supergroup" --> D{"Bot mentioned?"}
    D -- "No" --> X["Ignore message"]
    D -- "Yes" --> E["Trim bot mention and normalize input"]
    E --> F["Ensure chat exists and is active"]
    F --> G["Build llmchat.Request<br/>chat_id, user_tg_id, text, is_admin"]
    G --> H["chat.Service.Reply()"]
    H --> I["Load persisted normalized history by chat_id"]
    I --> J["Load per-chat memory"]
    J --> K["Build system item + replayed history + current user item"]
    K --> L["Open MCP/tool runtime session"]
    L --> M["Build available tools:<br/>x-search, web-search, memory-get/set,<br/>cron tools, MCP tools"]
    M --> N["Responses API step"]
    N --> O["Persist current user item once"]
    O --> P["Normalize model output into local history items:<br/>assistant text and/or function_call"]
    P --> Q["Append normalized items to SQLite history"]
    Q --> R{"Function call returned?"}
    R -- "No" --> S["Return final Telegram HTML answer"]
    R -- "Yes, exactly one" --> T["Notify Telegram placeholder/status"]
    T --> U["Execute tool"]
    U --> V{"Built-in tool or MCP tool?"}
    V -- "Built-in" --> W["Search / memory / cron task action"]
    V -- "MCP" --> Y["runtime.Execute(name, arguments)"]
    W --> Z["Build function_call_output item"]
    Y --> Z["Build function_call_output item"]
    Z --> AA["Persist function_call_output to SQLite history"]
    AA --> AB["Append function_call_output to next Responses input"]
    AB --> AC{"Reached max steps?"}
    AC -- "No" --> N
    AC -- "Yes" --> AD["Fallback response"]
    S --> AE["Send reply or edit placeholder in Telegram"]
    AD --> AE
    X --> AF["End"]
    AE --> AF
```

### Cron execution flow

```mermaid
flowchart TD
    A["Process starts"] --> B["cronjobs.Service.Start()"]
    B --> C["Load persisted cron tasks from SQLite"]
    C --> D["Register each task in robfig/cron"]
    D --> E["Scheduler waits for next trigger"]
    E --> F["Cron trigger fires"]
    F --> G{"Same task already running?"}
    G -- "Yes" --> H["Skip overlapping run"]
    G -- "No" --> I["Mark task as running"]
    I --> J["executeScheduled(chat_id, prompt)"]
    J --> K["chat.Service.ExecuteScheduled()"]
    K --> L["Skip chat history replay and persistence"]
    L --> M["Load per-chat memory"]
    M --> N["Build system item + scheduled prompt as user item"]
    N --> O["Open MCP/tool runtime session"]
    O --> P["Build available tools:<br/>x-search, web-search, memory-get/set, MCP tools<br/>cron management tools disabled"]
    P --> Q["Responses API step"]
    Q --> R["Normalize response items in-memory only"]
    R --> S{"Function call returned?"}
    S -- "No" --> T["Return final Telegram HTML answer text"]
    S -- "Yes, exactly one" --> U["Execute tool"]
    U --> V{"Built-in tool or MCP tool?"}
    V -- "Built-in" --> W["Search / memory action"]
    V -- "MCP" --> X["runtime.Execute(name, arguments)"]
    W --> Y["Build function_call_output item in-memory"]
    X --> Y["Build function_call_output item in-memory"]
    Y --> Z{"Reached max steps?"}
    Z -- "No" --> Q
    Z -- "Yes" --> AA["Fallback response"]
    T --> AB{"Non-empty text?"}
    AA --> AB
    AB -- "No" --> AC["Do not send Telegram message"]
    AB -- "Yes" --> AD["Send result to Telegram chat"]
    H --> AE["End"]
    AC --> AE
    AD --> AE
```

### Persistence model

Mangoduck keeps the provider-side main chat flow stateless:

- It does not use `store=true`.
- It does not use `previous_response_id`.
- It stores only normalized `user text`, `assistant text`, `function_call`, and `function_call_output` items locally by Telegram `chat_id`.
- Scheduled cron runs execute without replaying chat history.

## Requirements

- Docker available locally.
- `make` available locally.
- Telegram bot token.
- API key for your configured Responses provider.
- OpenAI API key for `web-search`, if enabled.
- xAI API key for `x-search`, if enabled.

Go must be run only through the provided `Makefile` targets. Do not run `go` directly for project tasks, and use direct Docker commands only for the documented local gateway setup or the provided `make` targets.

## Quick Start

### 1. Start the local gateway

The example config expects a Portkey-compatible gateway on `http://127.0.0.1:8787`.

```bash
docker compose up -d
```

### 2. Create local config

Use the committed example config as the starting point:

```bash
cp config.yaml.dist config.yaml
```

Then fill in:

- `telegram.token`
- `admin.tg_ids`
- `responses.provider_api_key`
- `built_it_tools.web_search.api_key`, if you enable `web-search`
- `built_it_tools.x_search.api_key`, if you enable `x-search`
- any MCP server settings you want to enable

`config.yaml` is ignored by Git and should be treated as a secrets file.

### 3. Run the bot

```bash
make go CMD="run ./cmd/mangoduck"
```

On startup the bot will:

- load `config.yaml`
- open or create the SQLite database
- run embedded migrations
- validate the Responses API and, when enabled, the xAI API, the OpenAI web search API, and enabled MCP servers
- start Telegram polling

## Configuration

The safe example config lives in [`config.yaml.dist`](config.yaml.dist).

### Telegram

- `telegram.token`: bot token
- `telegram.poll_timeout`: polling timeout, default `10s`

### Admin

- `admin.tg_ids`: list of Telegram user IDs allowed to manage chats and cron tasks

### Database

- `database.path`: file-backed SQLite path or file-backed SQLite `file:` URI, default `mangoduck.db`

### Responses API

- `responses.base_url`: gateway or provider base URL
- `responses.provider`: provider name sent to the gateway
- `responses.provider_api_key`: API key for the configured provider
- `responses.model`: main model, for example `gpt-5-mini`
- `responses.timeout`: request timeout, default `30s`

### Built-in tools

`built_it_tools` groups configuration for the built-in search tools.

- `built_it_tools.web_search.api_key`: dedicated OpenAI API key for the `web-search` tool
- `built_it_tools.web_search.enabled`: enable or disable the `web-search` tool, default `true`
- `built_it_tools.web_search.base_url`: direct OpenAI base URL, default `https://api.openai.com`
- `built_it_tools.web_search.model`: web search model, default `gpt-5.4-nano`
- `built_it_tools.web_search.timeout`: request timeout, default `30s`
- `built_it_tools.x_search.api_key`: xAI API key
- `built_it_tools.x_search.enabled`: enable or disable the `x-search` tool, default `true`
- `built_it_tools.x_search.base_url`: xAI base URL
- `built_it_tools.x_search.model`: search model, for example `grok-4-1-fast-reasoning`
- `built_it_tools.x_search.timeout`: request timeout, default `30s`

### MCP

Each entry under `mcp.servers` can be enabled independently.

- `transport: streamable_http` for remote MCP servers
- `transport: stdio` for local MCP servers started per run

## Development

### Common commands

```bash
make fmt
make vet
make test
make build
make lint
```

### Arbitrary Go command

```bash
make go CMD="test ./internal/llm/chat -run TestName"
```

### Notes

- All Go commands must go through `make`.
- The Go module cache is mounted from `$(HOME)/go/pkg/mod`.
- `make` targets use Docker images defined in the [`Makefile`](Makefile).

## Chat Model

- Every Telegram conversation target is a `chat`.
- New chats are created as inactive.
- Access control is managed only per Telegram `chat_id`.
- Group and supergroup messages are handled only when the bot is explicitly mentioned.
- Per-chat memory is stored as a single free-form text block and injected into the system prompt when non-empty.

## Scheduled Tasks

Admins can list, create, and delete cron-backed prompts through the agent tools. Scheduled tasks:

- are persisted in SQLite
- are restored on startup
- run in the same agentic mode as regular chat
- do not replay prior chat history
- send the result back into the originating Telegram chat

## MCP Integration

Enabled MCP servers are opened for a single run, translated into Responses API function tools, and closed after the run finishes. Mangoduck supports:

- remote `streamable_http` servers
- local `stdio` servers

MCP tool names are exposed to the model with a server prefix such as `github__search_repositories`.

## Project Status

Mangoduck is structured like a small production-oriented bot service rather than a demo app. The repository already includes tests across config loading, bot behavior, repositories, chat orchestration, MCP bridging, cron jobs, and Telegram handlers.
