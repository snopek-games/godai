package godot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	enginesDirName   = "engines"
	enginesFileName  = "engines.json"
	engineRecordName = "godai-engine.json"

	versionQueryTimeout = 10 * time.Second
)

var (
	ErrEngineNotInstalled = errors.New("that version of Godot isn't installed")
	ErrEngineInstalled    = errors.New("that version of Godot is already installed")
	ErrLinkNameIsVersion  = errors.New("a linked engine can't be named after a Godot version")
)

// Engine is a Godot editor that Godai can run: either one it installed, or one
// the user pointed it at with `godai engine link`.
type Engine struct {
	Name    string        `json:"version"`
	Version EngineVersion `json:"-"`
	Path    string        `json:"path"`
	Dir     string        `json:"-"`
	Linked  bool          `json:"linked"`
}

type engineRecord struct {
	Version    string `json:"version"`
	Executable string `json:"executable"`
}

type linkedEngines struct {
	Linked map[string]linkedEngine `json:"linked,omitempty"`
}

type linkedEngine struct {
	Path    string `json:"path"`
	Version string `json:"version,omitempty"`
}

type EngineManagerConfig struct {
	CacheDir     string
	ConfigDir    string
	TemplatesDir string
	Client       *BuildsClient
}

type EngineManager struct {
	cacheDir     string
	enginesDir   string
	linksPath    string
	templatesDir string
	client       *BuildsClient
}

func NewEngineManager(config EngineManagerConfig) (*EngineManager, error) {
	if config.CacheDir == "" {
		return nil, errors.New("no cache directory to install Godot into")
	}
	if config.ConfigDir == "" {
		return nil, errors.New("no config directory to record linked engines in")
	}

	templatesDir := config.TemplatesDir
	if templatesDir == "" {
		var err error
		if templatesDir, err = GetExportTemplatesPath(); err != nil {
			return nil, err
		}
	}

	client := config.Client
	if client == nil {
		client = NewBuildsClient(BuildsClientConfig{CacheDir: config.CacheDir})
	}

	return &EngineManager{
		cacheDir:     config.CacheDir,
		enginesDir:   filepath.Join(config.CacheDir, enginesDirName),
		linksPath:    filepath.Join(config.ConfigDir, enginesFileName),
		templatesDir: templatesDir,
		client:       client,
	}, nil
}

func (m *EngineManager) EnginesDir() string {
	return m.enginesDir
}

// List returns every engine Godai can run, installed ones first.
func (m *EngineManager) List() ([]Engine, error) {
	engines := []Engine{}

	entries, err := os.ReadDir(m.enginesDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		engine, err := m.installedEngine(entry.Name())
		if err != nil {
			slog.Debug("ignoring an unreadable engine install", "name", entry.Name(), "error", err)
			continue
		}
		engines = append(engines, *engine)
	}

	slices.SortFunc(engines, func(a, b Engine) int { return b.Version.Compare(a.Version) })

	links, err := m.loadLinks()
	if err != nil {
		return nil, err
	}
	for _, name := range slices.Sorted(maps.Keys(links.Linked)) {
		engines = append(engines, linkedEngineFor(name, links.Linked[name]))
	}

	return engines, nil
}

// Find looks up an engine by the name the user typed, which is either a linked
// name or a version.
func (m *EngineManager) Find(name string) (*Engine, error) {
	links, err := m.loadLinks()
	if err != nil {
		return nil, err
	}
	if link, ok := links.Linked[name]; ok {
		engine := linkedEngineFor(name, link)
		return &engine, nil
	}

	version, err := ParseEngineVersion(name)
	if err != nil {
		return nil, err
	}

	engine, err := m.installedEngine(version.String())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrEngineNotInstalled, version)
		}
		return nil, err
	}
	return engine, nil
}

func (m *EngineManager) installedEngine(name string) (*Engine, error) {
	dir := filepath.Join(m.enginesDir, name)

	b, err := os.ReadFile(filepath.Join(dir, engineRecordName))
	if err != nil {
		return nil, err
	}

	record := engineRecord{}
	if err := json.Unmarshal(b, &record); err != nil {
		return nil, err
	}

	version, err := ParseEngineVersion(record.Version)
	if err != nil {
		return nil, err
	}

	return &Engine{
		Name:    version.String(),
		Version: version,
		Path:    filepath.Join(dir, filepath.FromSlash(record.Executable)),
		Dir:     dir,
	}, nil
}

