package editor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"godai/mcp/jsonrpc"
)

// Client is a minimal MCP client that talks to the Godot editor over the
// addon's streamable HTTP transport (one JSON-RPC request per POST).
type Client struct {
	url    string
	httpc  *http.Client
	mu     sync.Mutex
	lastID int
}

func NewClient(url string) *Client {
	return &Client{
		url: url,
		httpc: &http.Client{
			// The editor closes the connection after every response.
			Transport: &http.Transport{DisableKeepAlives: true},
		},
	}
}

func (c *Client) post(ctx context.Context, req *jsonrpc.Request) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Close = true

	resp, err := c.httpc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %s: %s", resp.Status, respBody)
	}

	return respBody, nil
}

// Call sends a JSON-RPC request and returns the parsed response.
func (c *Client) Call(ctx context.Context, method string, params any) (*jsonrpc.Response, error) {
	c.mu.Lock()
	c.lastID++
	id := c.lastID
	c.mu.Unlock()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := c.post(ctx, jsonrpc.NewRequest(strconv.Itoa(id), method, paramsJSON))
	if err != nil {
		return nil, err
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("invalid JSON-RPC response %q: %w", respBody, err)
	}
	if resp.Error != nil {
		return &resp, fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
	}

	return &resp, nil
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}

	_, err = c.post(ctx, jsonrpc.NewNotification(method, paramsJSON))
	return err
}

type InitializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
	Capabilities map[string]json.RawMessage `json:"capabilities"`
}

// Initialize performs the MCP initialization handshake.
func (c *Client) Initialize(ctx context.Context) (*InitializeResult, error) {
	resp, err := c.Call(ctx, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
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

type ToolDef struct {
	Name         string         `json:"name"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	InputSchema  map[string]any `json:"inputSchema"`
	OutputSchema map[string]any `json:"outputSchema"`
}

func (c *Client) ListTools(ctx context.Context) ([]ToolDef, error) {
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

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

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

func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (*ToolCallResult, error) {
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
