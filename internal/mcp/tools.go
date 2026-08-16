package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/jsonrpc"
)

type successResult struct {
	Success bool `json:"success"`
}

type openResult struct {
	Success     bool `json:"success"`
	AlreadyOpen bool `json:"already_open"`
}

type installedVersionsResult struct {
	Versions []core.EngineListing `json:"versions"`
}

type pinResult struct {
	Success      bool   `json:"success"`
	GodotVersion string `json:"godot_version"`
}

type unpinResult struct {
	Success   bool `json:"success"`
	WasPinned bool `json:"was_pinned"`
}

// The CLI validates the toolset names before starting the server, so an
// invalid name here is a programming error.
func computeEnabledTools(toolsets []string) map[string]bool {
	enabledToolsets, err := core.ExpandToolsets(toolsets)
	if err != nil {
		panic(err)
	}

	enabled := map[string]bool{}
	for name, def := range AllToolDefinitions() {
		if def.InAnyToolset(enabledToolsets) {
			enabled[name] = true
		}
	}
	return enabled
}

func (s *Server) setupLocalTools() {
	s.addLocalTool("list_projects", s.toolListProjects)
	s.addLocalTool("open_godot_project", s.toolOpenGodotProject)
	s.addLocalTool("list_open_projects", s.toolListOpenProjects)
	s.addLocalTool("get_godai_settings", s.toolGetGodaiSettings)
	s.addLocalTool("set_godai_settings", s.toolSetGodaiSettings)
	s.addLocalTool("list_installed_godot_versions", s.toolListInstalledGodotVersions)
	s.addLocalTool("search_available_godot_versions", s.toolSearchAvailableGodotVersions)
	s.addLocalTool("install_godot_version", s.toolInstallGodotVersion)
	s.addLocalTool("remove_godot_version", s.toolRemoveGodotVersion)
	s.addLocalTool("pin_project_to_godot_version", s.toolPinProjectToGodotVersion)
	s.addLocalTool("unpin_project_from_godot_version", s.toolUnpinProjectFromGodotVersion)

	// Overrides a remote tool: the editor restarts itself, and we wait here for
	// it to disconnect and reconnect.
	s.addLocalToolOverride("restart_editor", s.toolRestartEditor)

	// Overrides a remote tool: the editor shuts itself down, and we wait here
	// for it to disconnect.
	s.addLocalToolOverride("close_editor", s.toolCloseEditor)
}

func (s *Server) addLocalTool(name string, handler ToolHandler) {
	def, ok := GetLocalToolDefinitions()[name]
	if !ok {
		slog.Error("unable to find local tool definition", "name", name)
		return
	}
	s.localTools[name] = &Tool{Definition: def, Handler: handler}
}

// For overriding a remote tool with a local version.
func (s *Server) addLocalToolOverride(name string, handler ToolHandler) {
	def, ok := core.RemoteToolDefinitions()[name]
	if !ok {
		slog.Error("unable to find remote tool definition", "name", name)
		return
	}
	s.localTools[name] = &Tool{Definition: def, Handler: handler, Override: true}
}

type projectsResult struct {
	Projects []core.ProjectInfo `json:"projects"`
	Note     string             `json:"note,omitempty"`
}

type openProjectsResult struct {
	Projects []core.OpenProjectInfo `json:"projects"`
}

func (s *Server) toolListProjects(_ context.Context, _ core.Args) (any, error) {
	projects, note, err := s.session.ListProjects()
	if err != nil {
		return nil, err
	}
	return projectsResult{Projects: projects, Note: note}, nil
}

func (s *Server) toolListOpenProjects(ctx context.Context, _ core.Args) (any, error) {
	projects, err := s.session.ListOpenProjects(ctx)
	if err != nil {
		return nil, err
	}
	return openProjectsResult{Projects: projects}, nil
}

func (s *Server) toolOpenGodotProject(ctx context.Context, args core.Args) (any, error) {
	projectPath, ok, err := args.String("project_path")
	if err != nil {
		return nil, core.NewUserError("project_path argument must be a string", err, nil)
	}
	if !ok {
		return nil, core.NewUserError("project_path argument is required", nil, nil)
	}

	headless, _, err := args.Bool("headless")
	if err != nil {
		return nil, core.NewUserError("headless argument must be a boolean", err, nil)
	}

	godotVersion, _, err := args.String("godot_version")
	if err != nil {
		return nil, core.NewUserError("godot_version argument must be a string", err, nil)
	}

	result, err := s.session.OpenProject(ctx, projectPath, core.OpenProjectOptions{
		Headless:     headless,
		GodotVersion: godotVersion,
	})
	if err != nil {
		return nil, err
	}

	return openResult{Success: true, AlreadyOpen: result.AlreadyOpen}, nil
}

