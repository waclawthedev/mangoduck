package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"mangoduck/internal/config"
)

type stubConnector struct {
	sessions    map[string]*stubSession
	err         error
	connectErrs map[string]error
}

func (c *stubConnector) Connect(_ context.Context, server *config.MCPServer) (session, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.connectErrs != nil {
		err := c.connectErrs[server.Name]
		if err != nil {
			return nil, err
		}
	}

	if c.sessions == nil {
		return nil, errors.New("missing session")
	}

	current := c.sessions[server.Name]
	if current == nil {
		return nil, errors.New("missing named session")
	}

	return current, nil
}

type stubSession struct {
	listResult *mcp.ListToolsResult
	callResult *mcp.CallToolResult
	listErr    error
	callErr    error
	callParams []*mcp.CallToolParams
	closed     bool
}

func (s *stubSession) ListTools(_ context.Context, _ *mcp.ListToolsParams) (*mcp.ListToolsResult, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}

	return s.listResult, nil
}

func (s *stubSession) CallTool(_ context.Context, params *mcp.CallToolParams) (*mcp.CallToolResult, error) {
	s.callParams = append(s.callParams, params)
	if s.callErr != nil {
		return nil, s.callErr
	}

	return s.callResult, nil
}

func (s *stubSession) Close() error {
	s.closed = true
	return nil
}

func TestBridgeOpenRunTranslatesMCPTools(t *testing.T) {
	session := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"search","description":"Remote search","inputSchema":{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}}`),
			},
		},
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{"github": session},
	})
	require.NoError(t, err)

	run, err := bridge.OpenSession(context.Background())
	require.NoError(t, err)
	require.Len(t, run.Tools(), 1)
	require.Equal(t, "github__search", run.Tools()[0].Name)
	require.Equal(t, "Remote search", run.Tools()[0].Description)
	require.Equal(t, "function", run.Tools()[0].Type)
}

func TestRunExecuteRoutesPrefixedToolAndFlattensResult(t *testing.T) {
	session := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"search","description":"Remote search","inputSchema":{"type":"object"}}`),
			},
		},
		callResult: mustResult(t, `{"content":[{"type":"text","text":"First line"},{"type":"text","text":"Second line"}],"structuredContent":{"source":"mcp"},"isError":false}`),
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{"github": session},
	})
	require.NoError(t, err)

	run, err := bridge.OpenSession(context.Background())
	require.NoError(t, err)

	output, handled, err := run.Execute(context.Background(), "github__search", `{"query":"hello"}`)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "First line\nSecond line\n{\"source\":\"mcp\"}", output)
	require.Len(t, session.callParams, 1)
	require.Equal(t, "search", session.callParams[0].Name)
	arguments, ok := session.callParams[0].Arguments.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "hello", arguments["query"])
}

