package godot

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/snopek-games/godai/internal/godot/variant"
)

type ProjectManagerEntry struct {
	ProjectPath string
	Favorite    bool
}

// Lists the projects in Godot's project manager.
func GetProjectManagerEntries() ([]ProjectManagerEntry, error) {
	l := []ProjectManagerEntry{}

	editorDataPath, err := GetEditorDataPath()
	if err != nil {
		return nil, err
	}

	projectListPath := filepath.Join(editorDataPath, "projects.cfg")
	cf, err := LoadConfigFile(projectListPath)
	if err != nil {
		return nil, err
	}

	for _, path := range cf.ListSections() {
		favorite, _ := cf.GetBool(path, "favorite")
		l = append(l, ProjectManagerEntry{
			ProjectPath: path,
			Favorite:    favorite,
		})
	}

	return l, nil
}

type Project struct {
	path string
}

func checkProjectPath(path string) error {
	pathInfo, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !pathInfo.IsDir() {
		return errors.New("project path must be directory")
	}

	configInfo, err := os.Stat(filepath.Join(path, "project.godot"))
	if err != nil {
		return err
	}
	if configInfo.IsDir() || !configInfo.Mode().IsRegular() {
		return errors.New("project config must be regular file")
	}

	return nil
}

func IsValidProject(path string) bool {
	if err := checkProjectPath(path); err != nil {
		return false
	}
	return true
}

func ProjectFromPath(path string) (*Project, error) {
	if err := checkProjectPath(path); err != nil {
		return nil, err
	}

	p := &Project{
		path: path,
	}
	return p, nil
}

func (p *Project) GetPath() string {
	return p.path
}

func (p *Project) GetConfigFile() (*ConfigFile, error) {
	return LoadConfigFile(filepath.Join(p.path, "project.godot"))
}

// ProjectEngine is the engine version listed in `project.godot`, which is
// major.minor version it was last saved with.
type ProjectEngine struct {
	Major int
	Minor int
	Mono  bool
}

func (e ProjectEngine) String() string {
	version := fmt.Sprintf("%d.%d", e.Major, e.Minor)
	if e.Mono {
		version += " (C#)"
	}
	return version
}

func (e ProjectEngine) Matches(version EngineVersion) bool {
	return version.Major == e.Major && version.Minor == e.Minor && version.Mono == e.Mono
}

const projectFeaturesKey = "config/features"

func (p *Project) GetEngine() (ProjectEngine, bool, error) {
	config, err := p.GetConfigFile()
	if err != nil {
		return ProjectEngine{}, false, err
	}

	value, ok := config.Get("application", projectFeaturesKey)
	if !ok {
		return ProjectEngine{}, false, nil
	}

	features, ok := value.(variant.PackedStringArray)
	if !ok {
		return ProjectEngine{}, false, fmt.Errorf("application/%s isn't a list of features", projectFeaturesKey)
	}

	engine := ProjectEngine{Mono: p.usesCSharp(features)}
	for _, feature := range features {
		if major, minor, ok := parseFeatureVersion(feature); ok {
			engine.Major, engine.Minor = major, minor
			return engine, true, nil
		}
	}

	return ProjectEngine{}, false, nil
}

func (p *Project) usesCSharp(features variant.PackedStringArray) bool {
	if slices.Contains(features, "C#") {
		return true
	}

	matches, err := filepath.Glob(filepath.Join(p.path, "*.csproj"))
	if err != nil {
		return false
	}
	return len(matches) > 0
}

func parseFeatureVersion(feature string) (major, minor int, ok bool) {
	parts := strings.Split(feature, ".")
	if len(parts) < 2 {
		return 0, 0, false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minor, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}

	return major, minor, true
}
