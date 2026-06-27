package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"

	"godai/mcp/jsonrpc"
)

// stdioTransport speaks line-delimited JSON-RPC over a process's stdin/stdout.
// A background reader routes each response to the goroutine waiting on its ID.
type stdioTransport struct {
	w io.Writer

	writeMu sync.Mutex

	idMu   sync.Mutex
	lastID int

	mu      sync.Mutex
	pending map[int]chan *jsonrpc.Response
	closed  bool

	handlers map[string]RequestHandler
}

func NewStdioClient(stdin io.Writer, stdout io.Reader) *MCPClient {
	return NewStdioClientWithConfig(stdin, stdout, ClientConfig{})
}

func NewStdioClientWithConfig(stdin io.Writer, stdout io.Reader, cfg ClientConfig) *MCPClient {
	t := &stdioTransport{
		w:        stdin,
		pending:  make(map[int]chan *jsonrpc.Response),
		handlers: cfg.Handlers,
	}
	go t.readLoop(stdout)
	return &MCPClient{transport: t, capabilities: cfg.Capabilities}
}

func (t *stdioTransport) readLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Tool results (e.g. list_projects) can be large; allow generous lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()

		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Result json.RawMessage `json:"result"`
			Error  *jsonrpc.Error  `json:"error"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		if msg.Method != "" {
			if len(msg.ID) > 0 {
				t.handleServerRequest(msg.Method, msg.ID, msg.Params)
			}
			continue
		}

		resp := &jsonrpc.Response{Result: msg.Result, Error: msg.Error, ID: msg.ID}
		idStr, ok := resp.GetID()
		if !ok {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue
		}

		t.mu.Lock()
		ch, ok := t.pending[id]
		if ok {
			delete(t.pending, id)
		}
		t.mu.Unlock()
		if ok {
			ch <- resp
		}
	}

	// The server's stdout closed: fail every in-flight call.
	t.mu.Lock()
	t.closed = true
	for id, ch := range t.pending {
		close(ch)
		delete(t.pending, id)
	}
	t.mu.Unlock()
}

func (t *stdioTransport) handleServerRequest(method string, id, params json.RawMessage) {
	handler := t.handlers[method]
	if handler == nil {
		t.write(jsonrpc.NewErrorResponse(id,
			jsonrpc.NewError(jsonrpc.MethodNotFoundErrorCode, "client does not handle "+method, nil)))
		return
	}

	result, rpcErr := handler(params)
	if rpcErr != nil {
		t.write(jsonrpc.NewErrorResponse(id, rpcErr))
		return
	}

	resp := jsonrpc.NewResponse(id)
	resp.SetResult(result)
	t.write(resp)
}

func (t *stdioTransport) write(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if _, err := t.w.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (t *stdioTransport) Call(ctx context.Context, method string, params any) (*jsonrpc.Response, error) {
	t.idMu.Lock()
	t.lastID++
	id := t.lastID
	t.idMu.Unlock()

	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		paramsJSON = b
	}

	ch := make(chan *jsonrpc.Response, 1)
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil, fmt.Errorf("stdio transport is closed")
	}
	t.pending[id] = ch
	t.mu.Unlock()

	if err := t.write(jsonrpc.NewRequest(strconv.Itoa(id), method, paramsJSON)); err != nil {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return nil, err
	}

	select {
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("server closed before responding to %s", method)
		}
		if resp.Error != nil {
			return resp, fmt.Errorf("JSON-RPC error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp, nil
	}
}

func (t *stdioTransport) Notify(ctx context.Context, method string, params any) error {
	var paramsJSON json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		paramsJSON = b
	}
	return t.write(jsonrpc.NewNotification(method, paramsJSON))
}

func (t *stdioTransport) Close() error {
	return nil
}
