package godot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matryer/is"
)

func TestEngineAssetName(t *testing.T) {
	for _, tc := range []struct {
		version string
		goos    string
		goarch  string
		want    string
	}{
		{"4.5", "linux", "amd64", "Godot_v4.5-stable_linux.x86_64.zip"},
		{"4.5", "linux", "arm64", "Godot_v4.5-stable_linux.arm64.zip"},
		{"4.5", "windows", "amd64", "Godot_v4.5-stable_win64.exe.zip"},
		{"4.5", "windows", "arm64", "Godot_v4.5-stable_windows_arm64.exe.zip"},
		{"4.5", "darwin", "arm64", "Godot_v4.5-stable_macos.universal.zip"},
		{"4.5-mono", "linux", "amd64", "Godot_v4.5-stable_mono_linux_x86_64.zip"},
		{"4.5-mono", "windows", "amd64", "Godot_v4.5-stable_mono_win64.zip"},
		{"4.5-mono", "darwin", "amd64", "Godot_v4.5-stable_mono_macos.universal.zip"},
		{"4.6-beta3", "linux", "amd64", "Godot_v4.6-beta3_linux.x86_64.zip"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			is := is.New(t)

			version, err := ParseEngineVersion(tc.version)
			is.NoErr(err)

			name, err := engineAssetName(version, tc.goos, tc.goarch)
			is.NoErr(err)
			is.Equal(name, tc.want)
		})
	}
}

func TestEngineAssetNameUnsupportedPlatform(t *testing.T) {
	version, err := ParseEngineVersion("4.5")
	is.New(t).NoErr(err)

	if _, err := engineAssetName(version, "plan9", "amd64"); err == nil {
		t.Error("plan9 was accepted as a platform Godot is built for")
	}
}

func TestTemplatesAssetName(t *testing.T) {
	is := is.New(t)

	standard, err := ParseEngineVersion("4.5")
	is.NoErr(err)
	is.Equal(TemplatesAssetName(standard), "Godot_v4.5-stable_export_templates.tpz")

	mono, err := ParseEngineVersion("4.5-mono")
	is.NoErr(err)
	is.Equal(TemplatesAssetName(mono), "Godot_v4.5-stable_mono_export_templates.tpz")
}

func TestFindEngineExecutable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		goos  string
		files []string
		want  string
	}{
		{
			name:  "linux",
			goos:  "linux",
			files: []string{"Godot_v4.5-stable_linux.x86_64"},
			want:  "Godot_v4.5-stable_linux.x86_64",
		},
		{
			name: "linux mono",
			goos: "linux",
			files: []string{
				"Godot_v4.5-stable_mono_linux_x86_64/Godot_v4.5-stable_mono_linux.x86_64",
				"Godot_v4.5-stable_mono_linux_x86_64/GodotSharp/Api/Debug/GodotSharp.dll",
			},
			want: "Godot_v4.5-stable_mono_linux_x86_64/Godot_v4.5-stable_mono_linux.x86_64",
		},
		{
			name: "windows",
			goos: "windows",
			files: []string{
				"Godot_v4.5-stable_win64.exe",
				"Godot_v4.5-stable_win64_console.exe",
			},
			want: "Godot_v4.5-stable_win64.exe",
		},
		{
			name: "macos",
			goos: "darwin",
			files: []string{
				"Godot.app/Contents/MacOS/Godot",
				"Godot.app/Contents/Info.plist",
			},
			want: "Godot.app/Contents/MacOS/Godot",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			is := is.New(t)

			root := t.TempDir()
			for _, file := range tc.files {
				path := filepath.Join(root, filepath.FromSlash(file))
				is.NoErr(os.MkdirAll(filepath.Dir(path), 0o755))
				is.NoErr(os.WriteFile(path, []byte("x"), 0o755))
			}

			found, err := findEngineExecutable(root, tc.goos)
			is.NoErr(err)
			is.Equal(found, filepath.Join(root, filepath.FromSlash(tc.want)))
		})
	}
}

func TestFindEngineExecutableWhenThereIsNone(t *testing.T) {
	if _, err := findEngineExecutable(t.TempDir(), "linux"); err == nil {
		t.Error("an empty directory was accepted as an unpacked engine")
	}
}
