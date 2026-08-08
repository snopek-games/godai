package server

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
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitlab.com/snopek-games/godai"
	"gitlab.com/snopek-games/godai/mcp/godot"
	"gitlab.com/snopek-games/godai/mcp/jsonrpc"
)

const ProtocolVersion string = "2025-11-25"

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

var GodaiVersion string = mustGetGodaiVersion()

func mustGetGodaiVersion() string {
	v, err := getPluginVersion(godai.AddonFS)
	if err != nil {
		panic(fmt.Errorf("unable to read Godai version from embedded plugin.cfg: %w", err))
	}
	return v
}

const GodaiMcpName string = "Godai"
const GodaiMcpTitle string = "Godai: AI agent integration with the Godot Engine"
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

// Lets the editor stop waiting when we do, rather than finishing a call the
// client was already told had failed.
const timeoutMetaKey = "godai/timeout_ms"

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
	Headless    bool
	Connection  *godot.Connection
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
	config             *Config
	writeCh            chan []byte
	jsonrpcDispatcher  *jsonrpc.Dispatcher
	connectionManager  *godot.ConnectionManager
	clientInfo         clientInfo
	clientInfoMutex    sync.RWMutex
	clientRequests     map[int]chan *jsonrpc.Response
	clientRequestID    int
	clientRequestMutex sync.Mutex
	toolQueueCh        chan *jsonrpc.Request
	requestQueueCh     chan *jsonrpc.Request
	localTools         map[string]*Tool
	editors            []*editorInfo
	editorsMutex       sync.RWMutex
	roots              []string
	rootsMutex         sync.RWMutex
	headlessProjects   map[string]struct{}
	headlessMutex      sync.Mutex
	updateAvailable    string
	updateMutex        sync.RWMutex
}

func NewServer(config *Config) *Server {
	d := jsonrpc.NewDispatcher()

	s := &Server{
		config:            config,
		writeCh:           make(chan []byte, 16),
		clientRequests:    make(map[int]chan *jsonrpc.Response),
		jsonrpcDispatcher: d,
		toolQueueCh:       make(chan *jsonrpc.Request, 4),
		requestQueueCh:    make(chan *jsonrpc.Request, 16),
		localTools:        make(map[string]*Tool),
		editors:           make([]*editorInfo, 0),
		roots:             config.RootPaths,
		headlessProjects:  make(map[string]struct{}),
	}

	var scanner godot.ConnectionScanner
	if config.Global {
		scanner = &godot.GlobalConnectionScanner{InstancesPath: config.EditorInstancesPath}
	} else {
		scanner = &godot.ProjectConnectionScanner{
			InstancesPath: config.EditorInstancesPath,
			GetRootPaths:  s.getRootPaths,
		}
	}

	s.connectionManager = godot.NewConnectionManager(godot.ConnectionManagerConfig{
		Scanner:      scanner,
		ScanInterval: config.EditorScanInterval,
		RetryDelay:   config.EditorRetryDelay,
		OnConnect:    s.onEditorConnect,
		OnDisconnect: s.onEditorDisconnect,
	})

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
// getRootPaths(), and onEditorConnect()).

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

// getClientInitializeParams returns the appInfo and raw capabilities to forward to
// an editor when initializing its connection.
func (s *Server) getClientInitializeParams() (appInfo, map[string]any) {
	s.clientInfoMutex.RLock()
	defer s.clientInfoMutex.RUnlock()
	return s.clientInfo.appInfo, s.clientInfo.rawCapabilities
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
	clientAppInfo, clientCapabilities := s.getClientInitializeParams()
	params := initializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      clientAppInfo,
		Capabilities:    clientCapabilities,
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
		Headless    bool   `json:"headless"`
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
		Headless:    projectInfo.Headless,
	}

	s.editorsMutex.Lock()
	s.editors = append(s.editors, editor)
	s.editorsMutex.Unlock()

	// Must stay after the editor is registered, or checkForUpdate() can miss it.
	s.sendUpdateNotification(ctx, conn)

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
	newEditors := make([]*editorInfo, 0, len(s.editors))
	for _, e := range s.editors {
		if e.Connection != conn {
			newEditors = append(newEditors, e)
		}
	}
	s.editors = newEditors
}

// markHeadlessProject records that we launched a headless editor for this
// project, so we can shut it down when the server exits.
func (s *Server) markHeadlessProject(projectPath string) {
	s.headlessMutex.Lock()
	defer s.headlessMutex.Unlock()
	s.headlessProjects[projectPath] = struct{}{}
}

// unmarkHeadlessProject forgets a headless editor we launched (e.g. because it
// was closed explicitly), so we don't try to close it again at shutdown.
func (s *Server) unmarkHeadlessProject(projectPath string) {
	s.headlessMutex.Lock()
	defer s.headlessMutex.Unlock()
	delete(s.headlessProjects, projectPath)
}

