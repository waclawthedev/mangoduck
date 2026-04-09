package config

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	defaultPollTimeout            = 10 * time.Second
	defaultDBPath                 = "mangoduck.db"
	defaultResponsesTimeout       = 30 * time.Second
	defaultConfigPath             = "config.yaml"
	defaultOpenAIWebSearchBaseURL = "https://api.openai.com"
	defaultOpenAIWebSearchModel   = "gpt-5.4-nano"
)

type Config struct {
	TelegramToken          string
	AdminTGIDs             []int64
	AdminTGID              int64
	DatabasePath           string
	PollTimeout            time.Duration
	PortkeyBaseURL         string
	PortkeyProvider        string
	PortkeyProviderAPIKey  string
	MainModel              string
	ResponsesTimeout       time.Duration
	EnableOpenAIWebSearch  *bool
	OpenAIWebSearchAPIKey  string
	OpenAIWebSearchBaseURL string
	OpenAIWebSearchModel   string
	OpenAIWebSearchTimeout time.Duration
	EnableXSearch          *bool
	XAIAPIKey              string
	XAIBaseURL             string
	XAIModel               string
	XAITimeout             time.Duration
	MCP                    MCPConfig
}

type MCPConfig struct {
	Servers []*MCPServer
}

type MCPServer struct {
	Name      string
	Enabled   bool
	Transport string
	HTTP      *MCPHTTPServer
	Stdio     *MCPStdioServer
}

type MCPHTTPServer struct {
	URL        string
	Headers    map[string]string
	AuthBearer string
}

type MCPStdioServer struct {
	Command string
	Args    []string
	CWD     string
	Env     map[string]string
}

type rawConfig struct {
	Telegram     *telegramConfig     `yaml:"telegram"`
	Admin        *adminConfig        `yaml:"admin"`
	Database     *databaseConfig     `yaml:"database"`
	Responses    *responsesConfig    `yaml:"responses"`
	BuiltITTools *builtITToolsConfig `yaml:"built_it_tools"`
	MCP          *mcpFileConfig      `yaml:"mcp"`
}

type telegramConfig struct {
	Token       string `yaml:"token"`
	PollTimeout string `yaml:"poll_timeout"`
}

type adminConfig struct {
	TGID  int64   `yaml:"tg_id"`
	TGIDs []int64 `yaml:"tg_ids"`
}

type databaseConfig struct {
	Path string `yaml:"path"`
}

type responsesConfig struct {
	BaseURL        string `yaml:"base_url"`
	Provider       string `yaml:"provider"`
	ProviderAPIKey string `yaml:"provider_api_key"`
	Model          string `yaml:"model"`
	Timeout        string `yaml:"timeout"`
}

type builtITToolsConfig struct {
	WebSearch *webSearchConfig `yaml:"web_search"`
	XSearch   *xSearchConfig   `yaml:"x_search"`
}

