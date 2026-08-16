package core

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gitlab.com/snopek-games/godai/internal/godot"
)

type ProjectInfo struct {
	ProjectPath string `json:"project_path"`
	ProjectName string `json:"project_name,omitempty"`
}

type OpenProjectInfo struct {
	ProjectPath  string `json:"project_path"`
	ProjectName  string `json:"project_name,omitempty"`
	Headless     bool   `json:"headless"`
	GodotVersion string `json:"godot_version,omitempty"`
}

const maxProjectScanDepth = 8

// Known problematic directories to skip when scanning for project directories.
var skipProjectScanDirs = map[string]bool{
	"node_modules": true,
	"__pycache__":  true,
	"vendor":       true,
}

const projectManagerOnlyNote = "only listing projects registered in the Godot project manager; " +
	"set a `project_base_path` with `godai config` (MCP: the `set_godai_settings` tool) to also list the projects in that directory"

func (s *Session) ListProjects() ([]ProjectInfo, string, error) {
	pathSet := map[string]struct{}{}
	note := ""

	if s.Global() {
		if projectBasePath := s.config.ProjectBasePath; projectBasePath == "" {
			note = projectManagerOnlyNote
		} else {
			entries, err := os.ReadDir(projectBasePath)
			if err != nil {
				return nil, "", NewUserError(fmt.Sprintf("unable to read project path: %s", projectBasePath), err, []string{
					"Check the base path: `godai config` (MCP: the `get_godai_settings` tool)",
					"Set a different one: `godai config --set project_base_path=<PATH>`",
				})
			}

			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				if realPath, err := CanonicalPath(filepath.Join(projectBasePath, entry.Name())); err == nil {
					pathSet[realPath] = struct{}{}
				}
			}
		}

		pml, err := godot.GetProjectManagerEntries()
		if err == nil {
			for _, e := range pml {
				if realPath, err := CanonicalPath(e.ProjectPath); err == nil {
					pathSet[realPath] = struct{}{}
				}
			}
		} else {
			slog.Error("error getting the project manager entries", "error", err)
		}
	} else {
		for _, rootPath := range s.RootPaths() {
			scanRootForProjects(rootPath, pathSet)
		}
	}

	list := make([]ProjectInfo, 0, len(pathSet))
	for projectPath := range pathSet {
		project, err := godot.ProjectFromPath(projectPath)
		if err != nil {
			continue
		}

		info := ProjectInfo{ProjectPath: projectPath}

		cf, err := project.GetConfigFile()
		if err != nil {
			slog.Error("unable to parse Godot project config", "projectPath", projectPath, "error", err)
		} else if projectName, ok := cf.GetString("application", "config/name"); ok {
			info.ProjectName = projectName
		}

		list = append(list, info)
	}

	sortProjects(list)

	return list, note, nil
}

