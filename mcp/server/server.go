package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"godai/mcp/godot"
	"godai/mcp/jsonrpc"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const ProtocolVersion string = "2025-06-18"

// @todo Should we read this from the plugin.cfg?
const GodaiVersion string = "0.1.0"

const TooManyToolCallsErrorCode jsonrpc.ErrorCode = jsonrpc.ServerErrorMinCode

type appInfo struct {
	Name    string `json:"name"`
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
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
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

type userVisibleError struct {
	message           string
	possibleSolutions []string
	err               error
}

type userErrorResponse struct {
	Success bool `json:"success"`
}

func newUserVisibleError(message string, err error, possibleSolutions []string) *userVisibleError {
	return &userVisibleError{
		message:           message,
		possibleSolutions: possibleSolutions,
		err:               err,
	}
}

func (err *userVisibleError) Error() string {
	if err.err != nil {
		return fmt.Sprintf("%s: %v", err.message, err.err)
	} else {
		return err.message
	}
}

func (err *userVisibleError) Unwrap() error {
	return err.err
}

func (err *userVisibleError) makeToolResult() *toolResult {
	result := toolResult{
		Content: []toolTextContent{
			{
				Type: "text",
				Text: err.message,
			},
		},
		IsError: true,
	}
	if len(err.possibleSolutions) > 0 {
		sb := strings.Builder{}
		sb.WriteString("Possible solutions:\n")
		for _, ps := range err.possibleSolutions {
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

type ToolHandler func(ctx context.Context, params json.RawMessage) (any, error)

type Tool struct {
	Definition *ToolDefinition
	Handler    ToolHandler
	Override   bool
}

type editorInfo struct {
	ProjectPath string
	ProjectName string
	Connection  *godot.Connection
}

type clientInfo struct {
	appInfo         appInfo
	rawCapabilities map[string]any
	// These are only the capabilities we care about.
	capabilities struct {
		formElicitation bool
	}
}

type Server struct {
	config             *Config
	writeCh            chan []byte
	jsonrpcDispatcher  *jsonrpc.Dispatcher
	connectionManager  *godot.ConnectionManager
	clientInfo         clientInfo
	clientRequests     map[int]chan *jsonrpc.Response
	clientRequestID    int
	clientRequestMutex sync.Mutex
	toolQueueCh        chan *jsonrpc.Request
	localTools         map[string]*Tool
	currentProjectPath string
	editors            []*editorInfo
	editorsMutex       sync.RWMutex
}

func NewServer(config *Config) *Server {
	d := jsonrpc.NewDispatcher()

	s := &Server{
		config:            config,
		writeCh:           make(chan []byte, 16),
		clientRequests:    make(map[int]chan *jsonrpc.Response),
		jsonrpcDispatcher: d,
		toolQueueCh:       make(chan *jsonrpc.Request, 4),
		localTools:        make(map[string]*Tool),
		editors:           make([]*editorInfo, 0, config.EditorPortCount),
	}

	s.connectionManager = godot.NewConnectionManager(godot.ConnectionManagerConfig{
		BasePort:     config.EditorBasePort,
		PortCount:    config.EditorPortCount,
		RetryDelay:   config.EditorRetryDelay,
		OnConnect:    s.onEditorConnect,
		OnDisconnect: s.onEditorDisconnect,
	})

	d.Register("initialize", s.rpcInitialize)
	d.Register("notifications/initialized", s.rpcClientInitialized)
	d.Register("tools/list", s.rpcListTools)
	d.Register("tools/call", s.rpcCallTool)

	s.setupLocalTools()

	return s

}

func (s *Server) saveConfig() error {
	if s.config.SavedConfigPath != "" {
		sc := &SavedConfig{
			DefaultGodotPath: s.config.DefaultGodotPath,
			ProjectBasePath:  s.config.ProjectBasePath,
		}
		return SaveConfig(s.config.SavedConfigPath, sc)
	}
	return nil
}

func canonicalPath(p string) (string, error) {
	if p == "" {
		return "", errors.New("empty path")
	}

	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			p = filepath.Join(home, p[2:])
		}
	}

	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}

	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}

	return real, nil
}

func ValidateDirectory(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !fi.IsDir() {
		return errors.New("not a directory")
	}

	return nil
}

func ValidateGodotExecutable(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	if fi.IsDir() || !fi.Mode().IsRegular() {
		return errors.New("not a regular file")
	}

	cmd := exec.Command(path, "--version")
	err = cmd.Run()
	return err
}

