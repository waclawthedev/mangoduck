package mcpbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mangoduck/internal/config"
	"mangoduck/internal/llm/responses"
)

const (
	streamableHTTPTransport = "streamable_http"
	stdioTransport          = "stdio"
	clientName              = "mangoduck"
	errNilServerConfig      = "mcp server config is nil"
)

type Bridge struct {
	servers   []*config.MCPServer
	connector connector
}

type Run struct {
	tools   []*responses.Tool
	servers map[string]*serverRuntime
}

type connector interface {
	Connect(ctx context.Context, server *config.MCPServer) (session, error)
}

type session interface {
	ListTools(ctx context.Context, params *mcp.ListToolsParams) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error)
	Close() error
}

type serverRuntime struct {
	toolPrefix string
	session    session
	tools      map[string]string
}

type sdkConnector struct{}

type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

func New(cfg config.MCPConfig) (*Bridge, error) {
	return newBridge(cfg, &sdkConnector{})
}

func newBridge(cfg config.MCPConfig, conn connector) (*Bridge, error) {
	if conn == nil {
		return nil, errors.New("mcp connector is required")
	}

	var bridge Bridge
	bridge.connector = conn

	for index, server := range cfg.Servers {
		if server == nil {
			return nil, fmt.Errorf("mcp server %d is nil", index)
		}
		if !server.Enabled {
			continue
		}
		transport := strings.TrimSpace(server.Transport)
		if transport != streamableHTTPTransport && transport != stdioTransport {
			return nil, fmt.Errorf("unsupported mcp transport %q for server %q", server.Transport, server.Name)
		}
		bridge.servers = append(bridge.servers, server)
	}

	return &bridge, nil
}

func (b *Bridge) OpenSession(ctx context.Context) (*Run, error) {
	if b == nil || len(b.servers) == 0 {
		return &Run{}, nil
	}

	var run Run
	run.servers = make(map[string]*serverRuntime, len(b.servers))

	for _, server := range b.servers {
		serverSession, err := b.connector.Connect(ctx, server)
		if err != nil {
			continue
		}

		tools, err := listAllTools(ctx, serverSession)
		if err != nil {
			_ = serverSession.Close()
			continue
		}

		prefix := strings.TrimSpace(server.Name)
		runtime := &serverRuntime{
			toolPrefix: prefix,
			session:    serverSession,
			tools:      make(map[string]string, len(tools)),
		}

		for _, tool := range tools {
			exportedTool, exportedName, originalName, err := translateTool(prefix, tool)
			if err != nil {
				continue
			}

			runtime.tools[exportedName] = originalName
			run.tools = append(run.tools, exportedTool)
		}

		run.servers[prefix] = runtime
	}

	return &run, nil
}

func (b *Bridge) Preflight(ctx context.Context) error {
	if b == nil || len(b.servers) == 0 {
		return nil
	}

	errs := make([]error, 0)

	for _, server := range b.servers {
		err := preflightServer(ctx, b.connector, server)
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (r *Run) Tools() []*responses.Tool {
	if r == nil || len(r.tools) == 0 {
		return nil
	}

	tools := make([]*responses.Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		if tool == nil {
			continue
		}

		cloned := *tool
		tools = append(tools, &cloned)
	}

	return tools
}

func (r *Run) Execute(ctx context.Context, name string, arguments string) (string, bool, error) {
	if r == nil || len(r.servers) == 0 {
		return "", false, nil
	}

	prefix, originalName, ok := splitToolName(name)
	if !ok {
		return "", false, nil
	}

	runtime, ok := r.servers[prefix]
	if !ok {
		return "", false, nil
	}

	if _, ok = runtime.tools[name]; !ok {
		return "", false, nil
	}

	parsedArguments, err := parseArguments(arguments)
	if err != nil {
		return "", true, fmt.Errorf("parse mcp function arguments: %w", err)
	}

	result, err := runtime.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      originalName,
		Arguments: parsedArguments,
	})
	if err != nil {
		return formatToolError(err.Error()), true, nil
	}

	output, err := flattenCallToolResult(result)
	if err != nil {
		return "", true, fmt.Errorf("flatten mcp function result: %w", err)
	}

	return output, true, nil
}

