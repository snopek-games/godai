package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gitlab.com/snopek-games/godai/internal/godot"
)

const EditorProtocolVersion = "2025-11-25"

const (
	AppName  = "Godai"
	AppTitle = "Godai: AI agent integration with the Godot Engine"
)

const (
	timeoutMetaKey      = "godai/timeout_ms"
	godaiVersionMetaKey = "godai/godai_version"
	clientKindMetaKey   = "godai/client_kind"
)

const (
	ClientKindCLI = "cli"
	ClientKindMCP = "mcp"
)

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Meta      map[string]any  `json:"_meta,omitempty"`
}

type CallOptions struct {
	Wait time.Duration
}

// Strict equality for now; loosen this function if a protocol-version scheme
// ever replaces it.
func AddonCompatible(addonVersion string) bool {
	return addonVersion == Version
}

// checkAddonVersion refuses an editor whose addon doesn't match this godai,
// so protocol drift surfaces as this error instead of subtle tool breakage.
// Closing stays allowed (see CloseEditor), since it's the way out.
func (s *Session) checkAddonVersion(editor *Editor) error {
	if AddonCompatible(editor.AddonVersion) {
		return nil
	}

	reported := editor.AddonVersion
	if reported == "" {
		reported = "unknown (before " + Version + ")"
	}
	return NewUserError(
		fmt.Sprintf("the editor for '%s' is running godai addon %s, which doesn't match this godai (%s)",
			editor.ProjectPath, reported, Version),
		ErrAddonVersionMismatch,
		[]string{
			"Relaunch it with the matching addon: `godai editor restart`",
			"Or close it and open it again: `godai editor close`, then `godai project open`",
		})
}

func (s *Session) CallEditorTool(ctx context.Context, projectPath, name string, args Args, opts CallOptions) (*ToolResult, error) {
	editor, err := s.resolveEditor(ctx, projectPath, opts.Wait)
	if err != nil {
		return nil, err
	}

	if name != "close_editor" {
		if err := s.checkAddonVersion(editor); err != nil {
			return nil, err
		}
	}

	raw, err := args.Raw()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, s.config.EditorToolTimeout)
	defer cancel()

	resp, err := editor.conn.CallMethod(ctx, "tools/call", &callToolParams{
		Name:      name,
		Arguments: raw,
		Meta: map[string]any{
			timeoutMetaKey:    s.config.EditorToolTimeout.Milliseconds(),
			clientKindMetaKey: s.getClientKind(),
		},
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, &EditorRPCError{RPC: resp.Error}
	}

	result, err := parseToolResult(resp.Result)
	if err != nil {
		// Forwarding is meant to be lossless, so a result we can't model still
		// has to reach the caller.
		slog.Warn("unable to parse the editor's tool result; passing it through unchanged", "tool", name, "error", err)
		return &ToolResult{Raw: resp.Result, Unparsed: true}, nil
	}

	return result, nil
}

func (s *Session) resolveEditor(ctx context.Context, projectPath string, wait time.Duration) (*Editor, error) {
	if wait <= 0 {
		return s.EditorFor(projectPath)
	}

	if err := s.ensureStarted(ctx); err != nil {
		return nil, err
	}
	s.ScanNow()

	waitCtx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	return s.WaitForEditor(waitCtx, projectPath)
}

func parseToolResult(raw json.RawMessage) (*ToolResult, error) {
	result := ToolResult{Raw: raw}
	if len(raw) == 0 {
		return &result, nil
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unable to parse the editor's tool result: %w", err)
	}
	return &result, nil
}

func callEditorToolInto(ctx context.Context, conn *godot.Connection, name string, params json.RawMessage, ret any) error {
	resp, err := conn.CallMethod(ctx, "tools/call", callToolParams{
		Name:      name,
		Arguments: params,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return fmt.Errorf("error calling tool: %v", resp.Error)
	}

	result, err := parseToolResult(resp.Result)
	if err != nil {
		return fmt.Errorf("error parsing tool result: %v", resp.Result)
	}

	if result.StructuredContent != nil {
		return json.Unmarshal(result.StructuredContent, ret)
	}
	if len(result.Content) > 0 {
		return json.Unmarshal([]byte(result.Content[0].Text), ret)
	}

	return nil
}

// restartReconnectTimeout bounds how long we wait for the editor to come back
// after restarting (it has to relaunch and re-scan the project).
const restartReconnectTimeout = 180 * time.Second

func (s *Session) RestartEditor(ctx context.Context, projectPath string, args Args) error {
	editor, err := s.EditorFor(projectPath)
	if err != nil {
		return err
	}

	// An editor-side restart relaunches with whatever addon is already in the
	// project, so a mismatched editor needs the full cycle: close, install the
	// matching addon, and launch again.
	if !AddonCompatible(editor.AddonVersion) {
		return s.stopThenStartEditor(ctx, editor, args)
	}

	if err := s.callAndHonorRefusal(ctx, editor, "restart_editor", args, "the editor did not restart"); err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, restartReconnectTimeout)
	defer cancel()
	if _, err := s.waitForReconnect(waitCtx, projectPath, editor.conn); err != nil {
		return NewUserError("the editor did not reconnect after restarting", err, nil)
	}

	return nil
}

