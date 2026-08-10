package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/jsonrpc"
)

const ProtocolVersion string = core.EditorProtocolVersion

var supportedProtocolVersions = map[string]bool{
	"2025-06-18": true,
	"2025-11-25": true,
}

func negotiateProtocolVersion(requested string) string {
	if supportedProtocolVersions[requested] {
		return requested
	}
	return ProtocolVersion
}

var GodaiVersion string = core.Version

const GodaiMcpName string = core.AppName
const GodaiMcpTitle string = core.AppTitle
const GodaiMcpInstructions string = `Open and control the Godot editor: inspect and edit the scene tree, node properties, scripts, resources, and project settings. Prefer these tools over editing .tscn/.tres/.gd files on disk - the editor owns that state and direct file edits can be clobbered or rejected.

Most tools require a project_path; call list_open_projects (or open_godot_project) first.

Before any add/remove/edit, read get_current_scene_tree and only use node paths you've seen there. Node and scene edits are in-memory until save_scene; scripts, resources, and project settings persist on their own (for an open script, use save_script). Most edits use the editor's undo/redo, so batch related changes into one call.`

const TooManyToolCallsErrorCode jsonrpc.ErrorCode = jsonrpc.ServerErrorMinCode - 0
const RequestQueueFullErrorCode jsonrpc.ErrorCode = jsonrpc.ServerErrorMinCode - 1

type appInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ClientInfo      appInfo        `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ServerInfo      appInfo        `json:"serverInfo"`
	Instructions    string         `json:"instructions"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      map[string]any  `json:"_meta,omitempty"`
}

type toolTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolResult struct {
	Content           []toolTextContent `json:"content"`
	StructuredContent json.RawMessage   `json:"structuredContent,omitempty"`
	IsError           bool              `json:"isError,omitempty"`
}

func toolResultForError(err *core.UserError) *toolResult {
	result := toolResult{
		Content: []toolTextContent{
			{
				Type: "text",
				Text: err.Message,
			},
		},
		IsError: true,
	}
	if len(err.Solutions) > 0 {
		sb := strings.Builder{}
		sb.WriteString("Possible solutions:\n")
		for _, ps := range err.Solutions {
			sb.WriteString("- ")
			sb.WriteString(ps)
			sb.WriteRune('\n')
		}
		result.Content = append(result.Content, toolTextContent{
			Type: "text",
			Text: sb.String(),
		})
	}
	return &result
}

type ToolHandler func(ctx context.Context, args core.Args) (any, error)

type Tool struct {
	Definition *core.ToolDefinition
	Handler    ToolHandler
	Override   bool
}

type clientInfo struct {
	appInfo         appInfo
	rawCapabilities map[string]any
	// These are only the capabilities we care about.
	capabilities struct {
		formElicitation  bool
		roots            bool
		rootsListChanged bool
	}
}

type Server struct {
	session            *core.Session
	writeCh            chan []byte
	jsonrpcDispatcher  *jsonrpc.Dispatcher
	clientInfo         clientInfo
	clientInfoMutex    sync.RWMutex
	clientRequests     map[int]chan *jsonrpc.Response
	clientRequestID    int
	clientRequestMutex sync.Mutex
	toolQueueCh        chan *jsonrpc.Request
	requestQueueCh     chan *jsonrpc.Request
	localTools         map[string]*Tool
	roots              []string
	rootsMutex         sync.RWMutex
}

func NewServer(session *core.Session) *Server {
	d := jsonrpc.NewDispatcher()

	s := &Server{
		session:           session,
		writeCh:           make(chan []byte, 16),
		clientRequests:    make(map[int]chan *jsonrpc.Response),
		jsonrpcDispatcher: d,
		toolQueueCh:       make(chan *jsonrpc.Request, 4),
		requestQueueCh:    make(chan *jsonrpc.Request, 16),
		localTools:        make(map[string]*Tool),
		roots:             session.Config().RootPaths,
	}

	session.SetRootsProvider(s.getRootPaths)
	session.SetClientInfoProvider(s.getClientInitializeParams)
	session.SetPrompter(&elicitPrompter{server: s})

	d.Register("initialize", s.rpcInitialize)
	d.Register("notifications/initialized", s.rpcClientInitialized)
	d.Register("notifications/roots/list_changed", s.rpcRootsListChanged)
	d.Register("tools/list", s.rpcListTools)
	d.Register("tools/call", s.rpcCallTool)

	s.setupLocalTools()

	return s
}

