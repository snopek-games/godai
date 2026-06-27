package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gitlab.com/snopek-games/godai"
	"gitlab.com/snopek-games/godai/mcp/godot"
	"gitlab.com/snopek-games/godai/mcp/godot/variant"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (s *Server) setupLocalTools() {
	s.addLocalTool("list_projects", s.toolListProjects)
	s.addLocalTool("open_godot_project", s.toolOpenGodotProject)
	s.addLocalTool("list_open_projects", s.toolListOpenProjects)
	s.addLocalTool("get_mcp_configuration", s.toolGetMcpConfiguration)
	s.addLocalTool("set_mcp_configuration", s.toolSetMcpConfiguration)

	// Overrides a remote tool: the editor restarts itself, and we wait here for
	// it to disconnect and reconnect.
	s.addLocalToolOverride("restart_editor", s.toolRestartEditor)

	// Overrides a remote tool: the editor shuts itself down, and we wait here
	// for it to disconnect.
	s.addLocalToolOverride("close_editor", s.toolCloseEditor)
}

func (s *Server) addLocalTool(name string, handler ToolHandler) {
	defs := GetLocalToolDefinitions()
	def, ok := defs[name]
	if !ok {
		slog.Error("unable to find local tool definition", "name", name)
	} else {
		s.localTools[name] = &Tool{
			Definition: def,
			Handler:    handler,
		}
	}
}

// For overriding a remote tool with a local version.
func (s *Server) addLocalToolOverride(name string, handler ToolHandler) {
	defs := GetDefaultRemoteToolDefinitions()
	def, ok := defs[name]
	if !ok {
		slog.Error("unable to find remote tool definition", "name", name)
	} else {
		s.localTools[name] = &Tool{
			Definition: def,
			Handler:    handler,
			Override:   true,
		}
	}
}

// See note on sendRequestToClient() which this function uses.
func (s *Server) elicitClient(message string, requestedSchema map[string]any) (map[string]any, error) {
	params := struct {
		Mode            string         `json:"mode"`
		Message         string         `json:"message"`
		RequestedSchema map[string]any `json:"requestedSchema"`
	}{
		Mode:            "form",
		Message:         message,
		RequestedSchema: requestedSchema,
	}

	resp, err := s.sendRequestToClient("elicitation/create", params)
	if err != nil {
		return nil, err
	}

	var result struct {
		Action  string `json:"action"`
		Content map[string]any
	}

	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	if result.Action != "accept" {
		return nil, fmt.Errorf("elicitation received action '%s'", result.Action)
	}

	return result.Content, nil
}

func (s *Server) getProjectBasePath() (string, error) {
	if s.config.ProjectBasePath != "" {
		return s.config.ProjectBasePath, nil
	}

	if s.clientSupportsFormElicitation() {
		result, err := s.elicitClient("Please provide the base path where your Godot projects usually live", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project_path": map[string]any{
					"type":        "string",
					"description": "The base path where your Godot projects usually live",
				},
			},
		})
		if err == nil {
			path, ok := result["project_path"]
			if ok {
				pathStr, ok := path.(string)
				if ok && pathStr != "" {
					if canonicalPathStr, err := canonicalPath(pathStr); err == nil {
						if err := ValidateDirectory(canonicalPathStr); err == nil {
							s.config.ProjectBasePath = canonicalPathStr
							if err := s.saveConfig(); err != nil {
								slog.Error("error saving config", "error", err)
							}

							return canonicalPathStr, nil
						}
					} else {
						slog.Error("unable to get canonical path", "path", pathStr)
					}
				}
			}
			slog.Error("invalid project_path returned from elicitation", "result", result)
		} else {
			slog.Error("error eliciting project_path", "error", err)
		}
	}

	return "", newUserVisibleError("the path where your Godot projects usually live is not configured or doesn't exist", nil, []string{
		"Update the configuration for the Godai MCP in your MCP client to include the `--project-base-path <PATH>` argument",
		"Update the configuration for the Godai MCP using the `set_mcp_configuration` tool to set the `project_base_path`",
	})
}

