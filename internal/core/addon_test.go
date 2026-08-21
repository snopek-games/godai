package core

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai"
	"gitlab.com/snopek-games/godai/internal/godot"
)

func newTestProject(t *testing.T) *godot.Project {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), []byte("config_version=5\n\n[application]\n\nconfig/name=\"Test\"\n"), 0o644); err != nil {
		t.Fatalf("writing project.godot: %v", err)
	}

	project, err := godot.ProjectFromPath(dir)
	if err != nil {
		t.Fatalf("creating project: %v", err)
	}
	return project
}

func checkInstalledAddon(t *testing.T, project *godot.Project) {
	t.Helper()
	is := is.New(t)

	installedVersion, err := PluginVersion(os.DirFS(project.GetPath()))
	is.NoErr(err)
	is.Equal(installedVersion, Version)

	is.NoErr(fs.WalkDir(godai.AddonFS, addonFSPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		dstPath := filepath.Join(project.GetPath(), filepath.FromSlash(p))
		if _, err := os.Stat(dstPath); err != nil {
			t.Errorf("addon file not installed: %s", p)
		}
		return nil
	}))
}

func TestInstallAddonIntoProjectWithoutAddonsFolder(t *testing.T) {
	is := is.New(t)

	project := newTestProject(t)

	is.NoErr(installAddon(project, false))
	checkInstalledAddon(t, project)
}

func TestInstallAddonReplacesStaleVersion(t *testing.T) {
	is := is.New(t)

	project := newTestProject(t)
	installedPath := filepath.Join(project.GetPath(), "addons", "godai")
	is.NoErr(os.MkdirAll(installedPath, 0o755))
	is.NoErr(os.WriteFile(filepath.Join(installedPath, "plugin.cfg"), []byte("[plugin]\n\nversion=\"0.0.0-stale\"\n"), 0o644))
	is.NoErr(os.WriteFile(filepath.Join(installedPath, "leftover.gd"), []byte("# from the old version\n"), 0o644))

	is.NoErr(installAddon(project, false))
	checkInstalledAddon(t, project)

	_, err := os.Stat(filepath.Join(installedPath, "leftover.gd"))
	is.True(os.IsNotExist(err)) // stale files are removed, not merged
}

func TestInstallAddonForceReplace(t *testing.T) {
	is := is.New(t)

	project := newTestProject(t)

	is.NoErr(installAddon(project, false))
	is.NoErr(installAddon(project, true))
	checkInstalledAddon(t, project)
}
