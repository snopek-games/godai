package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/snopek-games/godai/mcp/jsonrpc"
	"gitlab.com/snopek-games/godai/mcp/server"
)

type ToolDef struct {
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
	Annotations  map[string]any `json:"annotations"`
}

func (d ToolDef) ValidateAnnotations() error {
	ann := d.Annotations
	if ann == nil {
		return fmt.Errorf("tool %s: missing annotations", d.Name)
	}
	if title, _ := ann["title"].(string); title != d.Title {
		return fmt.Errorf("tool %s: annotations.title = %v, want %q", d.Name, ann["title"], d.Title)
	}
	readOnly, ok := ann["readOnlyHint"].(bool)
	if !ok {
		return fmt.Errorf("tool %s: annotations.readOnlyHint missing or not a bool", d.Name)
	}
	if _, ok := ann["openWorldHint"].(bool); !ok {
		return fmt.Errorf("tool %s: annotations.openWorldHint missing or not a bool", d.Name)
	}
	_, hasDestructive := ann["destructiveHint"]
	_, hasIdempotent := ann["idempotentHint"]
	if readOnly {
		if hasDestructive || hasIdempotent {
			return fmt.Errorf("tool %s: read-only tool must not set destructiveHint/idempotentHint", d.Name)
		}
	} else if !hasDestructive || !hasIdempotent {
		return fmt.Errorf("tool %s: non-read-only tool must set destructiveHint and idempotentHint", d.Name)
	}
	return nil
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ToolCallResult struct {
	IsError           bool            `json:"isError"`
	Content           []ContentItem   `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
}

func (r *ToolCallResult) Text() string {
	var buf bytes.Buffer
	for _, item := range r.Content {
		buf.WriteString(item.Text)
	}
	return buf.String()
}

func (r *ToolCallResult) Structured() (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(r.StructuredContent, &m); err != nil {
		return nil, fmt.Errorf("structuredContent %q: %w", r.StructuredContent, err)
	}
	return m, nil
}

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

type transport interface {
	Call(ctx context.Context, method string, params any) (*jsonrpc.Response, error)
	Notify(ctx context.Context, method string, params any) error
	Close() error
}

type MCPClient struct {
	transport
	capabilities map[string]any
}

func (c *MCPClient) Initialize(ctx context.Context) (*InitializeResult, error) {
	return c.InitializeWithVersion(ctx, server.ProtocolVersion)
}

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