func (s *Server) getDefaultGodotPath() (string, error) {
	if s.config.DefaultGodotPath != "" {
		return s.config.DefaultGodotPath, nil
	}

	if s.clientSupportsFormElicitation() {
		result, err := s.elicitClient("Please provide the full path to the Godot 4 executable on your system", map[string]any{
			"type": "object",
			"properties": map[string]any{
				"godot_path": map[string]any{
					"type":        "string",
					"description": "The full path to the Godot 4 executable on your system",
				},
			},
		})
		if err == nil {
			path, ok := result["godot_path"]
			if ok {
				pathStr, ok := path.(string)
				if ok && pathStr != "" {
					if canonicalPathStr, err := canonicalPath(pathStr); err == nil {
						if err := ValidateGodotExecutable(canonicalPathStr); err == nil {
							s.config.DefaultGodotPath = canonicalPathStr
							if err := s.saveConfig(); err != nil {
								slog.Error("error saving config", "error", err)
							}

							return canonicalPathStr, nil
						}
					} else {
						slog.Error("unable to get canonical path", "path", pathStr)
					}
				}
			}
			slog.Error("invalid godot_path returned from elicitation", "result", result)
		} else {
			slog.Error("error eliciting godot_path", "error", err)
		}
	}

	return "", newUserVisibleError("the path to the Godot 4 executable on your system is not configured or invalid", nil, []string{
		"Update the configuration for the Godai MCP in your MCP client to include the `--godot-path <PATH>` argument",
		"Update the configuration for the Godai MCP using the `set_mcp_configuration` tool to set the `godot_path`",
	})
}

const maxProjectScanDepth = 8

// Known problematic directories to skip when scanning for project directories.
var skipProjectScanDirs = map[string]bool{
	"node_modules": true,
	"__pycache__":  true,
	"vendor":       true,
}

func (s *Server) toolListProjects(ctx context.Context, rawParams json.RawMessage) (any, error) {
	pathSet := map[string]struct{}{}

	if s.config.Global {
		// Don't send the error to the client - we'll allow not having a base project path.
		projectBasePath, _ := s.getProjectBasePath()

		// List all the projects in the project base path.
		if projectBasePath != "" {
			entries, err := os.ReadDir(projectBasePath)
			if err != nil {
				return nil, newUserVisibleError(fmt.Sprintf("unable to read project path: %s", projectBasePath), err, []string{
					"Check the configuration for the Godai MCP in your MCP client and ensure the `--project-base-path <PATH>` argument is correct",
					"Check the configuration for the Godot MCP using the `get_mcp_configuration` tool and ensure the `project_base_path` is correct",
				})
			}

			for _, entry := range entries {
				if entry.IsDir() {
					projectPath := filepath.Join(projectBasePath, entry.Name())
					realProjectPath, err := canonicalPath(projectPath)
					if err == nil {
						pathSet[realProjectPath] = struct{}{}
					}
				}
			}
		}

		// List all the projects in the project manager.
		pml, err := godot.GetProjectManagerEntries()
		if err == nil {
			for _, e := range pml {
				realProjectPath, err := canonicalPath(e.ProjectPath)
				if err == nil {
					pathSet[realProjectPath] = struct{}{}
				}
			}
		} else {
			slog.Error("error getting the project manager entries", "error", err)
		}
	} else {
		// Walk our root paths and find Godot projects.
		for _, rootPath := range s.getRootPaths() {
			err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					slog.Debug("error walking for projects, skipping entry", "path", path, "error", err)
					return nil
				}

				if d.IsDir() && path != rootPath {
					// Skip hidden directories or known problematic ones.
					if strings.HasPrefix(d.Name(), ".") || skipProjectScanDirs[d.Name()] {
						return filepath.SkipDir
					}

					// Don't descend past the depth limit.
					if rel, err := filepath.Rel(rootPath, path); err == nil &&
						strings.Count(rel, string(filepath.Separator))+1 >= maxProjectScanDepth {
						return filepath.SkipDir
					}
				}

				// Consider a directory with a "project.godot" to be a Godot project.
				if !d.IsDir() && d.Name() == "project.godot" {
					realProjectPath, err := canonicalPath(filepath.Dir(path))
					if err != nil {
						// Skip this one, but keep scanning for others.
						slog.Error("unable to canonicalize project path, skipping", "path", path, "error", err)
						return filepath.SkipDir
					}
					pathSet[realProjectPath] = struct{}{}
					return filepath.SkipDir
				}

				return nil
			})
			if err != nil {
				slog.Error("error looking for projects in root", "path", rootPath, "error", err)
			}
		}
	}

	type outProject struct {
		ProjectPath string `json:"project_path"`
		ProjectName string `json:"project_name,omitempty"`
	}
	list := []outProject{}

	// Check all the project paths and produce the output.
	for projectPath := range pathSet {
		project, err := godot.ProjectFromPath(projectPath)
		if err != nil {
			continue
		}

		out := outProject{
			ProjectPath: projectPath,
		}

		cf, err := project.GetConfigFile()
		if err != nil {
			slog.Error("unable to parse Godot project config", "projectPath", projectPath, "error", err)
		} else {
			projectName, ok := cf.GetString("application", "config/name")
			if ok {
				out.ProjectName = projectName
			}
		}

		list = append(list, out)
	}

	var output struct {
		Projects []outProject `json:"projects"`
	}
	output.Projects = list

	return output, nil
}