func (r *Run) Close() error {
	if r == nil || len(r.servers) == 0 {
		return nil
	}

	var errs []error
	for _, runtime := range r.servers {
		if runtime == nil || runtime.session == nil {
			continue
		}
		if err := runtime.session.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (c *sdkConnector) Connect(ctx context.Context, server *config.MCPServer) (session, error) {
	if server == nil {
		return nil, errors.New(errNilServerConfig)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: clientName}, nil)
	transport, err := buildClientTransport(server)
	if err != nil {
		return nil, err
	}

	return client.Connect(ctx, transport, nil)
}

func preflightServer(ctx context.Context, conn connector, server *config.MCPServer) error {
	if conn == nil {
		return errors.New("mcp connector is required")
	}
	if server == nil {
		return errors.New(errNilServerConfig)
	}

	serverName := strings.TrimSpace(server.Name)
	if serverName == "" {
		serverName = "<unnamed>"
	}

	serverSession, err := conn.Connect(ctx, server)
	if err != nil {
		return fmt.Errorf("mcp server %q connect: %w", serverName, err)
	}

	_, err = listAllTools(ctx, serverSession)
	if err != nil {
		closeErr := serverSession.Close()
		if closeErr != nil {
			return errors.Join(
				fmt.Errorf("mcp server %q list tools: %w", serverName, err),
				fmt.Errorf("mcp server %q close: %w", serverName, closeErr),
			)
		}

		return fmt.Errorf("mcp server %q list tools: %w", serverName, err)
	}

	err = serverSession.Close()
	if err != nil {
		return fmt.Errorf("mcp server %q close: %w", serverName, err)
	}

	return nil
}

func buildClientTransport(server *config.MCPServer) (mcp.Transport, error) {
	if server == nil {
		return nil, errors.New(errNilServerConfig)
	}

	switch strings.TrimSpace(server.Transport) {
	case streamableHTTPTransport:
		if server.HTTP == nil {
			return nil, errors.New("mcp http config is nil")
		}
		return &mcp.StreamableClientTransport{
			Endpoint:             strings.TrimSpace(server.HTTP.URL),
			HTTPClient:           buildHTTPClient(server.HTTP),
			DisableStandaloneSSE: true,
		}, nil
	case stdioTransport:
		if server.Stdio == nil {
			return nil, errors.New("mcp stdio config is nil")
		}
		return buildCommandTransport(server.Stdio)
	default:
		return nil, fmt.Errorf("unsupported mcp transport %q", server.Transport)
	}
}

func buildHTTPClient(server *config.MCPHTTPServer) *http.Client {
	var headers map[string]string
	if server != nil && len(server.Headers) > 0 {
		headers = make(map[string]string, len(server.Headers)+1)
		for key, value := range server.Headers {
			headers[key] = value
		}
	}
	if server != nil && strings.TrimSpace(server.AuthBearer) != "" {
		if headers == nil {
			headers = make(map[string]string, 1)
		}
		headers["Authorization"] = "Bearer " + strings.TrimSpace(server.AuthBearer)
	}

	if len(headers) == 0 {
		return http.DefaultClient
	}

	var client http.Client
	client.Transport = &headerTransport{
		base:    http.DefaultTransport,
		headers: headers,
	}

	return &client
}

func buildCommandTransport(server *config.MCPStdioServer) (mcp.Transport, error) {
	if server == nil {
		return nil, errors.New("mcp stdio config is nil")
	}

	command := strings.TrimSpace(server.Command)
	if command == "" {
		return nil, errors.New("mcp stdio command is empty")
	}

	cmd := exec.CommandContext(context.Background(), command, server.Args...) //nolint:gosec // command and args are loaded from trusted local config
	cmd.Dir = strings.TrimSpace(server.CWD)
	cmd.Stderr = io.Discard
	if len(server.Env) > 0 {
		cmd.Env = append(os.Environ(), flattenEnv(server.Env)...)
	}

	return &mcp.CommandTransport{Command: cmd}, nil
}

func flattenEnv(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}

	env := make([]string, 0, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		env = append(env, key+"="+strings.TrimSpace(value))
	}

	return env
}

func (t *headerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if t == nil {
		return http.DefaultTransport.RoundTrip(request)
	}

	cloned := request.Clone(request.Context())
	for key, value := range t.headers {
		cloned.Header.Set(key, value)
	}

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	return base.RoundTrip(cloned)
}

