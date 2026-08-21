package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

func TestProjectGodotVersionRoundTrip(t *testing.T) {
	is := is.New(t)

	projectPath := t.TempDir()

	version, err := ProjectGodotVersion(projectPath)
	is.NoErr(err)
	is.Equal(version, "") // an unpinned project has no version

	is.NoErr(SetProjectGodotVersion(projectPath, "4.5-stable"))

	version, err = ProjectGodotVersion(projectPath)
	is.NoErr(err)
	is.Equal(version, "4.5-stable")

	pinned, err := UnsetProjectGodotVersion(projectPath)
	is.NoErr(err)
	is.True(pinned) // there was a pin to remove

	// Nothing else was in it, so the file goes rather than being left empty.
	_, err = os.Stat(ProjectConfigPath(projectPath))
	is.True(os.IsNotExist(err))

	pinned, err = UnsetProjectGodotVersion(projectPath)
	is.NoErr(err)
	is.True(!pinned) // nothing left to remove
}

func TestProjectConfigKeepsSettingsItDoesNotKnow(t *testing.T) {
	is := is.New(t)

	projectPath := t.TempDir()
	is.NoErr(os.WriteFile(ProjectConfigPath(projectPath), []byte(`{"something_else": "keep me"}`), 0o644))

	is.NoErr(SetProjectGodotVersion(projectPath, "4.5-stable"))
	_, err := UnsetProjectGodotVersion(projectPath)
	is.NoErr(err)

	raw, err := os.ReadFile(ProjectConfigPath(projectPath))
	is.NoErr(err)

	config := map[string]any{}
	is.NoErr(json.Unmarshal(raw, &config))
	is.Equal(config, map[string]any{"something_else": "keep me"})
}

func TestProjectConfigRejectsAVersionThatIsNotText(t *testing.T) {
	is := is.New(t)

	projectPath := t.TempDir()
	is.NoErr(os.WriteFile(filepath.Join(projectPath, ProjectConfigName), []byte(`{"godot_version": 4.5}`), 0o644))

	_, err := ProjectGodotVersion(projectPath)
	is.True(err != nil)
}
