package isolation_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/godot"
	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/internal/isolationtest"
)

func TestEnvCreatesTheHomeDirs(t *testing.T) {
	is := is.New(t)

	base := t.TempDir()
	_, err := isolation.Env(base)
	is.NoErr(err)

	for _, dir := range []string{isolation.ConfigHome(base), isolation.DataHome(base), isolation.CacheHome(base)} {
		info, err := os.Stat(dir)
		is.NoErr(err)
		is.True(info.IsDir())
	}
}

// The accessors promise where Godot will put things once Env is applied, so
// they must agree with the real path resolution.
func TestEnvRedirectsGodotEditorPaths(t *testing.T) {
	is := is.New(t)
	base := isolationtest.Isolate(t)

	dataPath, err := godot.GetEditorDataPath()
	is.NoErr(err)
	is.Equal(dataPath, isolation.GodotEditorDataDir(base))

	settingsPath, err := godot.GetEditorSettingsPath()
	is.NoErr(err)
	is.Equal(filepath.Dir(settingsPath), isolation.ConfigHome(base))

	cachePath, err := godot.GetEditorCachePath()
	is.NoErr(err)
	is.Equal(filepath.Dir(cachePath), isolation.CacheHome(base))
}