func TestRunExecuteFormatsToolErrors(t *testing.T) {
	session := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"search","description":"Remote search","inputSchema":{"type":"object"}}`),
			},
		},
		callResult: mustResult(t, `{"content":[{"type":"text","text":"Permission denied"}],"isError":true}`),
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{"github": session},
	})
	require.NoError(t, err)

	run, err := bridge.OpenSession(context.Background())
	require.NoError(t, err)

	output, handled, err := run.Execute(context.Background(), "github__search", `{}`)
	require.NoError(t, err)
	require.True(t, handled)
	require.Equal(t, "MCP tool error: Permission denied", output)
}

func TestRunCloseClosesSessions(t *testing.T) {
	session := &stubSession{
		listResult: &mcp.ListToolsResult{},
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{"github": session},
	})
	require.NoError(t, err)

	run, err := bridge.OpenSession(context.Background())
	require.NoError(t, err)

	err = run.Close()
	require.NoError(t, err)
	require.True(t, session.closed)
}

func TestBridgeOpenRunSupportsMixedHTTPAndStdioServers(t *testing.T) {
	httpSession := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"search","description":"Remote search","inputSchema":{"type":"object"}}`),
			},
		},
	}
	stdioSession := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"read_file","description":"Read local file","inputSchema":{"type":"object"}}`),
			},
		},
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
			{Name: "filesystem", Enabled: true, Transport: "stdio", Stdio: &config.MCPStdioServer{Command: "npx", Args: []string{"server"}}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{
			"github":     httpSession,
			"filesystem": stdioSession,
		},
	})
	require.NoError(t, err)

	run, err := bridge.OpenSession(context.Background())
	require.NoError(t, err)
	require.Len(t, run.Tools(), 2)
	require.Equal(t, "github__search", run.Tools()[0].Name)
	require.Equal(t, "filesystem__read_file", run.Tools()[1].Name)
}

func TestBridgeOpenRunSkipsServerWithToolListingFailure(t *testing.T) {
	healthy := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"search","description":"Remote search","inputSchema":{"type":"object"}}`),
			},
		},
	}
	broken := &stubSession{listErr: errors.New("boom")}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
			{Name: "filesystem", Enabled: true, Transport: "stdio", Stdio: &config.MCPStdioServer{Command: "npx", Args: []string{"server"}}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{
			"github":     healthy,
			"filesystem": broken,
		},
	})
	require.NoError(t, err)

	run, err := bridge.OpenSession(context.Background())
	require.NoError(t, err)
	require.Len(t, run.Tools(), 1)
	require.Equal(t, "github__search", run.Tools()[0].Name)
	require.True(t, broken.closed)
}

func TestBridgePreflightChecksEnabledServers(t *testing.T) {
	healthy := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"search","description":"Remote search","inputSchema":{"type":"object"}}`),
			},
		},
	}
	filesystem := &stubSession{
		listResult: &mcp.ListToolsResult{
			Tools: []*mcp.Tool{
				mustTool(t, `{"name":"read_file","description":"Read file","inputSchema":{"type":"object"}}`),
			},
		},
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
			{Name: "filesystem", Enabled: true, Transport: "stdio", Stdio: &config.MCPStdioServer{Command: "npx", Args: []string{"server"}}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{
			"github":     healthy,
			"filesystem": filesystem,
		},
	})
	require.NoError(t, err)

	err = bridge.Preflight(context.Background())
	require.NoError(t, err)
	require.True(t, healthy.closed)
	require.True(t, filesystem.closed)
}

func TestBridgePreflightReturnsNamedServerError(t *testing.T) {
	healthy := &stubSession{
		listResult: &mcp.ListToolsResult{},
	}

	bridge, err := newBridge(config.MCPConfig{
		Servers: []*config.MCPServer{
			{Name: "github", Enabled: true, Transport: "streamable_http", HTTP: &config.MCPHTTPServer{URL: "https://example.com/mcp"}},
			{Name: "filesystem", Enabled: true, Transport: "stdio", Stdio: &config.MCPStdioServer{Command: "npx", Args: []string{"server"}}},
		},
	}, &stubConnector{
		sessions: map[string]*stubSession{
			"filesystem": healthy,
		},
		connectErrs: map[string]error{
			"github": errors.New("unauthorized"),
		},
	})
	require.NoError(t, err)

	err = bridge.Preflight(context.Background())
	require.Error(t, err)
	require.ErrorContains(t, err, `mcp server "github" connect: unauthorized`)
	require.True(t, healthy.closed)
}

func mustTool(t *testing.T, raw string) *mcp.Tool {
	t.Helper()

	var tool mcp.Tool
	err := json.Unmarshal([]byte(raw), &tool)
	require.NoError(t, err)

	return &tool
}

func mustResult(t *testing.T, raw string) *mcp.CallToolResult {
	t.Helper()

	var result mcp.CallToolResult
	err := json.Unmarshal([]byte(raw), &result)
	require.NoError(t, err)

	return &result
}
