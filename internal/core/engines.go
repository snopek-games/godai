package core

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"gitlab.com/snopek-games/godai/internal/godot"
)

func NewEngineManager(configPath string) (*godot.EngineManager, error) {
	cachePath, err := GetCachePath()
	if err != nil {
		return nil, err
	}

	if configPath == "" {
		if configPath, err = GetConfigPath(); err != nil {
			return nil, err
		}
	}

	return godot.NewEngineManager(godot.EngineManagerConfig{
		CacheDir:  cachePath,
		ConfigDir: filepath.Dir(configPath),
	})
}

type DownloadOptions = godot.DownloadOptions

type EngineListing struct {
	Version string `json:"version"`
	// Only set for linked engines.
	ReportedVersion string `json:"reported_version,omitempty"`
	Path            string `json:"path"`
	// Empty for a linked engine, whose templates aren't Godai's to know about.
	Templates godot.TemplateState `json:"templates,omitempty"`
	Linked    bool                `json:"linked"`
	Default   bool                `json:"default"`
}

type EngineSearchResult struct {
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
}

type EngineInstallResult struct {
	Engine        EngineListing `json:"engine"`
	BecameDefault bool          `json:"became_default"`
}

type EngineRemovalResult struct {
	Version          string `json:"version"`
	Linked           bool   `json:"linked"`
	TemplatesRemoved bool   `json:"templates_removed"`
	DefaultCleared   bool   `json:"default_cleared"`
}

func (s *Session) ListEngines() ([]EngineListing, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	engines, err := manager.List()
	if err != nil {
		return nil, err
	}

	listings := make([]EngineListing, 0, len(engines))
	for _, engine := range engines {
		listings = append(listings, s.engineListing(manager, engine))
	}
	return listings, nil
}

func (s *Session) DescribeEngine(name string) (*EngineListing, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	engine, err := s.FindEngine(name)
	if err != nil {
		return nil, err
	}

	listing := s.engineListing(manager, *engine)
	return &listing, nil
}

func (s *Session) SearchEngines(ctx context.Context, filter string, includePre, refresh bool) ([]EngineSearchResult, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	versions, err := manager.Search(ctx, godot.SearchOptions{
		Filter:     filter,
		IncludePre: includePre,
		Refresh:    refresh,
	})
	if err != nil {
		return nil, err
	}

	installed, err := installedVersions(manager)
	if err != nil {
		return nil, err
	}

	results := make([]EngineSearchResult, 0, len(versions))
	for _, version := range versions {
		results = append(results, EngineSearchResult{version.String(), installed[version.String()]})
	}
	return results, nil
}

func (s *Session) InstallEngine(ctx context.Context, name string, opts DownloadOptions) (*EngineInstallResult, error) {
	version, err := godot.ParseEngineVersion(name)
	if err != nil {
		return nil, engineUserError(name, err)
	}

	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}
	if manager.Installed(version) && !opts.Replace {
		return nil, NewUserError(fmt.Sprintf("Godot %s is already installed", version),
			godot.ErrEngineInstalled, []string{"See what's installed: `godai engine list`"})
	}

	engine, err := manager.Install(ctx, version, opts)
	if err != nil {
		return nil, NewUserError(fmt.Sprintf("unable to install Godot %s", version), err, []string{
			"Check what there is: `godai engine search --all`",
		})
	}

	becameDefault, err := s.adoptDefaultVersion(engine.Name)
	if err != nil {
		return nil, err
	}

	return &EngineInstallResult{
		Engine:        s.engineListing(manager, *engine),
		BecameDefault: becameDefault,
	}, nil
}

func (s *Session) InstallTemplatesIfNeeded(ctx context.Context, name string, opts DownloadOptions) error {
	version, err := godot.ParseEngineVersion(name)
	if err != nil {
		return engineUserError(name, err)
	}

	manager, err := s.EngineManager()
	if err != nil {
		return err
	}

	templates, err := manager.Templates(version)
	if err != nil {
		return err
	}
	if templates.State() != godot.TemplatesNone {
		return nil
	}

	return manager.InstallTemplates(ctx, version, opts)
}