func dirExists(path string) (bool, error) {
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return fi.IsDir(), nil
}

func getPluginVersion(fsys fs.FS) (string, error) {
	f, err := fsys.Open("addons/godai/plugin.cfg")
	if err != nil {
		return "", err
	}
	defer f.Close()

	r := io.Reader(f)
	cf, err := godot.ReadConfigFile(r)
	if err != nil {
		return "", err
	}

	v, ok := cf.GetString("plugin", "version")
	if !ok {
		return "", errors.New("plugin version not found")
	}

	return v, nil
}

func installAddon(project *godot.Project, forceReplace bool) error {
	projectPath := project.GetPath()
	addonRelPath := filepath.Join("addons", "godai")
	installedPath := filepath.Join(projectPath, addonRelPath)

	exists, err := dirExists(installedPath)
	if err != nil {
		return err
	}

	if exists {
		shouldReplace := false
		if forceReplace {
			shouldReplace = true
			slog.Info("debug enabled; forcing replacement of Godai addon", "projectPath", projectPath)
		} else {
			myVersion, err := getPluginVersion(godai.AddonFS)
			if err != nil {
				return err
			}
			installedVersion, err := getPluginVersion(os.DirFS(projectPath))
			if err != nil {
				// We consider an error reading the installed version to be a reason to replace it.
				// So, we don't return the error here, just make sure the version won't match.
				installedVersion = "error"
			}
			shouldReplace = (myVersion != installedVersion)
			if shouldReplace {
				slog.Info("installed Godai addon version doesn't match embedded",
					"projectPath", projectPath,
					"installedVersion", installedVersion,
					"embeddedVersion", myVersion,
				)
			}
		}

		if !shouldReplace {
			return nil
		}

		err = os.RemoveAll(installedPath)
		if err != nil {
			return err
		}
	}

	slog.Info("installing Godai addon", "projectPath", projectPath)

	return fs.WalkDir(godai.AddonFS, addonRelPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(addonRelPath, p)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(installedPath, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		r, err := godai.AddonFS.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()

		w, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer w.Close()

		if _, err := io.Copy(w, r); err != nil {
			return err
		}

		return nil
	})
}

