package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"
	"gitlab.com/snopek-games/godai/internal/fakebin"
	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/internal/isolationtest"

	"github.com/matryer/is"
)

func TestEngineRemoveUnsetsTheDefault(t *testing.T) {
	is := is.New(t)

	dir := isolationtest.Isolate(t)
	configPath := filepath.Join(isolation.GodaiConfigDir(dir), "config.json")

	version := linkTestEngine(t, dir)
	other := linkOtherTestEngine(t, dir)

	_, err := runCLI(t, []string{"godai", "config", "--set", core.SettingGodotVersion + "=" + version})
	is.NoErr(err)

	_, err = runCLI(t, []string{"godai", "engine", "remove", other})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath).GodotVersion, version) // untouched by removing the other engine

	_, err = runCLI(t, []string{"godai", "engine", "remove", version})
	is.NoErr(err)
	is.Equal(readSavedConfig(t, configPath).GodotVersion, "")
}

func TestEngineLinkRecordsTheVersionTheBuildReports(t *testing.T) {
	is := is.New(t)

	dir := isolationtest.Isolate(t)

	linkTestEngine(t, dir)

	out, err := runCLI(t, []string{"godai", "engine", "list"})
	is.NoErr(err)
	is.True(strings.Contains(out, "test-build (linked: 4.5-stable)"))
}

func linkOtherTestEngine(t *testing.T, dir string) string {
	t.Helper()

	godotPath, err := fakebin.Exit(filepath.Join(dir, "other-godot"), 0)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := runCLI(t, []string{"godai", "engine", "link", "other-build", godotPath}); err != nil {
		t.Fatal(err)
	}
	return "other-build"
}
