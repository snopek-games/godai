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

func (s *Server) setupLocalTools() {
	s.addLocalTool("list_projects", s.toolListProjects)
	s.addLocalTool("open_godot_project", s.toolOpenGodotProject)
	s.addLocalTool("list_open_projects", s.toolListOpenProjects)
	s.addLocalTool("get_godai_settings", s.toolGetGodaiSettings)
	s.addLocalTool("set_godai_settings", s.toolSetGodaiSettings)

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
}

func (s *Server) toolListProjects(ctx context.Context, _ core.Args) (any, error) {
	projects, err := s.session.ListProjects(ctx)
	if err != nil {
		return nil, err
	}
	return projectsResult{Projects: projects}, nil
}

func (s *Server) toolListOpenProjects(ctx context.Context, _ core.Args) (any, error) {
	projects, err := s.session.ListOpenProjects(ctx)
	if err != nil {
		return nil, err
	}
	return projectsResult{Projects: projects}, nil
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

	if _, err := s.session.OpenProject(ctx, projectPath, core.OpenProjectOptions{Headless: headless}); err != nil {
		return nil, err
	}

	return successResult{Success: true}, nil
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

	resolved, err := core.ResolveSavedConfig(sc)
	if err != nil {
		return nil, err
	}

	if err := s.session.SetConfig(resolved); err != nil {
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

	list = append(list, core.RemoteToolListing()...)

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
