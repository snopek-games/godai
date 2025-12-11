package godot

import (
	"errors"
	"os"
	"path/filepath"
)

// @todo Add a way to list projects from the `~/.local/share/godot/projects.cfg`

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
