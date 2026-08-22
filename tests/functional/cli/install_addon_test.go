package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

const autoloadLine = `Godai="*res://addons/godai/game/godai.gd"`

// The plugin's _enable_plugin() only runs when the user toggles the plugin on
// in the editor UI, so installs done by `project install-addon` and
// `project open` have to add the Godai autoload themselves.
func TestInstallAddonAddsAutoload(t *testing.T) {
	is := is.New(t)

	dir := newProjectDir(t, "autoload-install")

	godai(t, "project", "install-addon", dir)

	config := readProjectGodot(t, dir)
	is.Equal(strings.Count(config, autoloadLine), 1)
	is.True(strings.Contains(config, `enabled=PackedStringArray("res://addons/godai/plugin.cfg")`))

	// Installing again must not add a second copy.
	godai(t, "project", "install-addon", dir)
	is.Equal(strings.Count(readProjectGodot(t, dir), autoloadLine), 1)
}

func TestInstallAddonKeepsOtherAutoloads(t *testing.T) {
	is := is.New(t)

	dir := newProjectDir(t, "autoload-existing")
	appendProjectGodot(t, dir, "\n[autoload]\n\nMyGlobals=\"*res://globals.gd\"\n")

	godai(t, "project", "install-addon", dir)

	config := readProjectGodot(t, dir)
	is.Equal(strings.Count(config, autoloadLine), 1)
	is.True(strings.Contains(config, `MyGlobals="*res://globals.gd"`))
}

func TestInstallAddonRepairsAutoload(t *testing.T) {
	is := is.New(t)

	dir := newProjectDir(t, "autoload-stale")
	appendProjectGodot(t, dir, "\n[autoload]\n\nGodai=\"*res://somewhere/else.gd\"\n")

	godai(t, "project", "install-addon", dir)

	config := readProjectGodot(t, dir)
	is.Equal(strings.Count(config, autoloadLine), 1)
	is.True(!strings.Contains(config, "res://somewhere/else.gd"))
}

func TestProjectOpenAddsAutoload(t *testing.T) {
	is := is.New(t)

	godotBin, err := harness.FindGodot()
	is.NoErr(err)

	dir := newProjectDir(t, "autoload-open")

	// The editor godai spawns inherits this environment, so its instance file
	// lands where --editor-instances-path sends the CLI looking.
	env, err := isolation.Env(harness.IsolationDir(projectPath))
	is.NoErr(err)
	env = append(env, "GODAI_EDITOR_LOG=1")

	t.Cleanup(func() {
		_, _ = godaiWithEnv(env, "editor", "close", dir, "--skip-save")
	})

	out, err := godaiWithEnv(env, "project", "open", dir,
		"--godot-path", godotBin, "--headless", "--auto-approve")
	if err != nil {
		t.Fatalf("project open: %v\n%s", err, out)
	}

	is.Equal(strings.Count(readProjectGodot(t, dir), autoloadLine), 1)
}

// Creates a throwaway project, without the addon, beside the shared one so the
// package's --root covers it.
func newProjectDir(t *testing.T, name string) string {
	t.Helper()

	dir := filepath.Join(filepath.Dir(projectPath), name)
	if err := harness.CreateTestProject(dir, harness.ProjectOptions{Name: name}); err != nil {
		t.Fatalf("creating test project: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func readProjectGodot(t *testing.T, dir string) string {
	t.Helper()

	config, err := os.ReadFile(filepath.Join(dir, "project.godot"))
	if err != nil {
		t.Fatalf("reading project.godot: %v", err)
	}
	return string(config)
}

func appendProjectGodot(t *testing.T, dir string, extra string) {
	t.Helper()

	path := filepath.Join(dir, "project.godot")
	config, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading project.godot: %v", err)
	}
	if err := os.WriteFile(path, append(config, []byte(extra)...), 0o644); err != nil {
		t.Fatalf("writing project.godot: %v", err)
	}
}

func godaiWithEnv(env []string, args ...string) (string, error) {
	cmd := command(args...)
	if cmd.Env == nil {
		cmd.Env = os.Environ()
	}
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
