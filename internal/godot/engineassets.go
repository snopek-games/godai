package godot

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// engineArchSuffixes maps a Go platform to the way godot-builds spells it. The
// two builds disagree about punctuation, so they're kept apart: the standard
// build uses "linux.x86_64", the .NET one "linux_x86_64".
var engineArchSuffixes = map[string]struct{ standard, mono string }{
	"linux/amd64":   {"linux.x86_64", "linux_x86_64"},
	"linux/arm64":   {"linux.arm64", "linux_arm64"},
	"linux/386":     {"linux.x86_32", "linux_x86_32"},
	"linux/arm":     {"linux.arm32", "linux_arm32"},
	"windows/amd64": {"win64.exe", "win64"},
	"windows/386":   {"win32.exe", "win32"},
	"windows/arm64": {"windows_arm64.exe", "windows_arm64"},
	"darwin/amd64":  {"macos.universal", "macos.universal"},
	"darwin/arm64":  {"macos.universal", "macos.universal"},
}

// EngineAssetName is the name of the release asset holding the editor for this
// version on the platform Godai is running on.
func EngineAssetName(version EngineVersion) (string, error) {
	return engineAssetName(version, runtime.GOOS, runtime.GOARCH)
}

func engineAssetName(version EngineVersion, goos, goarch string) (string, error) {
	suffixes, ok := engineArchSuffixes[goos+"/"+goarch]
	if !ok {
		return "", fmt.Errorf("Godot isn't built for %s/%s", goos, goarch)
	}

	if version.Mono {
		return fmt.Sprintf("Godot_v%s_mono_%s.zip", version.Tag(), suffixes.mono), nil
	}
	return fmt.Sprintf("Godot_v%s_%s.zip", version.Tag(), suffixes.standard), nil
}

// TemplatesAssetName is the name of the release asset holding the export
// templates, which are the same on every platform.
func TemplatesAssetName(version EngineVersion) string {
	if version.Mono {
		return fmt.Sprintf("Godot_v%s_mono_export_templates.tpz", version.Tag())
	}
	return fmt.Sprintf("Godot_v%s_export_templates.tpz", version.Tag())
}

// FindEngineExecutable looks for the Godot executable in an unpacked release.
// The layout differs between platforms and between the standard and .NET
// builds, and the executable is named after the version, so it's found by
// shape rather than by an expected path.
func FindEngineExecutable(root string) (string, error) {
	return findEngineExecutable(root, runtime.GOOS)
}

func findEngineExecutable(root, goos string) (string, error) {
	var candidates []string

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if isEngineExecutable(root, path, goos) {
			candidates = append(candidates, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("no Godot executable in %s", root)
	}

	// A release only ever holds one editor, but sorting keeps the choice from
	// depending on the order the filesystem hands entries back.
	slices.SortFunc(candidates, func(a, b string) int {
		if c := compareInt(depth(a), depth(b)); c != 0 {
			return c
		}
		return strings.Compare(a, b)
	})

	return candidates[0], nil
}

func isEngineExecutable(root, path, goos string) bool {
	name := filepath.Base(path)

	switch goos {
	case "windows":
		// The console executable is a launcher that re-runs the real one, so
		// picking it would leave a stray window open.
		return strings.HasSuffix(name, ".exe") && !strings.HasSuffix(name, "_console.exe")
	case "darwin":
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return false
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) < 4 {
			return false
		}
		return strings.HasSuffix(parts[len(parts)-4], ".app") &&
			parts[len(parts)-3] == "Contents" && parts[len(parts)-2] == "MacOS"
	default:
		// The Linux builds hold nothing but the executable and, for .NET, the
		// GodotSharp directory beside it.
		return strings.HasPrefix(name, "Godot_v")
	}
}

func depth(path string) int {
	return strings.Count(filepath.ToSlash(path), "/")
}