func (m *EngineManager) Installed(version EngineVersion) bool {
	_, err := m.installedEngine(version.String())
	return err == nil
}

type DownloadOptions struct {
	Progress        Progress
	AllowUnverified bool
	Replace         bool
}

func (m *EngineManager) Install(ctx context.Context, version EngineVersion, opts DownloadOptions) (*Engine, error) {
	installDir := filepath.Join(m.enginesDir, version.String())
	if _, err := os.Stat(installDir); err == nil && !opts.Replace {
		return nil, fmt.Errorf("%w: %s", ErrEngineInstalled, version)
	}

	assetName, err := EngineAssetName(version)
	if err != nil {
		return nil, err
	}

	unpackDir, cleanup, err := m.downloadAndUnpack(ctx, version, assetName, "", m.enginesDir, opts)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	executable, err := FindEngineExecutable(unpackDir)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(executable, 0o755); err != nil {
		return nil, err
	}

	relative, err := filepath.Rel(unpackDir, executable)
	if err != nil {
		return nil, err
	}
	record, err := json.Marshal(engineRecord{
		Version:    version.String(),
		Executable: filepath.ToSlash(relative),
	})
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(unpackDir, engineRecordName), record, 0o644); err != nil {
		return nil, err
	}

	// Only remove the old install once the new one is ready to be moved into place.
	if err := os.RemoveAll(installDir); err != nil {
		return nil, err
	}
	if err := os.Rename(unpackDir, installDir); err != nil {
		return nil, fmt.Errorf("installing %s: %w", version, err)
	}

	return m.installedEngine(version.String())
}