// shutdownCloseTimeout bounds how long we wait for a headless editor to save and
// shut down when the server exits.
const shutdownCloseTimeout = 30 * time.Second

// closeHeadlessEditors shuts down the headless editors we launched. It runs at
// server exit, so it uses fresh contexts rather than the (now-cancelled) run
// context. We look up the live connection by project path, so this still works
// after an editor has restarted with a new connection.
//
// We only close an editor that is *currently* headless. If the user killed our
// headless editor and launched their own (non-headless) editor for the same
// project, that replacement is left alone.
func (s *Server) closeHeadlessEditors() {
	s.headlessMutex.Lock()
	projects := make([]string, 0, len(s.headlessProjects))
	for p := range s.headlessProjects {
		projects = append(projects, p)
	}
	s.headlessProjects = make(map[string]struct{})
	s.headlessMutex.Unlock()

	var wg sync.WaitGroup
	for _, projectPath := range projects {
		conn, headless := s.getEditorConnectionForProject(projectPath)
		if conn == nil || !headless {
			// Already gone (crashed, or closed by the user), or replaced by an
			// editor we didn't launch.
			continue
		}

		args, err := json.Marshal(map[string]string{"project_path": projectPath})
		if err != nil {
			slog.Error("error marshalling close_editor arguments on shutdown", "projectPath", projectPath, "error", err)
			continue
		}

		wg.Add(1)
		go func(projectPath string, conn *godot.Connection, args json.RawMessage) {
			defer wg.Done()

			// A headless editor has no user to prompt, so close_editor saves and
			// quits on its own.
			ctx, cancel := context.WithTimeout(context.Background(), shutdownCloseTimeout)
			defer cancel()
			if _, err := conn.CallMethod(ctx, "tools/call", &callToolParams{
				Name:      "close_editor",
				Arguments: args,
			}); err != nil {
				slog.Error("error closing headless editor on shutdown", "projectPath", projectPath, "error", err)
			}
		}(projectPath, conn, args)
	}
	wg.Wait()
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

func (s *Server) getEditorConnection(projectPath string) (*godot.Connection, error) {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	for _, e := range s.editors {
		if e.ProjectPath == projectPath {
			return e.Connection, nil
		}
	}

	return nil, newUserVisibleError("not connected to the Godot editor for project: "+projectPath, nil, []string{
		"List open projects using the `list_open_projects` tool",
		"Open the Godot editor for this project using the `open_godot_project` tool",
	})

}

// getEditorConnectionForProject returns the live connection for a project and
// whether that editor is running headless. The connection is nil if no editor
// is currently connected for the project.
func (s *Server) getEditorConnectionForProject(projectPath string) (*godot.Connection, bool) {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	for _, e := range s.editors {
		if e.ProjectPath == projectPath {
			return e.Connection, e.Headless
		}
	}

	return nil, false
}

// waitForEditorReconnect blocks until an editor for the given project is
// connected on a connection other than oldConn (i.e. a fresh connection after
// a restart), or the context is done. It returns the new connection.
func (s *Server) waitForEditorReconnect(ctx context.Context, projectPath string, oldConn *godot.Connection) (*godot.Connection, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if conn, err := s.getEditorConnection(projectPath); err == nil && conn != oldConn {
				return conn, nil
			}
		}
	}
}

// waitForEditorDisconnect blocks until oldConn is no longer the connected
// editor for the given project (it dropped, or was replaced), or the context is
// done.
func (s *Server) waitForEditorDisconnect(ctx context.Context, projectPath string, oldConn *godot.Connection) error {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if conn, err := s.getEditorConnection(projectPath); err != nil || conn != oldConn {
				return nil
			}
		}
	}
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

	// Start the connection manager only after releasing the lock: it spawns the
	// scanLoop, which reads clientInfo via getRootPaths().
	s.connectionManager.Start()

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

func buildAnnotations(title string, ann map[string]any) map[string]any {
	// 'title' is also set on the annotations for backwards compatibility with old MCP clients.
	out := map[string]any{"title": title}

	readOnlyHint, _ := ann["readOnlyHint"].(bool)
	out["readOnlyHint"] = readOnlyHint

	if openWorldHint, ok := ann["openWorldHint"]; ok {
		out["openWorldHint"] = openWorldHint
	} else {
		out["openWorldHint"] = true
	}

	if !readOnlyHint {
		if destructiveHint, ok := ann["destructiveHint"]; ok {
			out["destructiveHint"] = destructiveHint
		} else {
			out["destructiveHint"] = true
		}

		if idempotentHint, ok := ann["idempotentHint"]; ok {
			out["idempotentHint"] = idempotentHint
		} else {
			out["idempotentHint"] = false
		}
	}

	return out
}