func (s *Server) onEditorConnect(conn *godot.Connection) error {
	params := initializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      s.clientInfo.appInfo,
		Capabilities:    s.clientInfo.rawCapabilities,
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.EditorTimeout)
	defer cancel()

	if _, err := conn.CallMethod(ctx, "initialize", params); err != nil {
		return err
	}
	if err := conn.SendNotification(ctx, "notification/initialized", nil); err != nil {
		return err
	}

	var projectInfo struct {
		ProjectPath string `json:"project_path"`
		ProjectName string `json:"project_name"`
	}
	if err := callEditorTool(ctx, conn, "get_current_project", json.RawMessage("{}"), &projectInfo); err != nil {
		return err
	}

	realProjectPath, err := canonicalPath(projectInfo.ProjectPath)
	if err != nil {
		return err
	}

	editor := &editorInfo{
		Connection:  conn,
		ProjectPath: realProjectPath,
		ProjectName: projectInfo.ProjectName,
	}

	s.editorsMutex.Lock()
	s.editors = append(s.editors, editor)
	s.editorsMutex.Unlock()

	return nil
}

func callEditorTool(ctx context.Context, conn *godot.Connection, name string, params json.RawMessage, ret any) error {
	callParams := callToolParams{
		Name:      name,
		Arguments: params,
	}
	resp, err := conn.CallMethod(ctx, "tools/call", callParams)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("error calling tool: %v", resp.Error)
	}

	var toolResult toolResult
	if err := json.Unmarshal(resp.Result, &toolResult); err != nil {
		return fmt.Errorf("error parsing tool result: %v", resp.Result)
	}

	if toolResult.StructuredContent != nil {
		if err := json.Unmarshal(toolResult.StructuredContent, ret); err != nil {
			return err
		}
	} else if len(toolResult.Content) > 0 {
		if err := json.Unmarshal([]byte(toolResult.Content[0].Text), ret); err != nil {
			return err
		}
	}

	return nil
}

func (s *Server) onEditorDisconnect(conn *godot.Connection) {
	s.editorsMutex.Lock()
	defer s.editorsMutex.Unlock()

	// Filter out the removed connection.
	newEditors := make([]*editorInfo, 0, s.config.EditorPortCount)
	for _, e := range s.editors {
		if e.Connection != conn {
			newEditors = append(newEditors, e)
		}
	}
	s.editors = newEditors
}

func (s *Server) hasEditorForProject(projectPath string) bool {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	for _, e := range s.editors {
		if e.ProjectPath == projectPath {
			return true
		}
	}

	return false
}

func (s *Server) getCurrentEditor() (*editorInfo, error) {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	if len(s.editors) == 0 {
		return nil, newUserVisibleError("not connected to any Godot editors", nil, []string{
			"Open a project in the editor using the `open_godot_project` tool",
		})
	}

	if s.currentProjectPath == "" {
		e := s.editors[0]
		s.currentProjectPath = e.ProjectPath
		return e, nil
	}

	for _, e := range s.editors {
		if e.ProjectPath == s.currentProjectPath {
			return e, nil
		}
	}

	return nil, newUserVisibleError("no longer connected to the Godot editor for current project: "+s.currentProjectPath, nil, []string{
		"List other open projects using the `list_open_projects` tool and switch to one using `switch_to_project`",
		"Re-open the Godot editor for the previous project using the `open_godot_project` tool",
	})
}

func (s *Server) getCurrentEditorConnection() (*godot.Connection, error) {
	e, err := s.getCurrentEditor()
	if err != nil {
		return nil, err
	}
	return e.Connection, nil
}

func (s *Server) rpcInitialize(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	var params initializeParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

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

	s.connectionManager.Start()

	response := initializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]any{
			"tools": map[string]any{},
		},
		ServerInfo: appInfo{
			Name:    "Godai",
			Version: GodaiVersion,
		},
	}

	return response, nil
}

func (s *Server) rpcClientInitialized(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	return nil, nil
}

