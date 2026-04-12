package config

import (
	"bytes"
	"errors"
	"fmt"
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
	TelegramToken           string
	AdminTGIDs              []int64
	AdminTGID               int64
	DatabasePath            string
	PollTimeout             time.Duration
	ResponsesProvider       string
	ResponsesProviderAPIKey string
	MainModel               string
	ResponsesTimeout        time.Duration
	EnableOpenAIWebSearch   *bool
	OpenAIWebSearchAPIKey   string
	OpenAIWebSearchBaseURL  string
	OpenAIWebSearchModel    string
	OpenAIWebSearchTimeout  time.Duration
	EnableXSearch           *bool
	XAIAPIKey               string
	XAIBaseURL              string
	XAIModel                string
	XAITimeout              time.Duration
	MCP                     MCPConfig
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

	applyConfigDefaults(&cfg)

	err := applyTelegramConfig(&cfg, parsed.Telegram)
	if err != nil {
		return Config{}, err
	}

	applyAdminConfig(&cfg, parsed.Admin)
	applyDatabaseConfig(&cfg, parsed.Database)

	err = applyResponsesConfig(&cfg, parsed.Responses)
	if err != nil {
		return Config{}, err
	}

	err = applyBuiltInToolsConfig(&cfg, parsed.BuiltITTools)
	if err != nil {
		return Config{}, err
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
		entry, err := buildMCPServer(index, server)
		if err != nil {
			return nil, err
		}
		cfg.Servers = append(cfg.Servers, &entry)
	}

	return &cfg, nil
}

func applyConfigDefaults(cfg *Config) {
	cfg.DatabasePath = defaultDBPath
	cfg.PollTimeout = defaultPollTimeout
	cfg.ResponsesTimeout = defaultResponsesTimeout
	cfg.OpenAIWebSearchBaseURL = defaultOpenAIWebSearchBaseURL
	cfg.OpenAIWebSearchModel = defaultOpenAIWebSearchModel
	cfg.OpenAIWebSearchTimeout = defaultResponsesTimeout
	cfg.EnableOpenAIWebSearch = boolPtr(true)
	cfg.EnableXSearch = boolPtr(true)
	cfg.XAITimeout = defaultResponsesTimeout
}

func applyTelegramConfig(cfg *Config, parsed *telegramConfig) error {
	if parsed == nil {
		return nil
	}

	cfg.TelegramToken = strings.TrimSpace(parsed.Token)

	timeout, err := parseDurationWithDefault(parsed.PollTimeout, defaultPollTimeout, "telegram.poll_timeout")
	if err != nil {
		return err
	}
	cfg.PollTimeout = timeout

	return nil
}

func applyAdminConfig(cfg *Config, parsed *adminConfig) {
	if parsed == nil {
		return
	}

	cfg.AdminTGIDs = normalizeAdminTGIDs(parsed.TGIDs, parsed.TGID)
	if len(cfg.AdminTGIDs) > 0 {
		cfg.AdminTGID = cfg.AdminTGIDs[0]
	}
}

func applyDatabaseConfig(cfg *Config, parsed *databaseConfig) {
	if parsed == nil {
		return
	}

	path := strings.TrimSpace(parsed.Path)
	if path != "" {
		cfg.DatabasePath = path
	}
}

func applyResponsesConfig(cfg *Config, parsed *responsesConfig) error {
	if parsed == nil {
		return nil
	}

	cfg.ResponsesProvider = strings.TrimSpace(parsed.Provider)
	cfg.ResponsesProviderAPIKey = strings.TrimSpace(parsed.ProviderAPIKey)
	cfg.MainModel = strings.TrimSpace(parsed.Model)

	timeout, err := parseDurationWithDefault(parsed.Timeout, defaultResponsesTimeout, "responses.timeout")
	if err != nil {
		return err
	}
	cfg.ResponsesTimeout = timeout

	return nil
}

func applyBuiltInToolsConfig(cfg *Config, parsed *builtITToolsConfig) error {
	if parsed == nil {
		return nil
	}

	err := applyWebSearchConfig(cfg, parsed.WebSearch)
	if err != nil {
		return err
	}

	return applyXSearchConfig(cfg, parsed.XSearch)
}

func applyWebSearchConfig(cfg *Config, parsed *webSearchConfig) error {
	if parsed == nil {
		return nil
	}

	cfg.EnableOpenAIWebSearch = boolPtr(boolOrDefault(parsed.Enabled, true))
	cfg.OpenAIWebSearchAPIKey = strings.TrimSpace(parsed.APIKey)
	cfg.OpenAIWebSearchBaseURL = strings.TrimSpace(parsed.BaseURL)
	cfg.OpenAIWebSearchModel = strings.TrimSpace(parsed.Model)

	timeout, err := parseDurationWithDefault(parsed.Timeout, defaultResponsesTimeout, "built_it_tools.web_search.timeout")
	if err != nil {
		return err
	}
	cfg.OpenAIWebSearchTimeout = timeout

	return nil
}

func applyXSearchConfig(cfg *Config, parsed *xSearchConfig) error {
	if parsed == nil {
		return nil
	}

	cfg.EnableXSearch = boolPtr(boolOrDefault(parsed.Enabled, true))
	cfg.XAIAPIKey = strings.TrimSpace(parsed.APIKey)
	cfg.XAIBaseURL = strings.TrimSpace(parsed.BaseURL)
	cfg.XAIModel = strings.TrimSpace(parsed.Model)

	timeout, err := parseDurationWithDefault(parsed.Timeout, defaultResponsesTimeout, "built_it_tools.x_search.timeout")
	if err != nil {
		return err
	}
	cfg.XAITimeout = timeout

	return nil
}