func (s *Session) RemoveEngine(name string, withTemplates bool) (*EngineRemovalResult, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	engine, err := s.FindEngine(name)
	if err != nil {
		return nil, err
	}
	if err := manager.Remove(engine.Name); err != nil {
		return nil, err
	}

	result := &EngineRemovalResult{Version: engine.Name, Linked: engine.Linked}

	if withTemplates && !engine.Linked && manager.TemplatesInstalled(engine.Version) {
		if err := manager.RemoveTemplates(engine.Version); err != nil {
			return nil, err
		}
		result.TemplatesRemoved = true
	}

	if result.DefaultCleared, err = s.dropDefaultVersion(engine.Name); err != nil {
		return nil, err
	}

	return result, nil
}

func (s *Session) LinkEngine(ctx context.Context, name, path string) (*EngineListing, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	executable, err := ResolveGodotExecutable(path)
	if err != nil {
		return nil, NewUserError("invalid Godot path: "+path, err, []string{
			"Give the path to a Godot executable, not to a directory",
		})
	}

	engine, err := manager.Link(ctx, name, executable)
	if err != nil {
		return nil, NewUserError(fmt.Sprintf("unable to link %s: %v", name, err), err, []string{
			"The name can be anything that isn't a Godot version, so that it can't be confused with one",
		})
	}

	listing := s.engineListing(manager, *engine)
	return &listing, nil
}

func (s *Session) engineListing(manager *godot.EngineManager, engine godot.Engine) EngineListing {
	listing := EngineListing{
		Version: engine.Name,
		Path:    engine.Path,
		Linked:  engine.Linked,
		Default: engine.Name == s.config.GodotVersion,
	}

	if engine.Linked {
		if engine.Version.Known() {
			listing.ReportedVersion = engine.Version.String()
		}
		return listing
	}

	templates, err := manager.Templates(engine.Version)
	if err != nil {
		slog.Debug("unable to look at the export templates", "version", engine.Name, "error", err)
		return listing
	}

	listing.Templates = templates.State()
	return listing
}

func (s *Session) adoptDefaultVersion(name string) (bool, error) {
	if s.config.SavedConfigPath == "" || SavedGodotVersion(s.config.SavedConfigPath) != "" {
		return false, nil
	}
	if err := s.SetConfig(SavedConfig{GodotVersion: name}); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Session) dropDefaultVersion(name string) (bool, error) {
	if s.config.SavedConfigPath == "" || SavedGodotVersion(s.config.SavedConfigPath) != name {
		return false, nil
	}
	if err := s.UnsetConfig([]string{SettingGodotVersion}); err != nil {
		return false, err
	}
	return true, nil
}

func installedVersions(manager *godot.EngineManager) (map[string]bool, error) {
	engines, err := manager.List()
	if err != nil {
		return nil, err
	}

	installed := map[string]bool{}
	for _, engine := range engines {
		installed[engine.Name] = true
	}
	return installed, nil
}

type InstallReporter func(version string) godot.Progress

// Sets the InstallReporter callback which is used to report when Godai auto
// installs an engine version (rather than the user asking for it).
func (s *Session) SetInstallReporter(f InstallReporter) {
	s.engineMutex.Lock()
	defer s.engineMutex.Unlock()
	s.installReporter = f
}

func (s *Session) startInstallReport(version string) godot.Progress {
	s.engineMutex.Lock()
	reporter := s.installReporter
	s.engineMutex.Unlock()

	if reporter == nil {
		return nil
	}
	return reporter(version)
}

func (s *Session) EngineManager() (*godot.EngineManager, error) {
	s.engineMutex.Lock()
	defer s.engineMutex.Unlock()

	if s.engines == nil && s.engineErr == nil {
		s.engines, s.engineErr = NewEngineManager(s.config.SavedConfigPath)
	}

	return s.engines, s.engineErr
}

func (s *Session) FindEngine(version string) (*godot.Engine, error) {
	manager, err := s.EngineManager()
	if err != nil {
		return nil, err
	}

	engine, err := manager.Find(version)
	if err != nil {
		return nil, engineUserError(version, err)
	}
	return engine, nil
}