func (s *Server) toolListInstalledGodotVersions(ctx context.Context, _ core.Args) (any, error) {
	engines, err := s.session.ListEngines()
	if err != nil {
		return nil, err
	}
	return installedVersionsResult{Versions: engines}, nil
}

func (s *Server) toolSearchAvailableGodotVersions(ctx context.Context, args core.Args) (any, error) {
	filter, _, err := args.String("filter")
	if err != nil {
		return nil, core.NewUserError("filter argument must be a string", err, nil)
	}

	includePre, _, err := args.Bool("include_prereleases")
	if err != nil {
		return nil, core.NewUserError("include_prereleases argument must be a boolean", err, nil)
	}

	versions, err := s.session.SearchEngines(ctx, filter, includePre, false)
	if err != nil {
		return nil, err
	}

	return struct {
		Versions []core.EngineSearchResult `json:"versions"`
	}{versions}, nil
}

func (s *Server) toolInstallGodotVersion(ctx context.Context, args core.Args) (any, error) {
	version, err := requiredStringArg(args, "godot_version")
	if err != nil {
		return nil, err
	}

	withTemplates, _, err := args.Bool("with_export_templates")
	if err != nil {
		return nil, core.NewUserError("with_export_templates argument must be a boolean", err, nil)
	}

	result, err := s.session.InstallEngine(ctx, version, core.DownloadOptions{})
	if err != nil {
		return nil, err
	}

	if withTemplates {
		if err := s.session.InstallTemplatesIfNeeded(ctx, result.Engine.Version, core.DownloadOptions{}); err != nil {
			return nil, err
		}
		// The templates are part of what a listing says about an engine.
		if engine, err := s.session.DescribeEngine(result.Engine.Version); err == nil {
			result.Engine = *engine
		}
	}

	return result, nil
}

func (s *Server) toolRemoveGodotVersion(ctx context.Context, args core.Args) (any, error) {
	version, err := requiredStringArg(args, "godot_version")
	if err != nil {
		return nil, err
	}

	withTemplates, _, err := args.Bool("with_export_templates")
	if err != nil {
		return nil, core.NewUserError("with_export_templates argument must be a boolean", err, nil)
	}

	return s.session.RemoveEngine(version, withTemplates)
}

func (s *Server) toolPinProjectToGodotVersion(ctx context.Context, args core.Args) (any, error) {
	projectPath, err := s.allowedProjectPathArg(args)
	if err != nil {
		return nil, err
	}

	version, err := requiredStringArg(args, "godot_version")
	if err != nil {
		return nil, err
	}

	engine, err := s.session.FindEngine(version)
	if err != nil {
		return nil, err
	}

	if err := core.SetProjectGodotVersion(projectPath, engine.Name); err != nil {
		return nil, core.NewUserError("unable to write "+core.ProjectConfigName, err, nil)
	}

	return pinResult{Success: true, GodotVersion: engine.Name}, nil
}

func (s *Server) toolUnpinProjectFromGodotVersion(ctx context.Context, args core.Args) (any, error) {
	projectPath, err := s.allowedProjectPathArg(args)
	if err != nil {
		return nil, err
	}

	wasPinned, err := core.UnsetProjectGodotVersion(projectPath)
	if err != nil {
		return nil, core.NewUserError("unable to write "+core.ProjectConfigName, err, nil)
	}

	return unpinResult{Success: true, WasPinned: wasPinned}, nil
}

func requiredStringArg(args core.Args, name string) (string, error) {
	value, ok, err := args.String(name)
	if err != nil {
		return "", core.NewUserError(name+" argument must be a string", err, nil)
	}
	if !ok || value == "" {
		return "", core.NewUserError(name+" argument is required", nil, nil)
	}
	return value, nil
}

func (s *Server) toolRestartEditor(ctx context.Context, args core.Args) (any, error) {
	projectPath, err := s.projectPathArg(args)
	if err != nil {
		return nil, err
	}

	if err := s.session.RestartEditor(ctx, projectPath, args); err != nil {
		return nil, err
	}

	return successResult{Success: true}, nil
}