func (m *EngineManager) downloadAndUnpack(ctx context.Context, version EngineVersion, assetName, stripPrefix, unpackParent string, opts DownloadOptions) (string, func(), error) {
	release, err := m.client.Release(ctx, version.Tag())
	if err != nil {
		return "", nil, err
	}

	asset, ok := release.Asset(assetName)
	if !ok {
		return "", nil, fmt.Errorf("%w: the %s release has no %s", ErrAssetNotFound, release.TagName, assetName)
	}

	if err := os.MkdirAll(m.cacheDir, 0o755); err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(unpackParent, 0o755); err != nil {
		return "", nil, err
	}

	archive, err := os.CreateTemp(m.cacheDir, ".download-*")
	if err != nil {
		return "", nil, err
	}
	archivePath := archive.Name()
	archive.Close()
	defer os.Remove(archivePath)

	sum, err := m.client.Download(ctx, asset, archivePath, opts.Progress)
	if err != nil {
		return "", nil, err
	}

	if opts.AllowUnverified {
		slog.Warn("installing a download that wasn't checked against the published checksums", "asset", assetName)
	} else if err := m.client.VerifyChecksum(ctx, release, assetName, sum); err != nil {
		return "", nil, err
	}

	unpackDir, err := os.MkdirTemp(unpackParent, ".unpack-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(unpackDir) }

	if err := extractZip(archivePath, unpackDir, stripPrefix); err != nil {
		cleanup()
		return "", nil, err
	}

	return unpackDir, cleanup, nil
}

// Find the newest installed engine that a project uses.
func (m *EngineManager) FindForProject(engine ProjectEngine) (*Engine, bool, error) {
	engines, err := m.List()
	if err != nil {
		return nil, false, err
	}

	var best *Engine
	for i, candidate := range engines {
		if candidate.Linked || !engine.Matches(candidate.Version) {
			continue
		}
		if best == nil || betterMatch(candidate.Version, best.Version) {
			best = &engines[i]
		}
	}

	return best, best != nil, nil
}

func betterMatch(candidate, best EngineVersion) bool {
	if candidate.IsStable() != best.IsStable() {
		return candidate.IsStable()
	}
	return candidate.Compare(best) > 0
}

// Find the newest stable release of the engine a project uses, whether or not it's installed.
func (m *EngineManager) LatestForProject(ctx context.Context, engine ProjectEngine) (EngineVersion, error) {
	versions, err := m.Search(ctx, SearchOptions{Filter: fmt.Sprintf("%d.%d", engine.Major, engine.Minor)})
	if err != nil {
		return EngineVersion{}, err
	}

	// The filter matches on text, so "4.1" also finds 4.10.
	for _, version := range versions {
		if version.Major == engine.Major && version.Minor == engine.Minor {
			version.Mono = engine.Mono
			return version, nil
		}
	}

	return EngineVersion{}, fmt.Errorf("%w: Godot %s", ErrReleaseNotFound, engine)
}

type SearchOptions struct {
	Filter     string
	IncludePre bool
	Refresh    bool
}

// Search lists the versions published on godotengine/godot-builds, newest first.
// A filter matches from the start of the version, so "4.5" finds 4.5 and 4.5.1,
// but not 3.4.5.
func (m *EngineManager) Search(ctx context.Context, opts SearchOptions) ([]EngineVersion, error) {
	tags, err := m.client.ReleaseTags(ctx, opts.Refresh)
	if err != nil {
		return nil, err
	}

	filter := strings.ToLower(strings.TrimSpace(opts.Filter))

	versions := []EngineVersion{}
	for _, tag := range tags {
		version, err := ParseEngineVersion(tag)
		if err != nil {
			slog.Debug("ignoring a release whose tag isn't a version", "tag", tag, "error", err)
			continue
		}
		if !opts.IncludePre && !version.IsStable() {
			continue
		}
		if filter != "" && !strings.HasPrefix(version.Tag(), filter) {
			continue
		}
		versions = append(versions, version)
	}

	slices.SortFunc(versions, func(a, b EngineVersion) int { return b.Compare(a) })
	return versions, nil
}

func (m *EngineManager) Remove(name string) error {
	engine, err := m.Find(name)
	if err != nil {
		return err
	}

	if engine.Linked {
		links, err := m.loadLinks()
		if err != nil {
			return err
		}
		delete(links.Linked, engine.Name)
		return m.saveLinks(links)
	}

	return os.RemoveAll(engine.Dir)
}

func (m *EngineManager) Link(ctx context.Context, name, executable string) (*Engine, error) {
	if _, err := ParseEngineVersion(name); err == nil {
		return nil, fmt.Errorf("%w: %s is one, so `godai engine install %s` would clash with it", ErrLinkNameIsVersion, name, name)
	}

	links, err := m.loadLinks()
	if err != nil {
		return nil, err
	}
	if links.Linked == nil {
		links.Linked = map[string]linkedEngine{}
	}

	link := linkedEngine{Path: executable}
	version, err := QueryEngineVersion(ctx, executable)
	if err != nil {
		slog.Warn("unable to tell which version of Godot this is", "executable", executable, "error", err)
	} else {
		link.Version = version.String()
	}
	links.Linked[name] = link

	if err := m.saveLinks(links); err != nil {
		return nil, err
	}

	engine := linkedEngineFor(name, link)
	return &engine, nil
}

func QueryEngineVersion(ctx context.Context, executable string) (EngineVersion, error) {
	ctx, cancel := context.WithTimeout(ctx, versionQueryTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, executable, "--version").Output()
	if err != nil {
		return EngineVersion{}, err
	}

	return ParseVersionOutput(string(out))
}

func linkedEngineFor(name string, link linkedEngine) Engine {
	engine := Engine{Name: name, Path: link.Path, Linked: true}
	if link.Version == "" {
		return engine
	}

	version, err := ParseEngineVersion(link.Version)
	if err != nil {
		slog.Debug("ignoring the recorded version of a linked engine", "name", name, "version", link.Version, "error", err)
		return engine
	}

	engine.Version = version
	return engine
}

func (m *EngineManager) loadLinks() (*linkedEngines, error) {
	links := &linkedEngines{Linked: map[string]linkedEngine{}}

	b, err := os.ReadFile(m.linksPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return links, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(b, links); err != nil {
		return nil, fmt.Errorf("reading %s: %w", m.linksPath, err)
	}
	if links.Linked == nil {
		links.Linked = map[string]linkedEngine{}
	}

	return links, nil
}

func (m *EngineManager) saveLinks(links *linkedEngines) error {
	if err := os.MkdirAll(filepath.Dir(m.linksPath), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(links, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.linksPath, append(b, '\n'), 0o644)
}
