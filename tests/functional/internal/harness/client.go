package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"godai/mcp/jsonrpc"
	"godai/mcp/server"
)

// ToolDef is a tool entry from tools/list.
type ToolDef struct {
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
}

// ContentItem is one item of a tool result's content array.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ToolCallResult is the result of a tools/call.
type ToolCallResult struct {
	IsError           bool            `json:"isError"`
	Content           []ContentItem   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

// Text returns the concatenated text content of the result.
func (r *ToolCallResult) Text() string {
	var buf bytes.Buffer
	for _, item := range r.Content {
		buf.WriteString(item.Text)
	}
	return buf.String()
}

// Structured decodes the structuredContent into a map.
func (r *ToolCallResult) Structured() (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(r.StructuredContent, &m); err != nil {
		return nil, fmt.Errorf("structuredContent %q: %w", r.StructuredContent, err)
	}
	return m, nil
}

// InitializeResult is the result of the initialize handshake.
type InitializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Title   string `json:"title"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	Capabilities map[string]json.RawMessage `json:"capabilities"`
	Instructions string                     `json:"instructions"`
}

type RequestHandler func(params json.RawMessage) (result any, rpcErr *jsonrpc.Error)

type ClientConfig struct {
	Capabilities map[string]any
	Handlers     map[string]RequestHandler
}

// transport is the underlying JSON-RPC channel an MCPClient speaks over. The
// editor tests use an HTTP transport (one request per POST); the Go MCP server
// tests use a stdio transport (line-delimited JSON over the process's pipes).
type transport interface {
	// Call sends a request and waits for its response.
	Call(ctx context.Context, method string, params any) (*jsonrpc.Response, error)
	// Notify sends a notification (no response expected).
	Notify(ctx context.Context, method string, params any) error
	// Close releases any resources held by the transport.
	Close() error
}

// MCPClient is a minimal MCP client that works over any transport.
type MCPClient struct {
	transport
	capabilities map[string]any
}

// Initialize performs the MCP initialization handshake, advertising the
// client's configured capabilities (none by default, so the server relies on
// its own configuration rather than calling back to us).
func (c *MCPClient) Initialize(ctx context.Context) (*InitializeResult, error) {
	return c.InitializeWithVersion(ctx, server.ProtocolVersion)
}

// InitializeWithVersion is like Initialize but requests a specific protocol
// version, so tests can exercise version negotiation.
func (c *MCPClient) InitializeWithVersion(ctx context.Context, protocolVersion string) (*InitializeResult, error) {
	capabilities := c.capabilities
	if capabilities == nil {
		capabilities = map[string]any{}
	}

	resp, err := c.Call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    capabilities,
		"clientInfo": map[string]any{
			"name":    "godai-functional-tests",
			"version": "1.0",
		},
	})
	if err != nil {
		return nil, err
	}

	var result InitializeResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	if err := c.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return nil, err
	}

	return &result, nil
}

// ListTools returns the tools advertised by tools/list.
func (c *MCPClient) ListTools(ctx context.Context) ([]ToolDef, error) {
	resp, err := c.Call(ctx, "tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}

	var result struct {
		Tools []ToolDef `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

// CallTool invokes a tool and decodes its result.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (*ToolCallResult, error) {
	if args == nil {
		args = map[string]any{}
	}

	resp, err := c.Call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}

	var result ToolCallResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("invalid tools/call result %q: %w", resp.Result, err)
	}
	return &result, nil
}