func (s *Server) toolCloseEditor(ctx context.Context, args core.Args) (any, error) {
	projectPath, err := s.projectPathArg(args)
	if err != nil {
		return nil, err
	}

	if err := s.session.CloseEditor(ctx, projectPath, args); err != nil {
		return nil, err
	}

	return successResult{Success: true}, nil
}

func (s *Server) projectPathArg(args core.Args) (string, error) {
	raw, ok, err := args.String("project_path")
	if err != nil || !ok {
		return "", core.NewUserError("project_path argument is required", err, nil)
	}
	return core.CanonicalPath(raw)
}

// allowedProjectPathArg is for the tools that write to a project without going
// through an editor, so we need to check the allowed roots.
func (s *Server) allowedProjectPathArg(args core.Args) (string, error) {
	raw, ok, err := args.String("project_path")
	if err != nil || !ok {
		return "", core.NewUserError("project_path argument is required", err, nil)
	}
	return s.session.AllowedProjectPath(raw)
}

func (s *Server) toolGetGodaiSettings(ctx context.Context, _ core.Args) (any, error) {
	sc := s.session.GetConfig()
	if !s.session.Global() {
		sc.ProjectBasePath = ""
	}
	return sc, nil
}

func (s *Server) toolSetGodaiSettings(ctx context.Context, args core.Args) (any, error) {
	raw, err := args.Raw()
	if err != nil {
		return nil, err
	}

	var sc core.SavedConfig
	if err := json.Unmarshal(raw, &sc); err != nil {
		return nil, err
	}

	if sc.ProjectBasePath != "" && !s.session.Global() {
		return nil, core.NewUserError("project_base_path only applies when Godai is running with --global", core.ErrNotConfigured, nil)
	}

	if err := s.session.SetConfig(sc); err != nil {
		return nil, err
	}

	return successResult{Success: true}, nil
}

func (s *Server) rpcListTools(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	list := []core.ToolListing{}

	for name, tool := range s.localTools {
		if tool.Override {
			// This is a local "override", so the definition will come from the remote definitions.
			continue
		}
		if !s.enabledTools[name] {
			continue
		}
		def := tool.Definition
		list = append(list, core.ToolListing{
			Name:         name,
			Title:        def.Title,
			Description:  def.GetDescription(),
			InputSchema:  def.GetInputSchema(),
			OutputSchema: def.GetOutputSchema(),
			Annotations:  core.BuildAnnotations(def.Title, def.Annotations),
		})
	}

	for _, listing := range core.RemoteToolListing() {
		if s.enabledTools[listing.Name] {
			list = append(list, listing)
		}
	}

	var resp struct {
		Tools []core.ToolListing `json:"tools"`
	}
	resp.Tools = list

	return resp, nil
}

func (s *Server) rpcCallTool(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	var params callToolParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

	if !s.enabledTools[params.Name] {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Unknown tool: "+params.Name, nil)
	}

	if tool, ok := s.localTools[params.Name]; ok {
		result, err := s.callLocalTool(ctx, tool, params.Arguments)
		if err != nil {
			slog.Error("error running local tool", "toolName", params.Name, "error", err)

			var userError *core.UserError
			if errors.As(err, &userError) {
				return toolResultForError(userError), nil
			}
			return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Error running local tool", nil)
		}
		return result, nil
	}

	result, err := s.callRemoteTool(ctx, params.Name, params.Arguments)
	if err != nil {
		slog.Error("error running remote tool", "toolName", params.Name, "error", err)

		var rpcError *core.EditorRPCError
		if errors.As(err, &rpcError) {
			return nil, rpcError.RPC
		}

		var userError *core.UserError
		if errors.As(err, &userError) {
			return toolResultForError(userError), nil
		}
		return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, err.Error(), nil)
	}

	return result.Raw, nil
}

func (s *Server) callRemoteTool(ctx context.Context, name string, rawArgs json.RawMessage) (*core.ToolResult, error) {
	rawProjectPath, args, err := core.SplitProjectPath(rawArgs)
	if err != nil {
		return nil, err
	}

	projectPath, err := core.CanonicalPath(rawProjectPath)
	if err != nil {
		return nil, err
	}

	return s.session.CallEditorTool(ctx, projectPath, name, args, core.CallOptions{})
}

func (s *Server) callLocalTool(ctx context.Context, tool *Tool, rawArgs json.RawMessage) (any, error) {
	args, err := core.ParseArgs(rawArgs)
	if err != nil {
		return nil, err
	}

	result, err := tool.Handler(ctx, args)
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
