package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"

	"gitlab.com/snopek-games/godai/internal/jsonrpc"
)

// httpTransport talks to the Godot editor's HTTP transport, one JSON-RPC
// request per POST.
type httpTransport struct {
	url    string
	httpc  *http.Client
	mu     sync.Mutex
	lastID int
}

func NewHTTPClient(url string) *MCPClient {
	return &MCPClient{transport: &httpTransport{
		url: url,
		httpc: &http.Client{
			// The editor closes the connection after every response.
			Transport: &http.Transport{DisableKeepAlives: true},
		},
	}}
}

func (t *httpTransport) post(ctx context.Context, req *jsonrpc.Request) ([]byte, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Close = true

	resp, err := t.httpc.Do(httpReq)
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

func (t *httpTransport) Call(ctx context.Context, method string, params any) (*jsonrpc.Response, error) {
	t.mu.Lock()
	t.lastID++
	id := t.lastID
	t.mu.Unlock()

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}

	respBody, err := t.post(ctx, jsonrpc.NewRequest(strconv.Itoa(id), method, paramsJSON))
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

func (t *httpTransport) Notify(ctx context.Context, method string, params any) error {
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return err
	}

	_, err = t.post(ctx, jsonrpc.NewNotification(method, paramsJSON))
	return err
}

func (t *httpTransport) Close() error {
	t.httpc.CloseIdleConnections()
	return nil
}