type webSearchConfig struct {
	Enabled *bool  `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	Timeout string `yaml:"timeout"`
}

type xSearchConfig struct {
	Enabled *bool  `yaml:"enabled"`
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	Model   string `yaml:"model"`
	Timeout string `yaml:"timeout"`
}

type mcpFileConfig struct {
	Servers []*mcpServerConfig `yaml:"servers"`
}

type mcpServerConfig struct {
	Name      string          `yaml:"name"`
	Enabled   *bool           `yaml:"enabled"`
	Transport string          `yaml:"transport"`
	HTTP      *mcpHTTPConfig  `yaml:"http"`
	Stdio     *mcpStdioConfig `yaml:"stdio"`
}

type mcpHTTPConfig struct {
	URL        string            `yaml:"url"`
	Headers    map[string]string `yaml:"headers"`
	AuthBearer string            `yaml:"auth_bearer"`
}

type mcpStdioConfig struct {
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	CWD     string            `yaml:"cwd"`
	Env     map[string]string `yaml:"env"`
}

func Load() (Config, error) {
	data, err := os.ReadFile(defaultConfigPath)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", defaultConfigPath, err)
	}

	var parsed rawConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	err = decoder.Decode(&parsed)
	if err != nil {
		return Config{}, fmt.Errorf("decode %s: %w", defaultConfigPath, err)
	}

	return buildConfig(&parsed)
}

func buildConfig(parsed *rawConfig) (Config, error) {
	if parsed == nil {
		return Config{}, errors.New("config payload is required")
	}

	var cfg Config

	if parsed.Telegram != nil {
		cfg.TelegramToken = strings.TrimSpace(parsed.Telegram.Token)

		timeout, err := parseDurationWithDefault(parsed.Telegram.PollTimeout, defaultPollTimeout, "telegram.poll_timeout")
		if err != nil {
			return Config{}, err
		}
		cfg.PollTimeout = timeout
	}

	if parsed.Admin != nil {
		cfg.AdminTGIDs = normalizeAdminTGIDs(parsed.Admin.TGIDs, parsed.Admin.TGID)
		if len(cfg.AdminTGIDs) > 0 {
			cfg.AdminTGID = cfg.AdminTGIDs[0]
		}
	}

	cfg.DatabasePath = defaultDBPath
	if parsed.Database != nil && strings.TrimSpace(parsed.Database.Path) != "" {
		cfg.DatabasePath = strings.TrimSpace(parsed.Database.Path)
	}

	if cfg.PollTimeout == 0 {
		cfg.PollTimeout = defaultPollTimeout
	}
	cfg.ResponsesTimeout = defaultResponsesTimeout
	cfg.OpenAIWebSearchBaseURL = defaultOpenAIWebSearchBaseURL
	cfg.OpenAIWebSearchModel = defaultOpenAIWebSearchModel
	cfg.OpenAIWebSearchTimeout = defaultResponsesTimeout
	cfg.EnableOpenAIWebSearch = boolPtr(true)
	cfg.EnableXSearch = boolPtr(true)
	cfg.XAITimeout = defaultResponsesTimeout

	if parsed.Responses != nil {
		cfg.PortkeyBaseURL = strings.TrimSpace(parsed.Responses.BaseURL)
		cfg.PortkeyProvider = strings.TrimSpace(parsed.Responses.Provider)
		cfg.PortkeyProviderAPIKey = strings.TrimSpace(parsed.Responses.ProviderAPIKey)
		cfg.MainModel = strings.TrimSpace(parsed.Responses.Model)

		timeout, err := parseDurationWithDefault(parsed.Responses.Timeout, defaultResponsesTimeout, "responses.timeout")
		if err != nil {
			return Config{}, err
		}
		cfg.ResponsesTimeout = timeout
	}

	if parsed.BuiltITTools != nil {
		if parsed.BuiltITTools.WebSearch != nil {
			cfg.EnableOpenAIWebSearch = boolPtr(boolOrDefault(parsed.BuiltITTools.WebSearch.Enabled, true))
			cfg.OpenAIWebSearchAPIKey = strings.TrimSpace(parsed.BuiltITTools.WebSearch.APIKey)
			cfg.OpenAIWebSearchBaseURL = strings.TrimSpace(parsed.BuiltITTools.WebSearch.BaseURL)
			cfg.OpenAIWebSearchModel = strings.TrimSpace(parsed.BuiltITTools.WebSearch.Model)

			timeout, err := parseDurationWithDefault(parsed.BuiltITTools.WebSearch.Timeout, defaultResponsesTimeout, "built_it_tools.web_search.timeout")
			if err != nil {
				return Config{}, err
			}
			cfg.OpenAIWebSearchTimeout = timeout
		}

		if parsed.BuiltITTools.XSearch != nil {
			cfg.EnableXSearch = boolPtr(boolOrDefault(parsed.BuiltITTools.XSearch.Enabled, true))
			cfg.XAIAPIKey = strings.TrimSpace(parsed.BuiltITTools.XSearch.APIKey)
			cfg.XAIBaseURL = strings.TrimSpace(parsed.BuiltITTools.XSearch.BaseURL)
			cfg.XAIModel = strings.TrimSpace(parsed.BuiltITTools.XSearch.Model)

			timeout, err := parseDurationWithDefault(parsed.BuiltITTools.XSearch.Timeout, defaultResponsesTimeout, "built_it_tools.x_search.timeout")
			if err != nil {
				return Config{}, err
			}
			cfg.XAITimeout = timeout
		}
	}

	mcpConfig, err := buildMCPConfig(parsed.MCP)
	if err != nil {
		return Config{}, err
	}
	cfg.MCP = *mcpConfig

	return cfg, nil
}

func buildMCPConfig(parsed *mcpFileConfig) (*MCPConfig, error) {
	var cfg MCPConfig
	if parsed == nil || len(parsed.Servers) == 0 {
		return &cfg, nil
	}

	cfg.Servers = make([]*MCPServer, 0, len(parsed.Servers))

	for index, server := range parsed.Servers {
		if server == nil {
			return nil, fmt.Errorf("mcp.servers[%d] is required", index)
		}

		var entry MCPServer
		entry.Name = strings.TrimSpace(server.Name)
		entry.Transport = strings.TrimSpace(server.Transport)
		if server.HTTP != nil {
			var httpEntry MCPHTTPServer
			httpEntry.URL = strings.TrimSpace(server.HTTP.URL)
			httpEntry.AuthBearer = strings.TrimSpace(server.HTTP.AuthBearer)
			if len(server.HTTP.Headers) > 0 {
				httpEntry.Headers = make(map[string]string, len(server.HTTP.Headers))
				for key, value := range server.HTTP.Headers {
					httpEntry.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
				}
			}
			entry.HTTP = &httpEntry
		}
		if server.Stdio != nil {
			var stdioEntry MCPStdioServer
			stdioEntry.Command = strings.TrimSpace(server.Stdio.Command)
			stdioEntry.CWD = strings.TrimSpace(server.Stdio.CWD)
			if len(server.Stdio.Args) > 0 {
				stdioEntry.Args = make([]string, 0, len(server.Stdio.Args))
				for _, value := range server.Stdio.Args {
					stdioEntry.Args = append(stdioEntry.Args, strings.TrimSpace(value))
				}
			}
			if len(server.Stdio.Env) > 0 {
				stdioEntry.Env = make(map[string]string, len(server.Stdio.Env))
				for key, value := range server.Stdio.Env {
					stdioEntry.Env[strings.TrimSpace(key)] = strings.TrimSpace(value)
				}
			}
			entry.Stdio = &stdioEntry
		}

		if server.Enabled == nil {
			entry.Enabled = true
		} else {
			entry.Enabled = *server.Enabled
		}

		if entry.Name == "" {
			return nil, fmt.Errorf("mcp.servers[%d].name is required", index)
		}
		if entry.Transport == "" {
			return nil, fmt.Errorf("mcp.servers[%d].transport is required", index)
		}
		switch entry.Transport {
		case "streamable_http":
			if entry.HTTP == nil || entry.HTTP.URL == "" {
				return nil, fmt.Errorf("mcp.servers[%d].http.url is required", index)
			}
		case "stdio":
			if entry.Stdio == nil || entry.Stdio.Command == "" {
				return nil, fmt.Errorf("mcp.servers[%d].stdio.command is required", index)
			}
		default:
			return nil, fmt.Errorf("mcp.servers[%d].transport %q is not supported", index, entry.Transport)
		}

		cfg.Servers = append(cfg.Servers, &entry)
	}

	return &cfg, nil
}

func parseDurationWithDefault(raw string, fallback time.Duration, field string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be a valid duration: %w", field, err)
	}

	return value, nil
}

func normalizeAdminTGIDs(ids []int64, legacyID int64) []int64 {
	normalized := make([]int64, 0, len(ids)+1)
	seen := make(map[int64]struct{}, len(ids)+1)

	appendID := func(id int64) {
		if id == 0 {
			return
		}
		if _, exists := seen[id]; exists {
			return
		}

		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}

	for _, id := range ids {
		appendID(id)
	}
	appendID(legacyID)

	return normalized
}

func boolOrDefault(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}

	return *value
}

func boolPtr(value bool) *bool {
	return &value
}

func (c Config) OpenAIWebSearchEnabled() bool {
	return boolOrDefault(c.EnableOpenAIWebSearch, true)
}

func (c Config) XSearchEnabled() bool {
	return boolOrDefault(c.EnableXSearch, true)
}

func (c Config) IsAdminTGID(tgID int64) bool {
	if tgID == 0 {
		return false
	}

	for _, adminTGID := range c.AdminTGIDs {
		if adminTGID == tgID {
			return true
		}
	}

	return c.AdminTGID == tgID
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.TelegramToken) == "" {
		return errors.New("telegram token is empty")
	}

	if len(c.AdminTGIDs) == 0 && c.AdminTGID == 0 {
		return errors.New("admin telegram IDs are not set")
	}

	if strings.TrimSpace(c.DatabasePath) == "" {
		return errors.New("database path is empty")
	}
	if c.DatabasePath == ":memory:" {
		return errors.New("database path must be file-backed; in-memory sqlite databases are not supported")
	}
	if strings.HasPrefix(c.DatabasePath, "file:") {
		dsnURL, err := url.Parse(c.DatabasePath)
		if err != nil {
			return fmt.Errorf("database path is not a valid sqlite file URI: %w", err)
		}

		query := dsnURL.Query()
		if strings.EqualFold(query.Get("mode"), "memory") || strings.EqualFold(query.Get("vfs"), "memdb") {
			return errors.New("database path must be file-backed; in-memory sqlite databases are not supported")
		}
	}

	if c.PollTimeout <= 0 {
		return fmt.Errorf("poll timeout must be positive, got %s", c.PollTimeout)
	}

	if c.ResponsesTimeout <= 0 {
		return fmt.Errorf("responses timeout must be positive, got %s", c.ResponsesTimeout)
	}

	if c.OpenAIWebSearchEnabled() {
		if strings.TrimSpace(c.OpenAIWebSearchAPIKey) == "" {
			return errors.New("openai web search api key is empty")
		}

		if c.OpenAIWebSearchTimeout <= 0 {
			return fmt.Errorf("openai web search timeout must be positive, got %s", c.OpenAIWebSearchTimeout)
		}
	}

	if c.XSearchEnabled() {
		if strings.TrimSpace(c.XAIAPIKey) == "" {
			return errors.New("xai api key is empty")
		}

		if c.XAITimeout <= 0 {
			return fmt.Errorf("xai timeout must be positive, got %s", c.XAITimeout)
		}
	}

	return nil
}