// StopThenStartEditor is the full restart cycle - close, reinstall the addon, launch
// again - regardless of whether the addon matches.
func (s *Session) StopThenStartEditor(ctx context.Context, projectPath string, args Args) error {
	editor, err := s.EditorFor(projectPath)
	if err != nil {
		return err
	}
	return s.stopThenStartEditor(ctx, editor, args)
}

func (s *Session) stopThenStartEditor(ctx context.Context, editor *Editor, args Args) error {
	projectPath := editor.ProjectPath
	headless, offscreen := editor.Headless, editor.Offscreen

	slog.Info("the editor's addon doesn't match; closing and reopening it",
		"projectPath", projectPath, "addonVersion", editor.AddonVersion, "version", Version)

	if err := s.CloseEditor(ctx, projectPath, args); err != nil {
		return err
	}

	// OpenProject installs the embedded addon before launching.
	if _, err := s.OpenProject(ctx, projectPath, OpenProjectOptions{Headless: headless, Offscreen: offscreen}); err != nil {
		return err
	}
	return nil
}

// closeShutdownTimeout bounds how long we wait for the editor to drop its
// connection and exit after the user confirms the close (it still has to save
// and shut down).
const closeShutdownTimeout = 120 * time.Second

func (s *Session) CloseEditor(ctx context.Context, projectPath string, args Args) error {
	editor, err := s.EditorFor(projectPath)
	if err != nil {
		return err
	}

	if err := s.callAndHonorRefusal(ctx, editor, "close_editor", args, "the editor did not close"); err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, closeShutdownTimeout)
	defer cancel()
	if err := s.waitForDisconnect(waitCtx, projectPath, editor.conn); err != nil {
		return NewUserError("the editor did not disconnect after closing", err, nil)
	}

	// The editor saves its state (e.g. editor settings) during teardown, after
	// the connection drops, so the close isn't done until the process is gone.
	if pid := editor.conn.GetPID(); pid > 0 {
		if err := godot.WaitForProcessExit(waitCtx, pid); err != nil {
			return NewUserError("the editor did not exit after closing", err, nil)
		}
	}

	s.unmarkUnattendedProject(projectPath)

	return nil
}

func (s *Session) callAndHonorRefusal(ctx context.Context, editor *Editor, name string, args Args, refusedMessage string) error {
	if args == nil {
		args = Args{}
	}
	args.Delete("project_path")

	raw, err := args.Raw()
	if err != nil {
		return err
	}

	editor.conn.ExpectClose(true)

	resp, callErr := editor.conn.CallMethod(ctx, "tools/call", &callToolParams{
		Name:      name,
		Arguments: raw,
	})
	if callErr != nil || resp == nil {
		return nil
	}

	if resp.Error != nil {
		editor.conn.ExpectClose(false)
		return fmt.Errorf("error calling %s in the editor: %v", name, resp.Error)
	}

	result, err := parseToolResult(resp.Result)
	if err == nil && result.IsError {
		editor.conn.ExpectClose(false)

		message := refusedMessage
		if text := result.ErrorMessage(); text != "" {
			message = text
		}
		return NewUserError(message, ErrToolFailed, nil)
	}

	return nil
}

// shutdownCloseTimeout bounds how long we wait for an unattended editor to
// save and shut down when the session closes.
const shutdownCloseTimeout = 30 * time.Second

// closeUnattendedEditors shuts down the headless and offscreen editors we
// launched. It runs at exit, so it uses fresh contexts rather than the
// (now-cancelled) run context. We look up the live connection by project path,
// so this still works after an editor has restarted with a new connection.
//
// We only close an editor that is *currently* unattended. If the user killed
// our editor and launched their own (windowed) editor for the same project,
// that replacement is left alone.
func (s *Session) closeUnattendedEditors() {
	s.unattendedMutex.Lock()
	projects := make([]string, 0, len(s.unattendedProjects))
	for p := range s.unattendedProjects {
		projects = append(projects, p)
	}
	s.unattendedProjects = make(map[string]string)
	s.unattendedMutex.Unlock()

	var wg sync.WaitGroup
	for _, projectPath := range projects {
		editor := s.findEditor(projectPath)
		if editor == nil || !(editor.Headless || editor.Offscreen) {
			// Already gone (crashed, or closed by the user), or replaced by an
			// editor we didn't launch.
			continue
		}

		editor.conn.ExpectClose(true)

		wg.Add(1)
		go func(projectPath string, conn *godot.Connection) {
			defer wg.Done()

			// An unattended editor has no user to prompt, so close_editor saves
			// and quits on its own.
			ctx, cancel := context.WithTimeout(context.Background(), shutdownCloseTimeout)
			defer cancel()
			if _, err := conn.CallMethod(ctx, "tools/call", &callToolParams{
				Name:      "close_editor",
				Arguments: json.RawMessage("{}"),
			}); err != nil && !errors.Is(err, godot.ErrConnectionClosed) {
				slog.Error("error closing unattended editor on shutdown", "projectPath", projectPath, "error", err)
			}
		}(projectPath, editor.conn)
	}
	wg.Wait()
}
