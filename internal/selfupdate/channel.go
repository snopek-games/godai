package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Package-managed installs (brew, npm, the MCPB bundle) ship this file next to
// the binary - or one directory up, for layouts like Homebrew's Cellar - naming
// the channel that owns the executable. Self-update refuses to replace a
// binary that has one, because the package manager would clobber or orphan the
// replacement.
const channelFileName = "godai-install-channel"

type channelInfo struct {
	hint               string
	installCommand     string
	installInstruction string
}

var channels = map[string]channelInfo{
	"brew": {
		hint:               "godai was installed by Homebrew, which manages this executable; update it with 'brew upgrade godai' instead",
		installCommand:     "brew upgrade godai",
		installInstruction: "run 'brew upgrade godai' to install it",
	},
	"npm": {
		hint:               "godai was installed by npm, which manages this executable; update it with 'npm install -g @snopek-games/godai@latest' instead",
		installCommand:     "npm install -g @snopek-games/godai@latest",
		installInstruction: "run 'npm install -g @snopek-games/godai@latest' to install it",
	},
	"mcpb": {
		hint:               "godai is part of an MCPB bundle, which manages this executable; install the newest godai-mcp bundle to update it",
		installInstruction: "install the newest godai-mcp bundle to get it",
	},
}

// InstallChannel reports which package manager installed exePath, or "" for a
// standalone (self-updatable) install. exePath should already have symlinks
// resolved (see ExecutablePath), so the channel file is found in the install
// location rather than next to a symlink.
func InstallChannel(exePath string) string {
	dir := filepath.Dir(exePath)
	for _, d := range []string{dir, filepath.Dir(dir)} {
		data, err := os.ReadFile(filepath.Join(d, channelFileName))
		if err != nil {
			continue
		}
		if name := strings.TrimSpace(string(data)); name != "" {
			return name
		}
	}

	// npm packages published before the channel file existed.
	if slices.Contains(strings.Split(filepath.ToSlash(exePath), "/"), "node_modules") {
		return "npm"
	}

	return ""
}

// PackageManagerHint explains why a package-managed executable shouldn't be
// replaced in place and what to do instead, or returns "" for a standalone
// install.
func PackageManagerHint(exePath string) string {
	channel := InstallChannel(exePath)
	if channel == "" {
		return ""
	}
	if info, ok := channels[channel]; ok {
		return info.hint
	}
	return fmt.Sprintf("godai was installed by a package manager (%q), which manages this executable; update it through that instead", channel)
}

// InstallInstruction says how to install a newer release of exePath.
func InstallInstruction(exePath string) string {
	channel := InstallChannel(exePath)
	if channel == "" {
		return fmt.Sprintf("run '%s self-update' to install it", exePath)
	}
	if info, ok := channels[channel]; ok {
		return info.installInstruction
	}
	return fmt.Sprintf("update it through the package manager that installed it (%q)", channel)
}

// InstallCommandForRuntime returns the command that updates the running
// executable, or "" when its install channel has no such command.
func InstallCommandForRuntime() string {
	exePath, err := ExecutablePath()
	if err != nil {
		return "godai self-update"
	}
	channel := InstallChannel(exePath)
	if channel == "" {
		return "godai self-update"
	}
	return channels[channel].installCommand
}
