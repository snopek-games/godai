package godot

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type TemplateSet struct {
	Name     string
	Platform string
	Files    []string
}

// exportTemplateSets mirrors the table in Godot's own export template manager
// (editor/export/export_template_manager.cpp).
var exportTemplateSets = []TemplateSet{
	{"Windows x86_32", "windows", []string{
		"windows_debug_x86_32.exe", "windows_debug_x86_32_console.exe",
		"windows_release_x86_32.exe", "windows_release_x86_32_console.exe",
	}},
	{"Windows x86_64", "windows", []string{
		"windows_debug_x86_64.exe", "windows_debug_x86_64_console.exe",
		"windows_release_x86_64.exe", "windows_release_x86_64_console.exe",
	}},
	{"Windows arm64", "windows", []string{
		"windows_debug_arm64.exe", "windows_debug_arm64_console.exe",
		"windows_release_arm64.exe", "windows_release_arm64_console.exe",
	}},
	{"Linux x86_32", "linux", []string{"linux_debug.x86_32", "linux_release.x86_32"}},
	{"Linux x86_64", "linux", []string{"linux_debug.x86_64", "linux_release.x86_64"}},
	{"Linux arm32", "linux", []string{"linux_debug.arm32", "linux_release.arm32"}},
	{"Linux arm64", "linux", []string{"linux_debug.arm64", "linux_release.arm64"}},
	{"macOS", "macos", []string{"macos.zip"}},
	{"Android", "android", []string{"android_debug.apk", "android_release.apk", "android_source.zip"}},
	{"iOS", "ios", []string{"ios.zip"}},
	{"Web", "web", []string{"web_debug.zip", "web_release.zip"}},
	{"Web with Extensions", "web", []string{"web_dlink_debug.zip", "web_dlink_release.zip"}},
	{"Web Single-Threaded", "web", []string{"web_nothreads_debug.zip", "web_nothreads_release.zip"}},
	{"Web with Extensions Single-Threaded", "web", []string{
		"web_dlink_nothreads_debug.zip", "web_dlink_nothreads_release.zip",
	}},
	{"ICU Data", "common", []string{"icudt_godot.dat"}},
}

// Written by a full .tpz install, but not by Godot's per-platform downloads.
const templatesVersionFile = "version.txt"

type TemplateSetStatus struct {
	Name     string   `json:"name"`
	Platform string   `json:"platform"`
	Present  []string `json:"present"`
	Total    int      `json:"total"`
}

func (s TemplateSetStatus) Installed() bool {
	return len(s.Present) == s.Total
}

func (s TemplateSetStatus) Missing() bool {
	return len(s.Present) == 0
}

type TemplateState string

const (
	TemplatesNone    TemplateState = "none"
	TemplatesPartial TemplateState = "partial"
	TemplatesAll     TemplateState = "all"
)

type InstalledTemplates struct {
	// Name is the version, or the raw directory name when that isn't one.
	Name    string              `json:"version"`
	Version EngineVersion       `json:"-"`
	Path    string              `json:"path"`
	Sets    []TemplateSetStatus `json:"templates"`
	// Other holds files Godai has no entry for, which a newer Godot may still
	// export with.
	Other []string `json:"other,omitempty"`
	// FromArchive reports whether a full .tpz was unpacked here, rather than
	// files being downloaded one platform at a time.
	FromArchive bool `json:"from_archive"`
}

func (t *InstalledTemplates) State() TemplateState {
	switch {
	case t.Empty():
		return TemplatesNone
	case t.Complete():
		return TemplatesAll
	default:
		return TemplatesPartial
	}
}

func (t *InstalledTemplates) Complete() bool {
	for _, set := range t.Sets {
		if !set.Installed() {
			return false
		}
	}
	return true
}

func (t *InstalledTemplates) Empty() bool {
	for _, set := range t.Sets {
		if !set.Missing() {
			return false
		}
	}
	return true
}

// Platforms lists the platforms with any templates at all, in the order Godot
// shows them.
func (t *InstalledTemplates) Platforms() []string {
	platforms := []string{}
	for _, set := range t.Sets {
		if !set.Missing() && !slices.Contains(platforms, set.Platform) {
			platforms = append(platforms, set.Platform)
		}
	}
	return platforms
}

// IncompletePlatforms lists the platforms that have some, but not all, of the
// files for a template they've started on.
func (t *InstalledTemplates) IncompletePlatforms() []string {
	platforms := []string{}
	for _, set := range t.Sets {
		if !set.Missing() && !set.Installed() && !slices.Contains(platforms, set.Platform) {
			platforms = append(platforms, set.Platform)
		}
	}
	return platforms
}

// ParseTemplateDirName is the reverse of EngineVersion.TemplateDirName.
func ParseTemplateDirName(name string) (EngineVersion, error) {
	parts := strings.Split(name, ".")

	mono := false
	if len(parts) > 0 && parts[len(parts)-1] == "mono" {
		mono = true
		parts = parts[:len(parts)-1]
	}

	if len(parts) < 3 {
		return EngineVersion{}, &VersionError{Input: name, Reason: "expected a directory like 4.5.stable"}
	}

	version := strings.Join(parts[:len(parts)-1], ".") + "-" + parts[len(parts)-1]
	if mono {
		version += "-mono"
	}

	return ParseEngineVersion(version)
}

func (m *EngineManager) Templates(version EngineVersion) (*InstalledTemplates, error) {
	templates, err := readTemplates(m.TemplatesPath(version))
	if err != nil {
		return nil, err
	}

	templates.Name = version.String()
	templates.Version = version
	return templates, nil
}

// ListTemplates reports every version with export templates installed, whether
// or not the matching engine is.
func (m *EngineManager) ListTemplates() ([]*InstalledTemplates, error) {
	entries, err := os.ReadDir(m.templatesDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	installed := []*InstalledTemplates{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		templates, err := readTemplates(filepath.Join(m.templatesDir, entry.Name()))
		if err != nil {
			slog.Debug("ignoring an unreadable export template directory", "name", entry.Name(), "error", err)
			continue
		}
		if templates.Empty() {
			continue
		}

		templates.Name = entry.Name()
		if version, err := ParseTemplateDirName(entry.Name()); err == nil {
			templates.Name = version.String()
			templates.Version = version
		}

		installed = append(installed, templates)
	}

	slices.SortFunc(installed, func(a, b *InstalledTemplates) int {
		if c := a.Version.Compare(b.Version); c != 0 {
			return c
		}
		return strings.Compare(a.Name, b.Name)
	})

	return installed, nil
}

func readTemplates(dir string) (*InstalledTemplates, error) {
	present := map[string]bool{}

	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			present[entry.Name()] = true
		}
	}

	templates := &InstalledTemplates{
		Path:        dir,
		FromArchive: present[templatesVersionFile],
	}

	known := map[string]bool{templatesVersionFile: true}
	for _, set := range exportTemplateSets {
		status := TemplateSetStatus{Name: set.Name, Platform: set.Platform, Total: len(set.Files), Present: []string{}}
		for _, file := range set.Files {
			known[file] = true
			if present[file] {
				status.Present = append(status.Present, file)
			}
		}
		templates.Sets = append(templates.Sets, status)
	}

	for file := range present {
		if !known[file] {
			templates.Other = append(templates.Other, file)
		}
	}
	slices.Sort(templates.Other)

	return templates, nil
}