func (s *Server) rpcListTools(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	type toolOut struct {
		Name         string          `json:"name"`
		Title        string          `json:"title"`
		Description  string          `json:"description"`
		InputSchema  json.RawMessage `json:"inputSchema"`
		OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
		Annotations  map[string]any  `json:"annotations"`
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
			Annotations:  buildAnnotations(toolDef.Title, toolDef.Annotations),
		}

		list = append(list, out)
	}
	for name, toolDef := range GetDefaultRemoteToolDefinitions() {
		if toolDef.DoNotForward {
			continue
		}

		var inputSchema map[string]any
		if err := json.Unmarshal(toolDef.GetInputSchema(), &inputSchema); err != nil {
			slog.Error("error parsing tool's inputSchema", "toolName", name, "error", err)
			continue
		}
		inputSchemaProperties, ok := inputSchema["properties"].(map[string]any)
		if !ok {
			slog.Error("tool's inputSchema doesn't have properties", "toolName", name)
			continue
		}
		inputSchemaProperties["project_path"] = map[string]any{
			"type":        "string",
			"description": "The path to the Godot project. It must already be open in the Godot editor.",
		}
		rawInputSchema, err := json.Marshal(&inputSchema)
		if err != nil {
			slog.Error("error marshalling tool's inputSchema", "toolName", name, "error", err)
			continue
		}

		out := toolOut{
			Name:         name,
			Title:        toolDef.Title,
			Description:  toolDef.GetDescription(),
			InputSchema:  rawInputSchema,
			OutputSchema: toolDef.GetOutputSchema(),
			Annotations:  buildAnnotations(toolDef.Title, toolDef.Annotations),
		}
		list = append(list, out)
	}

	var resp struct {
		Tools []toolOut `json:"tools"`
	}
	resp.Tools = list

	return resp, nil
}

func (s *Server) callLocalTool(ctx context.Context, tool *Tool, params *callToolParams) (any, error) {
	result, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		return nil, err
	}

	var structuredContent any

	text, ok := result.(string)
	if !ok {
		structuredContent = result
		b, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		text = string(b)
	}

	var structedContentJSON json.RawMessage
	if structuredContent != nil {
		b, err := json.Marshal(structuredContent)
		if err != nil {
			return nil, err
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

// Splits the "project_path" argument, which selects the editor to talk to, from
// the arguments forwarded to it.
//
// Decoding one level deep keeps every remaining value as the bytes the client
// sent: decoding into a map[string]any would turn each number into a float64,
// and re-encoding that silently rounds anything past 2^53.
func splitProjectPath(raw json.RawMessage) (string, json.RawMessage, error) {
	var arguments map[string]json.RawMessage
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return "", nil, newUserVisibleError("unable to parse tool arguments", err, nil)
	}

	rawProjectPath, ok := arguments["project_path"]
	if !ok {
		return "", nil, newUserVisibleError("project_path argument is required", nil, nil)
	}

	var projectPath string
	if err := json.Unmarshal(rawProjectPath, &projectPath); err != nil {
		return "", nil, newUserVisibleError("project_path argument must be a string", err, nil)
	}

	delete(arguments, "project_path")
	forwarded, err := json.Marshal(arguments)
	if err != nil {
		return "", nil, err
	}

	return projectPath, forwarded, nil
}

func (s *Server) callRemoteTool(ctx context.Context, params *callToolParams) (*jsonrpc.Response, error) {
	rawProjectPath, forwardedArguments, err := splitProjectPath(params.Arguments)
	if err != nil {
		return nil, err
	}

	projectPath, err := canonicalPath(rawProjectPath)
	if err != nil {
		return nil, err
	}

	conn, err := s.getEditorConnection(projectPath)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.config.EditorToolTimeout)
	defer cancel()

	return conn.CallMethod(ctx, "tools/call", &callToolParams{
		Name:      params.Name,
		Arguments: forwardedArguments,
		Meta: map[string]any{
			timeoutMetaKey: s.config.EditorToolTimeout.Milliseconds(),
		},
	})
}

func (s *Server) rpcCallTool(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	var params callToolParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

	// Run locally if this is a local tool.
	tool, ok := s.localTools[params.Name]
	if ok {
		result, err := s.callLocalTool(ctx, tool, &params)
		if err != nil {
			slog.Error("error running local tool", "toolName", params.Name, "error", err)

			var userError *userVisibleError
			if errors.As(err, &userError) {
				return userError.makeToolResult(), nil
			} else {
				return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Error running local tool", nil)
			}
		}
		return result, nil
	}

	// Otherwise, run remotely.
	resp, err := s.callRemoteTool(ctx, &params)
	if err != nil {
		slog.Error("error running remote tool", "toolName", params.Name, "error", err)

		var userError *userVisibleError
		if errors.As(err, &userError) {
			return userError.makeToolResult(), nil
		} else {
			return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, err.Error(), nil)
		}
	}

	if resp.Error != nil {
		return nil, resp.Error
	}
	return resp.Result, nil
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
	go s.checkForUpdate(ctx)

	err := s.readLoop(ctx)
	s.closeHeadlessEditors()
	return err
}
