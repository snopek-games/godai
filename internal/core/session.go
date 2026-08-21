package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"sync"

	"gitlab.com/snopek-games/godai/internal/godot"
)

type Editor struct {
	ProjectPath string
	ProjectName string
	Headless    bool
	// GodotVersion is what the editor reported it is, named the way Godai
	// names versions. Empty when it wouldn't say, or said something that isn't
	// a version.
	GodotVersion string
	// AddonVersion is the godai addon version the editor reported during the
	// handshake. Empty when the addon predates version reporting.
	AddonVersion string

	conn *godot.Connection
}

type ClientInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Version string `json:"version"`
}

type Session struct {
	config Config

	connectionManager *godot.ConnectionManager
	startOnce         sync.Once
	startErr          error
	started           bool
	startedMutex      sync.Mutex

	editors        []*Editor
	editorsMutex   sync.RWMutex
	editorsChanged chan struct{}

	headlessProjects map[string]struct{}
	headlessMutex    sync.Mutex

	prompter      Prompter
	prompterMutex sync.RWMutex

	rootsProvider      func() []string
	clientInfoProvider func() (ClientInfo, map[string]any)
	providerMutex      sync.RWMutex

	updateAvailable      string
	updateInstallCommand string
	updateMutex          sync.RWMutex

	engines         *godot.EngineManager
	engineErr       error
	installReporter InstallReporter
	engineMutex     sync.Mutex
}

func New(config Config) (*Session, error) {
	s := &Session{
		config:           config,
		editors:          make([]*Editor, 0),
		editorsChanged:   make(chan struct{}),
		headlessProjects: make(map[string]struct{}),
		prompter:         NoPrompter{},
	}

	var scanner godot.ConnectionScanner
	if config.Scope == ScopeGlobal {
		scanner = &godot.GlobalConnectionScanner{InstancesPath: config.EditorInstancesPath}
	} else {
		scanner = &godot.ProjectConnectionScanner{
			InstancesPath: config.EditorInstancesPath,
			GetRootPaths:  s.RootPaths,
		}
	}

	s.connectionManager = godot.NewConnectionManager(godot.ConnectionManagerConfig{
		Scanner:      scanner,
		ScanInterval: config.EditorScanInterval,
		RetryDelay:   config.EditorRetryDelay,
		OnConnect:    s.onEditorConnect,
		OnDisconnect: s.onEditorDisconnect,
		RequestMeta:  map[string]any{godaiVersionMetaKey: Version},
	})

	return s, nil
}

func (s *Session) Config() Config {
	return s.config
}

func (s *Session) Global() bool {
	return s.config.Scope == ScopeGlobal
}

func (s *Session) SetPrompter(p Prompter) {
	s.prompterMutex.Lock()
	defer s.prompterMutex.Unlock()
	if p == nil {
		p = NoPrompter{}
	}
	s.prompter = p
}

func (s *Session) getPrompter() Prompter {
	s.prompterMutex.RLock()
	defer s.prompterMutex.RUnlock()
	return s.prompter
}

func (s *Session) SetRootsProvider(f func() []string) {
	s.providerMutex.Lock()
	defer s.providerMutex.Unlock()
	s.rootsProvider = f
}

func (s *Session) SetClientInfoProvider(f func() (ClientInfo, map[string]any)) {
	s.providerMutex.Lock()
	defer s.providerMutex.Unlock()
	s.clientInfoProvider = f
}

func (s *Session) RootPaths() []string {
	s.providerMutex.RLock()
	provider := s.rootsProvider
	s.providerMutex.RUnlock()

	if provider != nil {
		return provider()
	}
	return slices.Clone(s.config.RootPaths)
}

func (s *Session) clientInfo() (ClientInfo, map[string]any) {
	s.providerMutex.RLock()
	provider := s.clientInfoProvider
	s.providerMutex.RUnlock()

	if provider != nil {
		return provider()
	}
	return ClientInfo{Name: AppName, Title: AppTitle, Version: Version}, nil
}

func (s *Session) Start(ctx context.Context) error {
	s.startOnce.Do(func() {
		s.connectionManager.Start()

		s.startedMutex.Lock()
		s.started = true
		s.startedMutex.Unlock()
	})
	return s.startErr
}

// Scanning connects sockets and prunes stale instance files, so a command that
// only reads config must never trigger it.
func (s *Session) ensureStarted(ctx context.Context) error {
	return s.Start(ctx)
}