// Attempt a safe edit of the project file.
func enableAddon(project *godot.Project) error {
	configPath := filepath.Join(project.GetPath(), "project.godot")

	f, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(f)))

	section := ""
	found := false
	out := strings.Builder{}

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			if line[0] == '[' && line[len(line)-1] == ']' {
				section = line[1 : len(line)-1]
			} else if section == "editor_plugins" && strings.HasPrefix(line, "enabled=PackedStringArray(") {
				parser := variant.NewParser(strings.NewReader(line))
				stmt, err := parser.ParseStatement()
				if err != nil {
					return err
				}

				values := stmt.Value.(variant.PackedStringArray)
				for _, v := range values {
					if v == "res://addons/godai/plugin.cfg" {
						// We already have the plugin, so we're good.
						return nil
					}
				}
				values = append(values, "res://addons/godai/plugin.cfg")

				buf := &strings.Builder{}
				writer := variant.NewWriter(buf)
				if err := writer.WriteAssignment("enabled", values); err != nil {
					return err
				}
				writer.Flush()
				line = buf.String()

				if len(line) > 0 && line[len(line)-1] == '\n' {
					line = line[:len(line)-1]
				}

				found = true
			}
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if !found {
		out.WriteString("[editor_plugins]\n\n")
		out.WriteString("enabled=PackedStringArray(\"res://addons/godai/plugin.cfg\")\n\n")
	}

	if err := os.WriteFile(configPath, []byte(out.String()), 0o755); err != nil {
		return err
	}

	return nil
}

func (s *Server) toolOpenGodotProject(ctx context.Context, rawParams json.RawMessage) (any, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
		Headless    bool   `json:"headless"`
	}

	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, err
	}

	realProjectPath, err := canonicalPath(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	if !s.config.Global && !godot.IsPathUnderAnyRoot(realProjectPath, s.getRootPaths()) {
		return nil, newUserVisibleError("project is not under one of our allowed roots", nil, []string{
			"Add this project's path to the allowed roots in your MCP client or by running the Godai MCP with an additional `--root <PATH>`",
			"Update the configuration for the Godai MCP in your MCP client and add the `--global` option to allow access to any project",
		})
	}

	if !s.hasEditorForProject(realProjectPath) {
		project, err := godot.ProjectFromPath(realProjectPath)
		if err != nil {
			return nil, newUserVisibleError("invalid project", err, nil)
		}

		defaultGodotPath, err := s.getDefaultGodotPath()
		if err != nil {
			return nil, err
		}

		err = installAddon(project, s.config.Debug)
		if err != nil {
			return nil, newUserVisibleError("unable to install godai addon", err, nil)
		}

		err = enableAddon(project)
		if err != nil {
			return nil, newUserVisibleError("unable to enable godai addon in project.godot file", err, nil)
		}

		args := []string{"--editor", "--path", realProjectPath}
		if params.Headless {
			// Use these arguments rather than `--headless` because they'll survive an editor restart.
			args = append(args, "--display-driver", "headless", "--audio-driver", "Dummy")
		}

		cmd := exec.Command(defaultGodotPath, args...)
		env := os.Environ()
		env = append(env, "DISPLAY="+s.config.X11Display)
		cmd.Env = env
		detachProcess(cmd)

		if err := cmd.Start(); err != nil {
			return nil, newUserVisibleError("unable to execute godot", err, []string{
				"Check the configuration for the Godai MCP in your MCP client and ensure the --godot-path argument is correct",
				"Check the configuration for the Godot MCP using the `get_mcp_configuration` tool and ensure the `godot_path` is correct",
			})
		}

		// Reap the zombies!
		go func() { _ = cmd.Wait() }()

		if params.Headless {
			// Mark before waiting to connect: a slow first import can take longer
			// than our timeout, and an editor that connects after we've returned is
			// still one we launched and must be shut down with the server.
			s.markHeadlessProject(realProjectPath)
		}

		ctx2, cancel := context.WithTimeout(ctx, time.Second*30)
		defer cancel()

		ticker := time.NewTicker(time.Second)
		connected := false

	outer:
		for {
			select {
			case <-ctx2.Done():
				break outer
			case <-ticker.C:
				if s.hasEditorForProject(realProjectPath) {
					connected = true
					break outer
				}
			}
		}

		if !connected {
			return nil, fmt.Errorf("timed out waiting for connection from Godot editor for '%s'", realProjectPath)
		}
	}

	var output struct {
		Success bool `json:"success"`
	}
	output.Success = true

	return output, nil
}

