package godot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

func TestParseTemplateDirName(t *testing.T) {
	for _, tc := range []struct{ dir, want string }{
		{"4.5.stable", "4.5-stable"},
		{"4.4.1.stable", "4.4.1-stable"},
		{"4.5.stable.mono", "4.5-stable-mono"},
		{"4.4.1.stable.mono", "4.4.1-stable-mono"},
		{"4.6.beta3", "4.6-beta3"},
		{"4.8.dev", "4.8-dev"},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			is := is.New(t)

			version, err := ParseTemplateDirName(tc.dir)
			is.NoErr(err)
			is.Equal(version.String(), tc.want)
			is.Equal(version.TemplateDirName(), tc.dir)
		})
	}
}

func TestParseTemplateDirNameRejects(t *testing.T) {
	for _, dir := range []string{"", "4.5", "4.5.stable.custom", "not-a-version"} {
		if _, err := ParseTemplateDirName(dir); err == nil {
			t.Errorf("%q was accepted as a template directory", dir)
		}
	}
}

// writeTemplates puts the named files in a version's template directory, as
// Godot would when downloading them a platform at a time.
func writeTemplates(t *testing.T, manager *EngineManager, version EngineVersion, files ...string) {
	t.Helper()

	dir := manager.TemplatesPath(version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if err := os.WriteFile(filepath.Join(dir, file), []byte("template"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTemplatesForOnePlatform(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	version := mustParse(t, "4.7")

	writeTemplates(t, manager, version, "linux_debug.x86_64", "linux_release.x86_64")

	templates, err := manager.Templates(version)
	is.NoErr(err)
	is.Equal(templates.State(), TemplatesPartial)
	is.Equal(templates.Platforms(), []string{"linux"})
	is.Equal(templates.IncompletePlatforms(), []string{})
	is.True(!templates.FromArchive) // no version.txt, so it wasn't a .tpz

	for _, set := range templates.Sets {
		switch set.Name {
		case "Linux x86_64":
			is.True(set.Installed())
		default:
			is.True(set.Missing())
		}
	}
}

func TestTemplatesWithAPlatformHalfThere(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	version := mustParse(t, "4.7")

	writeTemplates(t, manager, version, "android_debug.apk", "android_release.apk")

	templates, err := manager.Templates(version)
	is.NoErr(err)
	is.Equal(templates.State(), TemplatesPartial)
	is.Equal(templates.IncompletePlatforms(), []string{"android"})
}

func TestTemplatesReportsFilesItDoesNotKnow(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	version := mustParse(t, "4.5.1")

	// 4.5.1 ships a visionos.zip that no version of the editor lists.
	writeTemplates(t, manager, version, "macos.zip", "visionos.zip", templatesVersionFile)

	templates, err := manager.Templates(version)
	is.NoErr(err)
	is.Equal(templates.Other, []string{"visionos.zip"})
	is.True(templates.FromArchive)
}

func TestTemplatesWhenThereAreNone(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})

	templates, err := manager.Templates(mustParse(t, "4.5"))
	is.NoErr(err)
	is.Equal(templates.State(), TemplatesNone)
	is.True(templates.Empty())
	is.Equal(templates.Platforms(), []string{})
}

func TestListTemplatesFindsVersionsWithNoEngine(t *testing.T) {
	is := is.New(t)

	manager := testManager(t, map[string][]byte{})
	writeTemplates(t, manager, mustParse(t, "4.7"), "linux_debug.x86_64")
	writeTemplates(t, manager, mustParse(t, "4.5-mono"), "macos.zip")

	// A directory with nothing Godot would export with isn't a template install.
	writeTemplates(t, manager, mustParse(t, "4.4"), "notes.txt")

	installed, err := manager.ListTemplates()
	is.NoErr(err)
	is.Equal(len(installed), 2)
	is.Equal(installed[0].Name, "4.5-stable-mono")
	is.Equal(installed[1].Name, "4.7-stable")
}

func TestInstallTemplatesOverPartialOnes(t *testing.T) {
	is := is.New(t)

	version := mustParse(t, "4.5")
	manager := testManager(t, map[string][]byte{
		TemplatesAssetName(version): zipBytes(t, map[string]string{
			"templates/version.txt":        "4.5.stable",
			"templates/linux_debug.x86_64": "template",
		}),
	})

	writeTemplates(t, manager, version, "macos.zip")

	err := manager.InstallTemplates(t.Context(), version, DownloadOptions{})
	is.True(err != nil) // templates are already there, so it won't just clobber them

	is.NoErr(manager.InstallTemplates(t.Context(), version, DownloadOptions{Replace: true}))

	templates, err := manager.Templates(version)
	is.NoErr(err)
	is.True(templates.FromArchive)
	is.Equal(templates.Platforms(), []string{"linux"}) // the old macos.zip went with them
}