// The following accessors guard all reads and writes of s.clientInfo, which is
// written once on rpcInitialize() (the requestLoop goroutine) but read and
// occasionally updated from background goroutines (the editor scanLoop via
// getRootPaths(), and the session's connect hook).

func (s *Server) clientSupportsFormElicitation() bool {
	s.clientInfoMutex.RLock()
	defer s.clientInfoMutex.RUnlock()
	return s.clientInfo.capabilities.formElicitation
}

func (s *Server) clientSupportsRoots() bool {
	s.clientInfoMutex.RLock()
	defer s.clientInfoMutex.RUnlock()
	return s.clientInfo.capabilities.roots
}

// getClientRootsCapabilities reports whether the client supports roots and, if so,
// whether it also emits listChanged notifications.
func (s *Server) getClientRootsCapabilities() (roots, listChanged bool) {
	s.clientInfoMutex.RLock()
	defer s.clientInfoMutex.RUnlock()
	return s.clientInfo.capabilities.roots, s.clientInfo.capabilities.rootsListChanged
}

// setClientRootsUnsupported records that the client doesn't actually support roots,
// despite having advertised the capability.
func (s *Server) setClientRootsUnsupported() {
	s.clientInfoMutex.Lock()
	defer s.clientInfoMutex.Unlock()
	s.clientInfo.capabilities.roots = false
}

func (s *Server) getClientInitializeParams() (core.ClientInfo, map[string]any) {
	s.clientInfoMutex.RLock()
	defer s.clientInfoMutex.RUnlock()
	return core.ClientInfo(s.clientInfo.appInfo), s.clientInfo.rawCapabilities
}

func (s *Server) getRootPaths() []string {
	// If the client supports "roots" but not "listChanged", then refresh our list of root paths.
	if roots, listChanged := s.getClientRootsCapabilities(); roots && !listChanged {
		s.listRoots()
	}

	s.rootsMutex.RLock()
	defer s.rootsMutex.RUnlock()

	return slices.Clone(s.roots)
}

func (s *Server) rpcInitialize(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	var params initializeParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

	s.clientInfoMutex.Lock()

	s.clientInfo.appInfo = params.ClientInfo
	s.clientInfo.rawCapabilities = params.Capabilities

	slog.Info("client connected", "appInfo", s.clientInfo.appInfo, "capabilities", s.clientInfo.rawCapabilities)

	// Check if the client supports form elicitation.
	elicitation, ok := params.Capabilities["elicitation"]
	if ok {
		v, ok := elicitation.(map[string]any)
		if ok {
			isEmpty := (len(v) == 0)
			_, hasForm := v["form"]

			s.clientInfo.capabilities.formElicitation = isEmpty || hasForm
		}
	}
	if s.clientInfo.capabilities.formElicitation {
		slog.Info("client supports form elicitation")
	}

	// Check if the client supports roots.
	roots, ok := params.Capabilities["roots"]
	if ok {
		v, ok := roots.(map[string]any)
		if ok {
			s.clientInfo.capabilities.roots = true

			listChanged, ok := v["listChanged"]
			if ok {
				listChangedV, _ := listChanged.(bool)
				s.clientInfo.capabilities.rootsListChanged = listChangedV
			}
		}
	}
	if s.clientInfo.capabilities.roots {
		slog.Info(fmt.Sprintf("client supports roots (listChanged = %t)", s.clientInfo.capabilities.rootsListChanged))
	}

	s.clientInfoMutex.Unlock()

	// Start scanning only after releasing the lock: it spawns the scanLoop,
	// which reads clientInfo via getRootPaths().
	if err := s.session.Start(ctx); err != nil {
		slog.Error("unable to start scanning for Godot editors", "error", err)
	}

	response := initializeResult{
		ProtocolVersion: negotiateProtocolVersion(params.ProtocolVersion),
		Capabilities: map[string]any{
			"tools": map[string]any{},
		},
		ServerInfo: appInfo{
			Name:    GodaiMcpName,
			Title:   GodaiMcpTitle,
			Version: GodaiVersion,
		},
		Instructions: GodaiMcpInstructions,
	}

	return response, nil
}

func (s *Server) rpcClientInitialized(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	if s.clientSupportsRoots() {
		s.listRoots()
	}
	return nil, nil
}

func (s *Server) rpcRootsListChanged(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	s.listRoots()
	return nil, nil
}

const rootsListTimeout = 10 * time.Second