func (s *Server) rpcListTools(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	type toolOut struct {
		Name         string          `json:"name"`
		Title        string          `json:"title"`
		Description  string          `json:"description"`
		InputSchema  json.RawMessage `json:"inputSchema"`
		OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	}

	list := []toolOut{}
	for name, tool := range s.localTools {
		if tool.Override {
			// This is a local "override", so the definition will come from the remote definitions.
			continue
		}
		toolDef := tool.Definition
		out := toolOut{
			Name:         name,
			Title:        toolDef.Title,
			Description:  toolDef.GetDescription(),
			InputSchema:  toolDef.GetInputSchema(),
			OutputSchema: toolDef.GetOutputSchema(),
		}
		list = append(list, out)
	}
	for name, toolDef := range GetDefaultRemoteToolDefinitions() {
		out := toolOut{
			Name:         name,
			Title:        toolDef.Title,
			Description:  toolDef.GetDescription(),
			InputSchema:  toolDef.GetInputSchema(),
			OutputSchema: toolDef.GetOutputSchema(),
		}
		list = append(list, out)
	}

	var resp struct {
		Tools []toolOut `json:"tools"`
	}
	resp.Tools = list

	return resp, nil
}

func (s *Server) rpcCallTool(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	var params callToolParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

	// Run locally if this is a local tool.
	tool, ok := s.localTools[params.Name]
	if ok {
		result, err := tool.Handler(ctx, params.Arguments)
		if err != nil {
			slog.Error("error running local tool", "toolName", params.Name, "error", err)

			var userError *userVisibleError
			if errors.As(err, &userError) {
				return userError.makeToolResult(), nil
			} else {
				return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Error running local tool", nil)
			}
		}

		var structuredContent any

		text, ok := result.(string)
		if !ok {
			structuredContent = result
			b, err := json.Marshal(result)
			if err != nil {
				slog.Error("error marshalling local tool output to JSON", "toolName", params.Name, "error", err)
				return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Error running local tool", nil)
			}
			text = string(b)
		}

		var structedContentJSON json.RawMessage
		if structuredContent != nil {
			b, err := json.Marshal(structuredContent)
			if err != nil {
				slog.Error("error marshalling local tool structuredContent to JSON", "toolName", params.Name, "error", err)
				return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Error running local tool", nil)
			}
			structedContentJSON = json.RawMessage(b)
		}

		output := toolResult{
			Content: []toolTextContent{
				{
					Type: "text",
					Text: text,
				},
			},
			StructuredContent: structedContentJSON,
		}
		return output, nil
	}

	conn, err := s.getCurrentEditorConnection()
	if err != nil {
		var userError *userVisibleError
		if errors.As(err, &userError) {
			return userError.makeToolResult(), nil
		} else {
			return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, err.Error(), nil)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, s.config.EditorTimeout)
	defer cancel()

	resp, err := conn.CallMethod(ctx, "tools/call", params)
	if err != nil {
		slog.Error("error running remote tool", "toolName", params.Name, "error", err)
		return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Unable to call method on Godot editor", nil)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

// Not safe to call from any `rpc*()“ functions (will deadlock), except for rpcCallTool() because it has a special queue.
func (s *Server) sendRequestToClient(method string, params any) (*jsonrpc.Response, error) {
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
			return nil, err
		}
		req.Params = json.RawMessage(b)
	}

	b, err := json.Marshal(&req)
	if err != nil {
		return nil, err
	}

	s.writeCh <- b

	// @todo Having a timeout would be good, although, tricky because elicitation waits for user input
	resp := <-ch

	return resp, nil
}

func (s *Server) sendNotificationToClient(method string, params any) error {
	req := jsonrpc.NewNotification(method, nil)
	if params != nil {
		b, err := json.Marshal(&params)
		if err != nil {
			return err
		}
		req.Params = json.RawMessage(b)
	}

	b, err := json.Marshal(&req)
	if err != nil {
		return err
	}

	s.writeCh <- b
	return nil
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
				// Tools need to be executed one-at-a-time, so we queue it up.
				if req.Method == "tools/call" {
					select {
					case s.toolQueueCh <- &req:
						// Queue it if there's space.
					default:
						// @todo Should this be an MCP-level error (like with `content` and `isError`)?
						resp := jsonrpc.NewErrorResponse(req.ID, jsonrpc.NewError(TooManyToolCallsErrorCode, "Too many simultaneous tool calls", nil))
						b, err := json.Marshal(resp)
						if err != nil {
							slog.Error("error marshalling response to stdout", "response", resp)
							continue
						}
						s.writeCh <- b
					}
				} else {
					s.handleRequest(ctx, &req)
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

	return s.readLoop(ctx)
}
