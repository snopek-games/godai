package selfupdate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func writeChannelFile(t *testing.T, dir, channel string) {
	t.Helper()
	is.New(t).NoErr(os.WriteFile(filepath.Join(dir, channelFileName), []byte(channel+"\n"), 0o644))
}

func TestInstallChannelNextToExecutable(t *testing.T) {
	is := is.New(t)

	dir := t.TempDir()
	writeChannelFile(t, dir, "npm")

	is.Equal(InstallChannel(filepath.Join(dir, "godai")), "npm")
}

func TestInstallChannelInParentDir(t *testing.T) {
	is := is.New(t)

	// Homebrew's layout: the binary in <prefix>/bin, the file at <prefix>.
	prefix := t.TempDir()
	binDir := filepath.Join(prefix, "bin")
	is.NoErr(os.Mkdir(binDir, 0o755))
	writeChannelFile(t, prefix, "brew")

	is.Equal(InstallChannel(filepath.Join(binDir, "godai")), "brew")
}

func TestInstallChannelStandalone(t *testing.T) {
	is := is.New(t)

	is.Equal(InstallChannel(filepath.Join(t.TempDir(), "godai")), "")
}

func TestInstallChannelNodeModulesFallback(t *testing.T) {
	is := is.New(t)

	npmPath := filepath.Join("/usr", "lib", "node_modules", "@snopek-games",
		"godai-linux-x64", "bin", "godai")
	is.Equal(InstallChannel(npmPath), "npm")

	is.Equal(InstallChannel(filepath.Join("/opt", "node_modules_backup", "godai")), "") // only a real node_modules path component counts
}

func TestPackageManagerHint(t *testing.T) {
	is := is.New(t)

	brewPrefix := t.TempDir()
	brewBin := filepath.Join(brewPrefix, "bin")
	is.NoErr(os.Mkdir(brewBin, 0o755))
	writeChannelFile(t, brewPrefix, "brew")
	is.True(strings.Contains(PackageManagerHint(filepath.Join(brewBin, "godai")), "brew upgrade godai"))

	npmPath := filepath.Join("/usr", "lib", "node_modules", "@snopek-games",
		"godai-linux-x64", "bin", "godai")
	is.True(strings.Contains(PackageManagerHint(npmPath), "npm install"))

	mcpbDir := t.TempDir()
	writeChannelFile(t, mcpbDir, "mcpb")
	is.True(strings.Contains(PackageManagerHint(filepath.Join(mcpbDir, "godai")), "bundle"))

	unknownDir := t.TempDir()
	writeChannelFile(t, unknownDir, "scoop")
	is.True(strings.Contains(PackageManagerHint(filepath.Join(unknownDir, "godai")), "scoop"))

	is.Equal(PackageManagerHint(filepath.Join("/usr", "local", "bin", "godai")), "") // no channel file, no hint
}

func TestInstallInstruction(t *testing.T) {
	is := is.New(t)

	exePath := filepath.Join(t.TempDir(), "godai")
	is.True(strings.Contains(InstallInstruction(exePath), "'"+exePath+" self-update'"))

	brewPrefix := t.TempDir()
	brewBin := filepath.Join(brewPrefix, "bin")
	is.NoErr(os.Mkdir(brewBin, 0o755))
	writeChannelFile(t, brewPrefix, "brew")
	is.True(strings.Contains(InstallInstruction(filepath.Join(brewBin, "godai")), "brew upgrade godai"))

	mcpbDir := t.TempDir()
	writeChannelFile(t, mcpbDir, "mcpb")
	is.True(strings.Contains(InstallInstruction(filepath.Join(mcpbDir, "godai")), "bundle"))
}