func (s *Server) listRootsInternal() error {
	resp, err := s.sendRequestToClientWithTimeout("roots/list", nil, rootsListTimeout)
	if err != nil {
		return err
	}

	if resp.Error != nil {
		if resp.Error.Code == -32601 {
			// Server reported capabilities to us incorrectly?
			s.setClientRootsUnsupported()
			return errors.New("client does not support roots")
		}
		return fmt.Errorf("error code %d: %s", resp.Error.Code, resp.Error.Message)
	}

	var result struct {
		Roots []struct {
			Uri  string `json:"uri"`
			Name string `json:"name"`
		} `json:"roots"`
	}

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return err
	}

	var rootPaths []string
	for _, root := range result.Roots {
		rootPaths = append(rootPaths, fileURIToPath(root.Uri))
	}

	s.rootsMutex.Lock()
	s.roots = rootPaths
	s.rootsMutex.Unlock()

	slog.Debug(fmt.Sprintf("updated roots: %+v", rootPaths))

	return nil
}

// fileURIToPath converts a "file://" URI (as sent by MCP clients for roots)
// into a native filesystem path. It percent-decodes the path and handles
// Windows drive-letter URIs like "file:///C:/Users/foo" (which parse to
// "/C:/Users/foo"). Anything that isn't a file URI is returned unchanged, so
// plain paths still pass through.
func fileURIToPath(uri string) string {
	if !strings.HasPrefix(uri, "file:") {
		return uri
	}

	u, err := url.Parse(uri)
	if err != nil {
		// Fall back to the naive prefix strip rather than dropping the root.
		return strings.TrimPrefix(uri, "file://")
	}

	// u.Path is already percent-decoded by url.Parse.
	path := u.Path
	if path == "" {
		return uri
	}

	// On Windows, "file:///C:/foo" yields "/C:/foo"; strip the leading slash so
	// it's a valid drive path, and convert forward slashes to backslashes.
	if runtime.GOOS == "windows" {
		path = strings.TrimPrefix(path, "/")
		path = filepath.FromSlash(path)
	}

	return path
}

func (s *Server) listRoots() {
	err := s.listRootsInternal()
	if err != nil {
		slog.Error("error listing client roots", "error", err)
	}
}

func (s *Server) sendRequestToClient(method string, params any) (*jsonrpc.Response, error) {
	// No timeout: some requests (e.g. elicitation) legitimately wait on user input.
	return s.sendRequestToClientWithTimeout(method, params, 0)
}

func (s *Server) sendRequestToClientWithTimeout(method string, params any, timeout time.Duration) (*jsonrpc.Response, error) {
	ch := make(chan *jsonrpc.Response, 1)

	s.clientRequestMutex.Lock()
	s.clientRequestID++
	id := s.clientRequestID
	s.clientRequests[id] = ch
	s.clientRequestMutex.Unlock()

	req := jsonrpc.NewRequest(strconv.Itoa(id), method, nil)
	if params != nil {
		b, err := json.Marshal(&params)
		if err != nil {
			s.discardClientRequest(id)
			return nil, err
		}
		req.Params = json.RawMessage(b)
	}

	b, err := json.Marshal(&req)
	if err != nil {
		s.discardClientRequest(id)
		return nil, err
	}

	s.writeCh <- b

	if timeout <= 0 {
		return <-ch, nil
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case resp := <-ch:
		return resp, nil
	case <-timer.C:
		s.discardClientRequest(id)
		return nil, fmt.Errorf("timed out after %s waiting for client response to %q", timeout, method)
	}
}

// discardClientRequest drops a pending client request so a late or missing
// response doesn't leak the map entry. Safe to call even if the response
// already arrived (the response channel is buffered, so the router never
// blocks on a discarded request).
func (s *Server) discardClientRequest(id int) {
	s.clientRequestMutex.Lock()
	delete(s.clientRequests, id)
	s.clientRequestMutex.Unlock()
}

func (s *Server) handleResponse(resp *jsonrpc.Response) {
	idStr, ok := resp.GetID()
	if !ok {
		slog.Error("unable to parse response ID from MCP client", "id", string(resp.ID))
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		slog.Error("unable to parse response ID from MCP client", "id", idStr)
		return
	}

	s.clientRequestMutex.Lock()
	ch, ok := s.clientRequests[id]
	if ok {
		delete(s.clientRequests, id)
	}
	s.clientRequestMutex.Unlock()

	if ok {
		ch <- resp
		close(ch)
	}
}