// restartReconnectTimeout bounds how long we wait for the editor to come back
// after restarting (it has to relaunch and re-scan the project).
const restartReconnectTimeout = 180 * time.Second

func (s *Server) toolRestartEditor(ctx context.Context, rawParams json.RawMessage) (any, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, newUserVisibleError("project_path argument is required", err, nil)
	}

	projectPath, err := canonicalPath(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	conn, err := s.getEditorConnection(projectPath)
	if err != nil {
		return nil, err
	}

	// Ask the editor to restart. It prompts the user to save and, once they
	// confirm, replies with success and then restarts (dropping this
	// connection). We deliberately don't impose our own timeout on this call,
	// since the editor may be sitting at the save prompt waiting for the user.
	resp, callErr := conn.CallMethod(ctx, "tools/call", &callToolParams{
		Name:      "restart_editor",
		Arguments: rawParams,
	})

	// A nil response or transport error means the connection dropped before the
	// editor replied (e.g. it restarted very quickly). That's fine: we just
	// proceed to wait for it to come back. But if we got a real reply, honor it
	// — in particular, the user may have declined the restart.
	if callErr == nil && resp != nil {
		if resp.Error != nil {
			return nil, fmt.Errorf("error calling restart_editor in the editor: %v", resp.Error)
		}
		var editorResult toolResult
		if err := json.Unmarshal(resp.Result, &editorResult); err == nil && editorResult.IsError {
			message := "the editor did not restart"
			if len(editorResult.Content) > 0 && editorResult.Content[0].Text != "" {
				message = editorResult.Content[0].Text
			}
			return nil, newUserVisibleError(message, nil, nil)
		}
	}

	// Wait for the editor to disconnect and reconnect (a fresh connection for
	// the same project).
	waitCtx, cancel := context.WithTimeout(ctx, restartReconnectTimeout)
	defer cancel()
	if _, err := s.waitForEditorReconnect(waitCtx, projectPath, conn); err != nil {
		return nil, newUserVisibleError("the editor did not reconnect after restarting", err, nil)
	}

	var output struct {
		Success bool `json:"success"`
	}
	output.Success = true

	return output, nil
}

// closeDisconnectTimeout bounds how long we wait for the editor to drop its
// connection after the user confirms the close (it still has to save and shut
// down).
const closeDisconnectTimeout = 120 * time.Second

