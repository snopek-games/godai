package godot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

func TestProjectGetEngine(t *testing.T) {
	for _, tc := range []struct {
		name     string
		features string
		want     string
		named    bool
	}{
		{"version only", `PackedStringArray("4.5")`, "4.5", true},
		{"version and renderer", `PackedStringArray("4.7", "GL Compatibility")`, "4.7", true},
		{"c sharp", `PackedStringArray("4.7", "C#", "Forward Plus")`, "4.7 (C#)", true},
		{"renderer first", `PackedStringArray("Forward Plus", "4.6")`, "4.6", true},
		{"no version", `PackedStringArray("Forward Plus")`, "", false},
		{"empty", `PackedStringArray()`, "", false},
		{"", "", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			dir := t.TempDir()
			contents := "config_version=5\n\n[application]\n\nconfig/name=\"Test\"\n"
			if tc.features != "" {
				contents += "config/features=" + tc.features + "\n"
			}
			is.NoErr(os.WriteFile(filepath.Join(dir, "project.godot"), []byte(contents), 0o644))

			project, err := ProjectFromPath(dir)
			is.NoErr(err)

			engine, named, err := project.GetEngine()
			is.NoErr(err)
			is.Equal(named, tc.named)
			if named {
				is.Equal(engine.String(), tc.want)
			}
		})
	}
}

func TestProjectGetEngineFindsACsprojWithoutTheFeature(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	is.NoErr(os.WriteFile(filepath.Join(dir, "project.godot"),
		[]byte("config_version=5\n\n[application]\n\nconfig/features=PackedStringArray(\"4.7\")\n"), 0o644))
	is.NoErr(os.WriteFile(filepath.Join(dir, "Game.csproj"), []byte("<Project/>"), 0o644))

	project, err := ProjectFromPath(dir)
	is.NoErr(err)

	engine, named, err := project.GetEngine()
	is.NoErr(err)
	is.True(named)
	is.True(engine.Mono)
}
