# This project is Go telegram bot that is agentic assistant for individual user / telegram groups

# Security
- Treat `config.yaml` as a secrets file with credentials — do not read or dump it unless the task explicitly requires config work
- Use `config.yaml.dist` as the safe example config committed to the repo
- `config.yaml` is listed in `.gitignore`

# Go / Docker
- Go is NOT installed locally — all Go commands MUST be run via `Makefile` targets (which use Docker)
- Go module cache with downloaded library sources is available at `$(HOME)/go/pkg/mod`
- Use `make go CMD="..."` for arbitrary go commands
- Available targets: `make tidy`, `make fmt`, `make vet`, `make test`, `make build`, `make lint`
- NEVER run `go` directly — always use `make`
- NEVER run `docker` directly — only use `make` targets

# Go Style
- Use pointer receivers and pointer results (unless simple types like int/string)
- Avoid struct `{}` literals; use `var foo StructType` (or pointer) so zero values remain unallocated
- Use `any` instead of `interface{}`
- Don't use `strings.Strip` where it is not needed

# Important
- CLAUDE.md and AGENTS.md must always be kept in sync — any change to one must be applied to the other
- Always run the relevant tests after making changes
- README.md must always be kept up to date when user flows, roles, statuses, commands, or admin actions change
- README.md always follows the real code behavior, not the other way around
- Access control is managed only per Telegram `chat_id`; admins are defined only in YAML under `admin.tg_ids`
- LLM chat context is stateless on the OpenAI side: do not use `store:true` or `previous_response_id` for main chat flow
- Persist and replay only normalized text/tool Responses API items locally by Telegram `chat_id`: user text, assistant text, `function_call`, and `function_call_output`
- Persist cron tasks in the database and execute scheduled prompts in the same agentic mode but without replaying chat history