func scanRootForProjects(rootPath string, pathSet map[string]struct{}) {
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
			realProjectPath, err := CanonicalPath(filepath.Dir(path))
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

func sortProjects(list []ProjectInfo) {
	sort.Slice(list, func(i, j int) bool { return list[i].ProjectPath < list[j].ProjectPath })
}

func sortOpenProjects(list []OpenProjectInfo) {
	sort.Slice(list, func(i, j int) bool { return list[i].ProjectPath < list[j].ProjectPath })
}

func (s *Session) ListOpenProjects(ctx context.Context) ([]OpenProjectInfo, error) {
	if err := s.ensureStarted(ctx); err != nil {
		return nil, err
	}
	s.ScanNow()

	seen := map[string]struct{}{}
	list := []OpenProjectInfo{}
	for _, e := range s.Editors() {
		if _, ok := seen[e.ProjectPath]; ok {
			continue
		}
		seen[e.ProjectPath] = struct{}{}
		list = append(list, OpenProjectInfo{
			ProjectPath:  e.ProjectPath,
			ProjectName:  e.ProjectName,
			Headless:     e.Headless,
			GodotVersion: e.GodotVersion,
		})
	}

	sortOpenProjects(list)

	return list, nil
}

const autoApproveToolsEnv = "GODAI_AUTO_APPROVE_TOOLS"

type OpenProjectOptions struct {
	Headless    bool
	AutoApprove bool
	Wait        time.Duration
	// GodotVersion opens the project with the version named, whatever the
	// project itself asks for.
	GodotVersion string
}

// AllowedProjectPath canonicalizes a project path the caller was given, and
// refuses one outside the roots this session was told it may touch.
func (s *Session) AllowedProjectPath(path string) (string, error) {
	realProjectPath, err := CanonicalPath(path)
	if err != nil {
		return "", err
	}

	if !s.Global() && !godot.IsPathUnderAnyRoot(realProjectPath, s.RootPaths()) {
		return "", NewUserError("project is not under one of our allowed roots", nil, []string{
			"Allow this project's path with an additional `--root <PATH>`",
			"Allow access to any project with the `--global` option",
		})
	}

	return realProjectPath, nil
}

type OpenProjectResult struct {
	ProjectPath string `json:"project_path"`
	AlreadyOpen bool   `json:"already_open"`
	Headless    bool   `json:"headless"`
}

// InstallAddon installs and enables the godai addon in a project, exactly as
// opening it would, but without launching an editor.
func (s *Session) InstallAddon(path string) (string, error) {
	realProjectPath, err := s.AllowedProjectPath(path)
	if err != nil {
		return "", err
	}

	project, err := godot.ProjectFromPath(realProjectPath)
	if err != nil {
		return "", NewUserError("invalid project", err, nil)
	}

	if err := setupAddon(project, s.config.Debug); err != nil {
		return "", err
	}
	return realProjectPath, nil
}

func (s *Session) OpenProject(ctx context.Context, path string, opts OpenProjectOptions) (*OpenProjectResult, error) {
	opts.Headless = opts.Headless || s.config.ForceHeadless
	opts.AutoApprove = opts.AutoApprove || s.config.ForceAutoApprove

	realProjectPath, err := s.AllowedProjectPath(path)
	if err != nil {
		return nil, err
	}

	if err := s.ensureStarted(ctx); err != nil {
		return nil, err
	}
	s.ScanNow()

	wait := opts.Wait
	if wait <= 0 {
		wait = s.config.OpenProjectTimeout
	}

	if s.hasRunningEditorForProject(realProjectPath) {
		waitCtx, cancel := context.WithTimeout(ctx, wait)
		defer cancel()

		editor, err := s.WaitForEditor(waitCtx, realProjectPath)
		if err != nil {
			return nil, waitError(waitCtx, "timed out connecting to the Godot editor already running for '"+realProjectPath+"'", err)
		}
		if err := s.checkEditorVersion(editor, opts); err != nil {
			return nil, err
		}
		if err := s.checkAddonVersion(editor); err != nil {
			return nil, err
		}

		return &OpenProjectResult{ProjectPath: realProjectPath, AlreadyOpen: true, Headless: editor.Headless}, nil
	}

	project, err := godot.ProjectFromPath(realProjectPath)
	if err != nil {
		return nil, NewUserError("invalid project", err, nil)
	}

	godotPath, err := s.GodotExecutable(ctx, realProjectPath, EngineOptions{
		Version:     opts.GodotVersion,
		AutoInstall: !s.config.NoAutoInstall,
		Prompt:      true,
	})
	if err != nil {
		return nil, err
	}

	if err := setupAddon(project, s.config.Debug); err != nil {
		return nil, err
	}

	args := []string{"--editor", "--path", realProjectPath}
	if opts.Headless {
		// Use these arguments rather than `--headless` because they'll survive an editor restart.
		args = append(args, "--display-driver", "headless", "--audio-driver", "Dummy")
	}

	cmd := exec.Command(godotPath, args...)
	cmd.Env = append(os.Environ(), "DISPLAY="+s.config.X11Display)
	if opts.AutoApprove {
		cmd.Env = append(cmd.Env, autoApproveToolsEnv+"=1")
	}
	detachProcess(cmd)

	logFile, logPath, logErr := editorLogFile(realProjectPath)
	if logErr != nil {
		slog.Warn("cannot capture editor output", "error", logErr)
	}
	if logFile != nil {
		// Hand the editor a real file, never a pipe: the editor outlives godai,
		// and its next write to a closed pipe would kill it with SIGPIPE.
		cmd.Stdout, cmd.Stderr = logFile, logFile
	}

	startErr := cmd.Start()
	if logFile != nil {
		logFile.Close()
	}
	if startErr != nil {
		return nil, NewUserError("unable to execute godot", startErr, []string{
			"Check which Godot that is: `godai engine which`",
			"Install it again: `godai engine install <VERSION>`",
		})
	}

	// Reap the zombies!
	go func() { _ = cmd.Wait() }()

	if opts.Headless {
		// Mark before waiting to connect: a slow first import can take longer
		// than our timeout, and an editor that connects after we've returned is
		// still one we launched and must be shut down with the session.
		s.markHeadlessProject(realProjectPath)
	}

	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	if _, err := s.WaitForEditor(waitCtx, realProjectPath); err != nil {
		return nil, waitError(waitCtx, "timed out waiting for connection from Godot editor for '"+realProjectPath+"'"+editorLogTail(logPath), err)
	}

	return &OpenProjectResult{ProjectPath: realProjectPath, Headless: opts.Headless}, nil
}

// checkEditorVersion refuses to hand back an editor that isn't the version
// that was asked for, since an editor that's already running can't change the
// Godot it started with.
func (s *Session) checkEditorVersion(editor *Editor, opts OpenProjectOptions) error {
	wanted := opts.GodotVersion
	if wanted == "" && s.config.GodotVersionIsExplicit {
		wanted = s.config.GodotVersion
	}
	if wanted == "" || editor.GodotVersion == "" {
		return nil
	}

	engine, err := s.FindEngine(wanted)
	if err != nil {
		return err
	}
	// A linked engine that wouldn't say what it is can't be told apart from
	// the editor that's running, so it's taken at its word.
	if !engine.Version.Known() || engine.Version.String() == editor.GodotVersion {
		return nil
	}

	return NewUserError(
		fmt.Sprintf("the editor already open for '%s' is Godot %s, not %s", editor.ProjectPath, editor.GodotVersion, engine.Name),
		ErrEditorVersionMismatch, []string{
			"Close it first: `godai editor close <PATH>` (MCP: the `close_editor` tool)",
			"Or open it without asking for a version",
		})
}

// Reports why the wait ended, so that a timeout or an interrupt doesn't get
// reported (and exit) as "no editor connected".
func waitError(waitCtx context.Context, message string, err error) error {
	if ctxErr := waitCtx.Err(); ctxErr != nil {
		err = ctxErr
	}
	return NewUserError(message, err, nil)
}

func ResolveProjectPath(hint string) (string, error) {
	if hint != "" {
		return validateProjectPath(hint)
	}

	if path, ok := findProjectFromCwd(); ok {
		return path, nil
	}

	return "", NewUserError("no Godot project found", ErrNotConfigured, []string{
		"Run this from inside a Godot project directory",
		"Or name one with `--project-path <PATH>`",
	})
}

func validateProjectPath(path string) (string, error) {
	realPath, err := CanonicalPath(path)
	if err != nil {
		return "", NewUserError("invalid project path: "+path, err, nil)
	}
	if _, err := godot.ProjectFromPath(realPath); err != nil {
		return "", NewUserError("not a Godot project: "+realPath, err, nil)
	}
	return realPath, nil
}

func findProjectFromCwd() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "project.godot")); err == nil {
			if realPath, err := CanonicalPath(dir); err == nil {
				return realPath, true
			}
			return "", false
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func (s *Session) GetConfig() SavedConfig {
	return SavedConfig{
		GodotVersion:    s.config.GodotVersion,
		ProjectBasePath: s.config.ProjectBasePath,
		UpdateCheck:     s.config.UpdateCheck,
	}
}

func (s *Session) ResolveSavedConfig(sc SavedConfig) (SavedConfig, error) {
	resolved := SavedConfig{}

	if sc.GodotVersion != "" {
		engine, err := s.FindEngine(sc.GodotVersion)
		if err != nil {
			return resolved, err
		}
		resolved.GodotVersion = engine.Name
	}

	if sc.ProjectBasePath != "" {
		path, err := resolveDirectory(sc.ProjectBasePath)
		if err != nil {
			return resolved, NewUserError("invalid project_base_path - doesn't exist or isn't a directory", err, nil)
		}
		resolved.ProjectBasePath = path
	}

	if sc.UpdateCheck != "" {
		if sc.UpdateCheck != UpdateCheckOn && sc.UpdateCheck != UpdateCheckOff {
			return resolved, NewUserError(`invalid update_check - must be "on" or "off"`, nil, nil)
		}
		resolved.UpdateCheck = sc.UpdateCheck
	}

	return resolved, nil
}

func (s *Session) SetConfig(sc SavedConfig) error {
	resolved, err := s.ResolveSavedConfig(sc)
	if err != nil {
		return err
	}

	if resolved.GodotVersion != "" {
		s.config.GodotVersion = resolved.GodotVersion
	}
	if resolved.ProjectBasePath != "" {
		s.config.ProjectBasePath = resolved.ProjectBasePath
	}
	if resolved.UpdateCheck != "" {
		s.config.UpdateCheck = resolved.UpdateCheck
	}

	return s.mergeSavedConfig(resolved)
}

func (s *Session) UnsetConfig(names []string) error {
	for _, name := range names {
		if err := CheckSettingName(name); err != nil {
			return err
		}
	}

	saved := SavedConfig{}
	if s.config.SavedConfigPath != "" {
		if existing, err := LoadConfig(s.config.SavedConfigPath); err == nil {
			saved = *existing
		}
	}

	live := s.GetConfig()
	for _, name := range names {
		if err := saved.SetSetting(name, ""); err != nil {
			return err
		}
		if err := live.SetSetting(name, ""); err != nil {
			return err
		}
	}

	s.config.GodotVersion = live.GodotVersion
	s.config.ProjectBasePath = live.ProjectBasePath
	s.config.UpdateCheck = live.UpdateCheck

	if s.config.SavedConfigPath == "" {
		return nil
	}
	return SaveConfig(s.config.SavedConfigPath, &saved)
}