func listAllTools(ctx context.Context, session session) ([]*mcp.Tool, error) {
	if session == nil {
		return nil, errors.New("mcp session is nil")
	}

	tools := make([]*mcp.Tool, 0)
	var cursor string

	for {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if result == nil {
			break
		}
		tools = append(tools, result.Tools...)
		cursor = strings.TrimSpace(result.NextCursor)
		if cursor == "" {
			break
		}
	}

	return tools, nil
}

func translateTool(prefix string, tool *mcp.Tool) (*responses.Tool, string, string, error) {
	if tool == nil {
		return nil, "", "", errors.New("mcp tool is nil")
	}

	payload, err := json.Marshal(tool)
	if err != nil {
		return nil, "", "", err
	}

	var raw struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema any    `json:"inputSchema"`
	}
	err = json.Unmarshal(payload, &raw)
	if err != nil {
		return nil, "", "", err
	}

	originalName := strings.TrimSpace(raw.Name)
	if originalName == "" {
		return nil, "", "", errors.New("mcp tool name is empty")
	}

	exportedName := prefix + "__" + originalName

	var translated responses.Tool
	translated.Type = "function"
	translated.Name = exportedName
	translated.Description = strings.TrimSpace(raw.Description)
	translated.Parameters = raw.InputSchema

	return &translated, exportedName, originalName, nil
}

func splitToolName(name string) (string, string, bool) {
	prefix, suffix, found := strings.Cut(strings.TrimSpace(name), "__")
	if !found || strings.TrimSpace(prefix) == "" || strings.TrimSpace(suffix) == "" {
		return "", "", false
	}

	return prefix, suffix, true
}

func parseArguments(arguments string) (map[string]any, error) {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return map[string]any{}, nil
	}

	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err != nil {
		return nil, err
	}

	if value == nil {
		return map[string]any{}, nil
	}

	parsed, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("arguments must decode to an object")
	}

	return parsed, nil
}

func flattenCallToolResult(result *mcp.CallToolResult) (string, error) {
	if result == nil {
		return "", nil
	}

	raw, err := decodeCallToolResult(result)
	if err != nil {
		return "", err
	}

	text, err := flattenCallToolContent(raw.Content, raw.StructuredContent)
	if err != nil {
		return "", err
	}
	if raw.IsError {
		return formatToolError(text), nil
	}

	return text, nil
}

type flattenedCallToolResult struct {
	Content           []json.RawMessage `json:"content"`
	StructuredContent any               `json:"structuredContent"`
	IsError           bool              `json:"isError"`
}

func decodeCallToolResult(result *mcp.CallToolResult) (*flattenedCallToolResult, error) {
	payload, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}

	var raw flattenedCallToolResult
	err = json.Unmarshal(payload, &raw)
	if err != nil {
		return nil, err
	}

	return &raw, nil
}

func flattenCallToolContent(content []json.RawMessage, structuredContent any) (string, error) {
	var builder strings.Builder

	err := appendFlattenedContent(&builder, content)
	if err != nil {
		return "", err
	}

	err = appendStructuredContent(&builder, structuredContent)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(builder.String()), nil
}

func appendFlattenedContent(builder *strings.Builder, content []json.RawMessage) error {
	for _, item := range content {
		text, err := flattenContentItem(item)
		if err != nil {
			return err
		}
		appendFlattenedLine(builder, text)
	}

	return nil
}

func appendStructuredContent(builder *strings.Builder, structuredContent any) error {
	if structuredContent == nil {
		return nil
	}

	structured, err := json.Marshal(structuredContent)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(structured)) == 0 || string(structured) == "null" {
		return nil
	}

	appendFlattenedLine(builder, string(structured))
	return nil
}

func appendFlattenedLine(builder *strings.Builder, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if builder.Len() > 0 {
		builder.WriteString("\n")
	}
	builder.WriteString(text)
}

func flattenContentItem(raw json.RawMessage) (string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", nil
	}

	var envelope struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}

	if strings.TrimSpace(envelope.Type) == "text" {
		return strings.TrimSpace(envelope.Text), nil
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return "", err
	}

	return string(encoded), nil
}

func formatToolError(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		message = "Unknown MCP tool error."
	}

	return "MCP tool error: " + message
}