func (s *Session) isStarted() bool {
	s.startedMutex.Lock()
	defer s.startedMutex.Unlock()
	return s.started
}

func (s *Session) Close() {
	if !s.isStarted() {
		return
	}
	if s.config.CloseHeadlessOnExit {
		s.closeHeadlessEditors()
	}
	s.connectionManager.Stop()
}

func (s *Session) ScanNow() {
	if s.isStarted() {
		s.connectionManager.ScanNow()
	}
}

// Callers must hold editorsMutex for writing.
func (s *Session) notifyEditorsChanged() {
	close(s.editorsChanged)
	s.editorsChanged = make(chan struct{})
}

func (s *Session) waitForEditors(ctx context.Context, match func([]*Editor) (*Editor, bool)) (*Editor, error) {
	for {
		s.editorsMutex.RLock()
		changed := s.editorsChanged
		editor, ok := match(s.editors)
		s.editorsMutex.RUnlock()

		if ok {
			return editor, nil
		}

		select {
		case <-changed:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *Session) onEditorConnect(conn *godot.Connection) error {
	info, capabilities := s.clientInfo()
	params := struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities,omitempty"`
		ClientInfo      ClientInfo     `json:"clientInfo"`
	}{
		ProtocolVersion: EditorProtocolVersion,
		ClientInfo:      info,
		Capabilities:    capabilities,
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.config.EditorTimeout)
	defer cancel()

	initResp, err := conn.CallMethod(ctx, "initialize", params)
	if err != nil {
		return err
	}
	var initResult struct {
		ServerInfo struct {
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if len(initResp.Result) > 0 {
		if err := json.Unmarshal(initResp.Result, &initResult); err != nil {
			slog.Debug("unable to parse the editor's initialize result", "error", err)
		}
	}

	if err := conn.SendNotification(ctx, "notification/initialized", nil); err != nil {
		return err
	}

	var projectInfo struct {
		ProjectPath  string `json:"project_path"`
		ProjectName  string `json:"project_name"`
		Headless     bool   `json:"headless"`
		GodotVersion string `json:"godot_version"`
	}
	if err := callEditorToolInto(ctx, conn, "get_current_project", json.RawMessage("{}"), &projectInfo); err != nil {
		return err
	}

	realProjectPath, err := CanonicalPath(projectInfo.ProjectPath)
	if err != nil {
		return err
	}

	editor := &Editor{
		conn:         conn,
		ProjectPath:  realProjectPath,
		ProjectName:  projectInfo.ProjectName,
		Headless:     projectInfo.Headless,
		GodotVersion: editorVersionName(projectInfo.GodotVersion),
		AddonVersion: initResult.ServerInfo.Version,
	}

	s.editorsMutex.Lock()
	s.editors = append(s.editors, editor)
	s.notifyEditorsChanged()
	s.editorsMutex.Unlock()

	// Must stay after the editor is registered, or CheckForUpdate() can miss it.
	s.sendUpdateNotification(ctx, conn)

	return nil
}

func editorVersionName(reported string) string {
	if reported == "" {
		return ""
	}

	version, err := godot.ParseVersionOutput(reported)
	if err != nil {
		slog.Debug("unable to read the version the editor reported", "version", reported, "error", err)
		return ""
	}
	return version.String()
}

func (s *Session) onEditorDisconnect(conn *godot.Connection) {
	s.editorsMutex.Lock()
	defer s.editorsMutex.Unlock()

	remaining := make([]*Editor, 0, len(s.editors))
	for _, e := range s.editors {
		if e.conn != conn {
			remaining = append(remaining, e)
		}
	}
	s.editors = remaining
	s.notifyEditorsChanged()
}

func (s *Session) Editors() []Editor {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	out := make([]Editor, 0, len(s.editors))
	for _, e := range s.editors {
		out = append(out, *e)
	}
	return out
}

func (s *Session) findEditor(projectPath string) *Editor {
	s.editorsMutex.RLock()
	defer s.editorsMutex.RUnlock()

	for _, e := range s.editors {
		if e.ProjectPath == projectPath {
			return e
		}
	}
	return nil
}

func (s *Session) hasEditorForProject(projectPath string) bool {
	return s.findEditor(projectPath) != nil
}

// An editor is only registered once its handshake finishes, so a session that
// just started has to ask the scanner too or it will think nothing is open.
func (s *Session) hasRunningEditorForProject(projectPath string) bool {
	if s.hasEditorForProject(projectPath) {
		return true
	}
	if !s.isStarted() {
		return false
	}

	for _, path := range s.connectionManager.InstanceProjectPaths() {
		if canonical, err := CanonicalPath(path); err == nil && canonical == projectPath {
			return true
		}
	}
	return false
}

func noEditorError(projectPath string) *UserError {
	return NewUserError("not connected to the Godot editor for project: "+projectPath, ErrNoEditor, []string{
		"See what's open: `godai editor list` (MCP: the `list_open_projects` tool)",
		"Open this project's editor: `godai project open " + projectPath + "` (MCP: the `open_godot_project` tool)",
	})
}

func (s *Session) EditorFor(projectPath string) (*Editor, error) {
	if editor := s.findEditor(projectPath); editor != nil {
		return editor, nil
	}
	return nil, noEditorError(projectPath)
}

func (s *Session) WaitForEditor(ctx context.Context, projectPath string) (*Editor, error) {
	if err := s.ensureStarted(ctx); err != nil {
		return nil, err
	}

	editor, err := s.waitForEditors(ctx, func(editors []*Editor) (*Editor, bool) {
		for _, e := range editors {
			if e.ProjectPath == projectPath {
				return e, true
			}
		}
		return nil, false
	})
	if err != nil {
		return nil, noEditorError(projectPath)
	}
	return editor, nil
}

func (s *Session) WaitForAnyEditor(ctx context.Context) (*Editor, error) {
	if err := s.ensureStarted(ctx); err != nil {
		return nil, err
	}

	return s.waitForEditors(ctx, func(editors []*Editor) (*Editor, bool) {
		if len(editors) > 0 {
			return editors[0], true
		}
		return nil, false
	})
}

func (s *Session) waitForReconnect(ctx context.Context, projectPath string, oldConn *godot.Connection) (*Editor, error) {
	return s.waitForEditors(ctx, func(editors []*Editor) (*Editor, bool) {
		for _, e := range editors {
			if e.ProjectPath == projectPath && e.conn != oldConn {
				return e, true
			}
		}
		return nil, false
	})
}

func (s *Session) waitForDisconnect(ctx context.Context, projectPath string, oldConn *godot.Connection) error {
	_, err := s.waitForEditors(ctx, func(editors []*Editor) (*Editor, bool) {
		for _, e := range editors {
			if e.ProjectPath == projectPath && e.conn == oldConn {
				return nil, false
			}
		}
		return nil, true
	})
	return err
}

// markHeadlessProject records that we launched a headless editor for this
// project, so we can shut it down when the session closes.
func (s *Session) markHeadlessProject(projectPath string) {
	s.headlessMutex.Lock()
	defer s.headlessMutex.Unlock()
	s.headlessProjects[projectPath] = struct{}{}
}

// unmarkHeadlessProject forgets a headless editor we launched (e.g. because it
// was closed explicitly), so we don't try to close it again at shutdown.
func (s *Session) unmarkHeadlessProject(projectPath string) {
	s.headlessMutex.Lock()
	defer s.headlessMutex.Unlock()
	delete(s.headlessProjects, projectPath)
}

func (s *Session) HeadlessProjects() []string {
	s.headlessMutex.Lock()
	defer s.headlessMutex.Unlock()

	projects := make([]string, 0, len(s.headlessProjects))
	for p := range s.headlessProjects {
		projects = append(projects, p)
	}
	slices.Sort(projects)
	return projects
}

// Saves only the setting the user just gave us: the rest of the live config can
// come from a flag, the environment or $PATH, which we have no business keeping.
func (s *Session) logSaveSetting(name, value string) {
	update := SavedConfig{}
	if err := update.SetSetting(name, value); err != nil {
		slog.Error("error saving config", "setting", name, "error", err)
		return
	}
	if err := s.mergeSavedConfig(update); err != nil {
		slog.Error("error saving config", "setting", name, "error", err)
	}
}

func (s *Session) mergeSavedConfig(update SavedConfig) error {
	if s.config.SavedConfigPath == "" {
		return nil
	}

	if existing, err := LoadConfig(s.config.SavedConfigPath); err == nil {
		if update.GodotVersion == "" {
			update.GodotVersion = existing.GodotVersion
		}
		if update.ProjectBasePath == "" {
			update.ProjectBasePath = existing.ProjectBasePath
		}
		if update.UpdateCheck == "" {
			update.UpdateCheck = existing.UpdateCheck
		}
	}

	return SaveConfig(s.config.SavedConfigPath, &update)
}