func buildMCPServer(index int, server *mcpServerConfig) (MCPServer, error) {
	if server == nil {
		return MCPServer{}, fmt.Errorf("mcp.servers[%d] is required", index)
	}

	var entry MCPServer
	entry.Name = strings.TrimSpace(server.Name)
	entry.Transport = strings.TrimSpace(server.Transport)
	entry.Enabled = boolOrDefault(server.Enabled, true)
	entry.HTTP = buildMCPHTTPServer(server.HTTP)
	entry.Stdio = buildMCPStdioServer(server.Stdio)

	if err := validateMCPServer(index, &entry); err != nil {
		return MCPServer{}, err
	}

	return entry, nil
}

func buildMCPHTTPServer(parsed *mcpHTTPConfig) *MCPHTTPServer {
	if parsed == nil {
		return nil
	}

	var entry MCPHTTPServer
	entry.URL = strings.TrimSpace(parsed.URL)
	entry.AuthBearer = strings.TrimSpace(parsed.AuthBearer)
	if len(parsed.Headers) > 0 {
		entry.Headers = make(map[string]string, len(parsed.Headers))
		for key, value := range parsed.Headers {
			entry.Headers[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}

	return &entry
}

func buildMCPStdioServer(parsed *mcpStdioConfig) *MCPStdioServer {
	if parsed == nil {
		return nil
	}

	var entry MCPStdioServer
	entry.Command = strings.TrimSpace(parsed.Command)
	entry.CWD = strings.TrimSpace(parsed.CWD)
	if len(parsed.Args) > 0 {
		entry.Args = make([]string, 0, len(parsed.Args))
		for _, value := range parsed.Args {
			entry.Args = append(entry.Args, strings.TrimSpace(value))
		}
	}
	if len(parsed.Env) > 0 {
		entry.Env = make(map[string]string, len(parsed.Env))
		for key, value := range parsed.Env {
			entry.Env[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}

	return &entry
}

func validateMCPServer(index int, entry *MCPServer) error {
	if entry.Name == "" {
		return fmt.Errorf("mcp.servers[%d].name is required", index)
	}
	if entry.Transport == "" {
		return fmt.Errorf("mcp.servers[%d].transport is required", index)
	}

	switch entry.Transport {
	case "streamable_http":
		if entry.HTTP == nil || entry.HTTP.URL == "" {
			return fmt.Errorf("mcp.servers[%d].http.url is required", index)
		}
	case "stdio":
		if entry.Stdio == nil || entry.Stdio.Command == "" {
			return fmt.Errorf("mcp.servers[%d].stdio.command is required", index)
		}
	default:
		return fmt.Errorf("mcp.servers[%d].transport %q is not supported", index, entry.Transport)
	}

	return nil
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

	err := validateDatabasePath(c.DatabasePath)
	if err != nil {
		return err
	}

	if c.PollTimeout <= 0 {
		return fmt.Errorf("poll timeout must be positive, got %s", c.PollTimeout)
	}

	if c.ResponsesTimeout <= 0 {
		return fmt.Errorf("responses timeout must be positive, got %s", c.ResponsesTimeout)
	}

	err = validateResponsesConfig(c)
	if err != nil {
		return err
	}

	err = validateOpenAIWebSearchConfig(c)
	if err != nil {
		return err
	}

	return validateXSearchConfig(c)
}

func validateResponsesConfig(c Config) error {
	provider := strings.TrimSpace(c.ResponsesProvider)
	if provider == "" {
		return errors.New("responses provider is empty")
	}

	switch provider {
	case "openai", "xai":
	default:
		return fmt.Errorf("responses provider %q is not supported", provider)
	}

	if strings.TrimSpace(c.ResponsesProviderAPIKey) == "" {
		return errors.New("responses provider api key is empty")
	}

	if strings.TrimSpace(c.MainModel) == "" {
		return errors.New("responses model is empty")
	}

	return nil
}

func validateDatabasePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("database path is empty")
	}
	if strings.Contains(path, "?") || strings.Contains(path, "#") || strings.HasPrefix(path, "file:") {
		return errors.New("database path must be a sqlite file path, not a URI")
	}

	return nil
}

func validateOpenAIWebSearchConfig(c Config) error {
	if !c.OpenAIWebSearchEnabled() {
		return nil
	}
	if strings.TrimSpace(c.OpenAIWebSearchAPIKey) == "" {
		return errors.New("openai web search api key is empty")
	}
	if c.OpenAIWebSearchTimeout <= 0 {
		return fmt.Errorf("openai web search timeout must be positive, got %s", c.OpenAIWebSearchTimeout)
	}

	return nil
}

func validateXSearchConfig(c Config) error {
	if !c.XSearchEnabled() {
		return nil
	}
	if strings.TrimSpace(c.XAIAPIKey) == "" {
		return errors.New("xai api key is empty")
	}
	if c.XAITimeout <= 0 {
		return fmt.Errorf("xai timeout must be positive, got %s", c.XAITimeout)
	}

	return nil
}
