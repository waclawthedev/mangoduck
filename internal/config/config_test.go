package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mangoduck/internal/config"

	"github.com/stretchr/testify/require"
)

func boolPtr(value bool) *bool {
	return &value
}

func TestLoadReadsValuesFromConfigYAML(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"
  poll_timeout: "12s"

admin:
  tg_ids:
    - 42
    - 84

database:
  path: "data.db"

responses:
  base_url: "https://api.portkey.ai"
  provider: "openai"
  provider_api_key: "openai-test"
  model: "gpt-5"
  timeout: "15s"

built_it_tools:
  web_search:
    enabled: true
    api_key: "openai-web-search-test"
    base_url: "https://api.openai.com"
    model: "gpt-5.4-nano"
    timeout: "18s"
  x_search:
    enabled: true
    api_key: "xai-test"
    base_url: "https://api.x.ai"
    model: "grok-test"
    timeout: "25s"

mcp:
  servers:
    - name: "github"
      enabled: true
      transport: "streamable_http"
      http:
        url: "https://example.com/mcp"
        auth_bearer: "test-token"
        headers:
          x-team: "backend"
`)

	cfg, err := config.Load()
	require.NoError(t, err)

	require.Equal(t, "telegram-token", cfg.TelegramToken)
	require.Equal(t, []int64{42, 84}, cfg.AdminTGIDs)
	require.Equal(t, int64(42), cfg.AdminTGID)
	require.Equal(t, "data.db", cfg.DatabasePath)
	require.Equal(t, 12*time.Second, cfg.PollTimeout)
	require.Equal(t, "https://api.portkey.ai", cfg.PortkeyBaseURL)
	require.Equal(t, "openai", cfg.PortkeyProvider)
	require.Equal(t, "openai-test", cfg.PortkeyProviderAPIKey)
	require.Equal(t, "gpt-5", cfg.MainModel)
	require.Equal(t, 15*time.Second, cfg.ResponsesTimeout)
	require.Equal(t, "openai-web-search-test", cfg.OpenAIWebSearchAPIKey)
	require.Equal(t, "https://api.openai.com", cfg.OpenAIWebSearchBaseURL)
	require.Equal(t, "gpt-5.4-nano", cfg.OpenAIWebSearchModel)
	require.Equal(t, 18*time.Second, cfg.OpenAIWebSearchTimeout)
	require.True(t, cfg.OpenAIWebSearchEnabled())
	require.Equal(t, "xai-test", cfg.XAIAPIKey)
	require.Equal(t, "https://api.x.ai", cfg.XAIBaseURL)
	require.Equal(t, "grok-test", cfg.XAIModel)
	require.Equal(t, 25*time.Second, cfg.XAITimeout)
	require.True(t, cfg.XSearchEnabled())
	require.Len(t, cfg.MCP.Servers, 1)
	require.Equal(t, "github", cfg.MCP.Servers[0].Name)
	require.True(t, cfg.MCP.Servers[0].Enabled)
	require.Equal(t, "streamable_http", cfg.MCP.Servers[0].Transport)
	require.NotNil(t, cfg.MCP.Servers[0].HTTP)
	require.Equal(t, "https://example.com/mcp", cfg.MCP.Servers[0].HTTP.URL)
	require.Equal(t, "test-token", cfg.MCP.Servers[0].HTTP.AuthBearer)
	require.Equal(t, "backend", cfg.MCP.Servers[0].HTTP.Headers["x-team"])
}

func TestLoadAppliesDefaults(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42
`)

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Equal(t, "mangoduck.db", cfg.DatabasePath)
	require.Equal(t, 10*time.Second, cfg.PollTimeout)
	require.Equal(t, 30*time.Second, cfg.ResponsesTimeout)
	require.Equal(t, "https://api.openai.com", cfg.OpenAIWebSearchBaseURL)
	require.Equal(t, "gpt-5.4-nano", cfg.OpenAIWebSearchModel)
	require.Equal(t, 30*time.Second, cfg.OpenAIWebSearchTimeout)
	require.Equal(t, 30*time.Second, cfg.XAITimeout)
	require.True(t, cfg.OpenAIWebSearchEnabled())
	require.True(t, cfg.XSearchEnabled())
	require.Empty(t, cfg.MCP.Servers)
}