func parseRequestOrResponse(b []byte) (any, error) {
	// Has fields for both request and response.
	var input struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		// Request:
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
		// Response:
		Result json.RawMessage `json:"result"`
		Error  *jsonrpc.Error  `json:"error"`
	}
	if err := json.Unmarshal(b, &input); err != nil {
		return nil, err
	}

	if input.Method != "" {
		if input.Error != nil || len(input.Result) != 0 {
			return jsonrpc.Request{JSONRPC: "Invalid", ID: jsonrpc.NullID()}, nil
		}
		return jsonrpc.Request{
			JSONRPC: input.JSONRPC,
			ID:      input.ID,
			Method:  input.Method,
			Params:  input.Params,
		}, nil
	}

	return jsonrpc.Response{
		JSONRPC: input.JSONRPC,
		ID:      input.ID,
		Result:  input.Result,
		Error:   input.Error,
	}, nil
}

func (s *Server) writeLoop(ctx context.Context) {
	w := bufio.NewWriter(os.Stdout)
	for {
		select {
		case <-ctx.Done():
			return
		case line := <-s.writeCh:
			w.Write(line)
			w.WriteRune('\n')
			w.Flush()
		}
	}
}

func (s *Server) toolLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-s.toolQueueCh:
			s.handleRequest(ctx, req)
		}
	}
}

// requestLoop handles all non-tool requests on its own goroutine, off the
// readLoop, so handlers may safely call sendRequestToClient() without
// blocking the goroutine that reads the client's responses.
func (s *Server) requestLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case req := <-s.requestQueueCh:
			s.handleRequest(ctx, req)
		}
	}
}

// Enqueues a request on the channel, returning an error response if it's full.
func (s *Server) enqueueRequest(ch chan *jsonrpc.Request, req *jsonrpc.Request, errCode jsonrpc.ErrorCode, errMsg string) {
	select {
	case ch <- req:
		// Queued, there was space.
	default:
		if !req.HasID() {
			slog.Warn("dropping notification, queue full", "method", req.Method)
			return
		}
		resp := jsonrpc.NewErrorResponse(req.ID, jsonrpc.NewError(errCode, errMsg, nil))
		b, err := json.Marshal(resp)
		if err != nil {
			slog.Error("error marshalling response to stdout", "response", resp)
			return
		}
		s.writeCh <- b
	}
}

func (s *Server) handleRequest(ctx context.Context, req *jsonrpc.Request) {
	resp := s.jsonrpcDispatcher.HandleRequest(ctx, req)
	if req.HasID() {
		output, err := json.Marshal(resp)
		if err != nil {
			slog.Error("error marshalling response to stdout", "response", resp)
			return
		}
		if len(output) > 0 {
			s.writeCh <- output
		}
	}
}

func (s *Server) readLoop(ctx context.Context) error {
	r := bufio.NewReader(os.Stdin)

	lineCh := make(chan []byte)
	errCh := make(chan error, 1)

	// This goroutine will get cleaned up when Stdin closes at process exit.
	go func() {
		for {
			line, err := r.ReadBytes('\n')
			if err != nil {
				errCh <- err
				return
			}
			lineCh <- line
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case err := <-errCh:
			if err != io.EOF {
				return err
			}
			return nil

		case line := <-lineCh:
			input, err := parseRequestOrResponse(line)
			if err != nil {
				slog.Error("error parsing line from stdin", "line", line, "error", err)
				continue
			}

			switch v := input.(type) {
			case jsonrpc.Request:
				req := v
				// Tools need to be executed one-at-a-time, so they get their
				// own queue; all other requests share requestQueueCh. Both
				// sends are non-blocking so readLoop stays free to process
				// client responses.
				if req.Method == "tools/call" {
					s.enqueueRequest(s.toolQueueCh, &req, TooManyToolCallsErrorCode, "Too many simultaneous tool calls")
				} else {
					s.enqueueRequest(s.requestQueueCh, &req, RequestQueueFullErrorCode, "Server busy: too many pending requests")
				}
			case jsonrpc.Response:
				resp := v
				if !resp.IsValid() {
					slog.Error("received invalid response from stdin", "response", r)
					continue
				}
				s.handleResponse(&resp)
			default:
				continue
			}
		}
	}
}

func (s *Server) Run(ctx context.Context) error {
	go s.writeLoop(ctx)
	go s.toolLoop(ctx)
	go s.requestLoop(ctx)
	go s.session.CheckForUpdate(ctx)

	err := s.readLoop(ctx)
	s.session.Close()
	return err
}