func (s *Server) toolCloseEditor(ctx context.Context, rawParams json.RawMessage) (any, error) {
	var params struct {
		ProjectPath string `json:"project_path"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, newUserVisibleError("project_path argument is required", err, nil)
	}

	projectPath, err := canonicalPath(params.ProjectPath)
	if err != nil {
		return nil, err
	}

	conn, err := s.getEditorConnection(projectPath)
	if err != nil {
		return nil, err
	}

	// Ask the editor to close. It prompts the user to save and, once they
	// confirm, replies with success and then shuts down (dropping this
	// connection). We deliberately don't impose our own timeout on this call,
	// since the editor may be sitting at the save prompt waiting for the user.
	resp, callErr := conn.CallMethod(ctx, "tools/call", &callToolParams{
		Name:      "close_editor",
		Arguments: rawParams,
	})

	// A nil response or transport error means the connection dropped before the
	// editor replied (e.g. it closed very quickly). That's fine: we just proceed
	// to wait for the disconnect. But if we got a real reply, honor it — in
	// particular, the user may have declined the close.
	if callErr == nil && resp != nil {
		if resp.Error != nil {
			return nil, fmt.Errorf("error calling close_editor in the editor: %v", resp.Error)
		}
		var editorResult toolResult
		if err := json.Unmarshal(resp.Result, &editorResult); err == nil && editorResult.IsError {
			message := "the editor did not close"
			if len(editorResult.Content) > 0 && editorResult.Content[0].Text != "" {
				message = editorResult.Content[0].Text
			}
			return nil, newUserVisibleError(message, nil, nil)
		}
	}

	// Wait for the editor to disconnect, so a fast client doesn't move on
	// assuming the editor is gone while it's still saving and shutting down.
	waitCtx, cancel := context.WithTimeout(ctx, closeDisconnectTimeout)
	defer cancel()
	if err := s.waitForEditorDisconnect(waitCtx, projectPath, conn); err != nil {
		return nil, newUserVisibleError("the editor did not disconnect after closing", err, nil)
	}

	s.unmarkHeadlessProject(projectPath)

	var output struct {
		Success bool `json:"success"`
	}
	output.Success = true

	return output, nil
}

func (s *Server) toolListOpenProjects(ctx context.Context, rawParams json.RawMessage) (any, error) {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	type outProject struct {
		ProjectPath string `json:"project_path"`
		ProjectName string `json:"project_name"`
	}

	set := make(map[string]outProject, len(s.editors))
	for _, e := range s.editors {
		_, ok := set[e.ProjectPath]
		if !ok {
			set[e.ProjectPath] = outProject{
				ProjectPath: e.ProjectPath,
				ProjectName: e.ProjectName,
			}
		}
	}

	list := make([]outProject, 0, len(set))
	for _, p := range set {
		list = append(list, p)
	}

	var output struct {
		Projects []outProject `json:"projects"`
	}
	output.Projects = list

	return output, nil
}

func (s *Server) toolGetMcpConfiguration(ctx context.Context, rawParams json.RawMessage) (any, error) {
	sc := &SavedConfig{
		DefaultGodotPath: s.config.DefaultGodotPath,
	}
	if s.config.Global {
		sc.ProjectBasePath = s.config.ProjectBasePath
	}
	return sc, nil
}

func (s *Server) toolSetMcpConfiguration(ctx context.Context, rawParams json.RawMessage) (any, error) {
	sc := SavedConfig{}
	if err := json.Unmarshal(rawParams, &sc); err != nil {
		return nil, err
	}

	if sc.DefaultGodotPath != "" {
		canonicalPath, err := canonicalPath(sc.DefaultGodotPath)
		if err != nil {
			return nil, newUserVisibleError("invalid godot_path - doesn't exist or isn't executable", err, nil)
		}
		sc.DefaultGodotPath = canonicalPath
		if err := ValidateGodotExecutable(sc.DefaultGodotPath); err != nil {
			return nil, newUserVisibleError("invalid godot_path - doesn't exist or isn't executable", err, nil)
		}
	}

	if sc.ProjectBasePath != "" {
		canonicalPath, err := canonicalPath(sc.ProjectBasePath)
		if err != nil {
			return nil, newUserVisibleError("invalid project_path", err, nil)
		}
		sc.ProjectBasePath = canonicalPath
		if err := ValidateDirectory(sc.ProjectBasePath); err != nil {
			return nil, newUserVisibleError("invalid project_path", err, nil)
		}
	}

	s.config.DefaultGodotPath = sc.DefaultGodotPath
	if s.config.Global {
		s.config.ProjectBasePath = sc.ProjectBasePath
	}

	err := s.saveConfig()
	if err != nil {
		slog.Error("unable to save config file", "error", err)
	}

	var output struct {
		Success bool `json:"success"`
	}
	output.Success = true

	return output, nil
}