func TestValidateRejectsInMemoryDatabasePath(t *testing.T) {
	var cfg config.Config
	cfg.TelegramToken = "telegram-token"
	cfg.AdminTGIDs = []int64{42}
	cfg.DatabasePath = "file:mangoduck.db?cache=shared"
	cfg.PollTimeout = 10 * time.Second
	cfg.ResponsesTimeout = 30 * time.Second
	cfg.EnableOpenAIWebSearch = boolPtr(false)
	cfg.EnableXSearch = boolPtr(false)

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorContains(t, err, "database path must be a sqlite file path, not a URI")
}

func TestValidateRejectsQueryStringInDatabasePath(t *testing.T) {
	var cfg config.Config
	cfg.TelegramToken = "telegram-token"
	cfg.AdminTGIDs = []int64{42}
	cfg.DatabasePath = "mangoduck.db?cache=shared"
	cfg.PollTimeout = 10 * time.Second
	cfg.ResponsesTimeout = 30 * time.Second
	cfg.EnableOpenAIWebSearch = boolPtr(false)
	cfg.EnableXSearch = boolPtr(false)

	err := cfg.Validate()
	require.Error(t, err)
	require.ErrorContains(t, err, "database path must be a sqlite file path, not a URI")
}

func TestLoadAllowsDisablingBuiltInSearchTools(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42

built_it_tools:
  web_search:
    enabled: false
  x_search:
    enabled: false
`)

	cfg, err := config.Load()
	require.NoError(t, err)
	require.False(t, cfg.OpenAIWebSearchEnabled())
	require.False(t, cfg.XSearchEnabled())
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"
  extra: "oops"

admin:
  tg_ids:
    - 42
`)

	_, err := config.Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "field extra not found")
}

func TestLoadRejectsInvalidDuration(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42

responses:
  timeout: "later"
`)

	_, err := config.Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "responses.timeout must be a valid duration")
}

func TestLoadRejectsInvalidMCPServer(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42

mcp:
  servers:
    - name: ""
      transport: "streamable_http"
      http:
        url: "https://example.com/mcp"
`)

	_, err := config.Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "mcp.servers[0].name is required")
}

func TestLoadReadsValidStdioMCPServer(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42

mcp:
  servers:
    - name: "filesystem"
      enabled: true
      transport: "stdio"
      stdio:
        command: "npx"
        args:
          - "-y"
          - "@modelcontextprotocol/server-filesystem"
          - "/tmp"
        cwd: "/workspace"
        env:
          HOME: "/tmp/mcp-home"
          LOG_LEVEL: "debug"
`)

	cfg, err := config.Load()
	require.NoError(t, err)
	require.Len(t, cfg.MCP.Servers, 1)
	require.Equal(t, "filesystem", cfg.MCP.Servers[0].Name)
	require.Equal(t, "stdio", cfg.MCP.Servers[0].Transport)
	require.NotNil(t, cfg.MCP.Servers[0].Stdio)
	require.Equal(t, "npx", cfg.MCP.Servers[0].Stdio.Command)
	require.Equal(t, []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp"}, cfg.MCP.Servers[0].Stdio.Args)
	require.Equal(t, "/workspace", cfg.MCP.Servers[0].Stdio.CWD)
	require.Equal(t, "/tmp/mcp-home", cfg.MCP.Servers[0].Stdio.Env["HOME"])
	require.Equal(t, "debug", cfg.MCP.Servers[0].Stdio.Env["LOG_LEVEL"])
}

func TestLoadRejectsStdioServerWithoutCommand(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42

mcp:
  servers:
    - name: "filesystem"
      transport: "stdio"
      stdio:
        args:
          - "server"
`)

	_, err := config.Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "mcp.servers[0].stdio.command is required")
}

func TestLoadRejectsHTTPServerWithoutURL(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	writeConfigFile(t, tempDir, `
telegram:
  token: "telegram-token"

admin:
  tg_ids:
    - 42

mcp:
  servers:
    - name: "github"
      transport: "streamable_http"
      http:
        auth_bearer: "test-token"
`)

	_, err := config.Load()
	require.Error(t, err)
	require.ErrorContains(t, err, "mcp.servers[0].http.url is required")
}

func TestConfigYAMLDistIsValid(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config.yaml.dist"))
	require.NoError(t, err)

	tempDir := t.TempDir()
	t.Chdir(tempDir)

	err = os.WriteFile("config.yaml", data, 0o600) //nolint:gosec // test writes a fixed filename inside t.TempDir after t.Chdir
	require.NoError(t, err)

	_, err = config.Load()
	require.NoError(t, err)
}

func writeConfigFile(t *testing.T, dir string, content string) {
	t.Helper()

	err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o600)
	require.NoError(t, err)
}
