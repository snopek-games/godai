package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/matryer/is"
)

// If releaseVariants and the CI build matrix drift apart, `godai self-update`
// either says there's no build for a platform when there is one, or looks for
// a file that was never published - and neither shows up until after a release.
func TestReleaseVariantsMatchCIMatrix(t *testing.T) {
	is := is.New(t)

	data, err := os.ReadFile(filepath.Join("..", "..", ".gitlab-ci.yml"))
	is.NoErr(err)

	fromCI := parseCIBuildMatrix(t, string(data))
	is.True(len(fromCI) > 0)

	for platform, want := range fromCI {
		got, ok := releaseVariants[platform]
		if !ok {
			t.Errorf("CI builds %s but releaseVariants has no entry for it", platform)
			continue
		}
		is.Equal(got, want)
	}

	for platform := range releaseVariants {
		if _, ok := fromCI[platform]; !ok {
			t.Errorf("releaseVariants has an entry for %s but CI doesn't build it", platform)
		}
	}
}

// parseCIBuildMatrix reads the GOOS/GOARCH -> variant mapping out of the
// cli-build job's parallel matrix, which isn't worth a YAML dependency.
func parseCIBuildMatrix(t *testing.T, yaml string) map[string]Variant {
	t.Helper()

	var (
		entryStart = regexp.MustCompile(`^(\s*)-\s+(\w+):\s*"([^"]*)"\s*$`)
		entryField = regexp.MustCompile(`^\s*(\w+):\s*"([^"]*)"\s*$`)
	)

	variants := map[string]Variant{}
	entries := []map[string]string{}
	var current map[string]string
	inMatrix := false
	matrixIndent := 0

	for line := range strings.SplitSeq(yaml, "\n") {
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " "))

		if trimmed == "matrix:" {
			inMatrix = true
			matrixIndent = indent
			continue
		}
		if !inMatrix {
			continue
		}
		if trimmed != "" && indent <= matrixIndent {
			inMatrix = false
			current = nil
			continue
		}

		if match := entryStart.FindStringSubmatch(line); match != nil {
			current = map[string]string{match[2]: match[3]}
			entries = append(entries, current)
			continue
		}
		if match := entryField.FindStringSubmatch(line); match != nil && current != nil {
			current[match[1]] = match[2]
		}
	}

	for _, entry := range entries {
		goos, goarch, variant := entry["GOOS"], entry["GOARCH"], entry["VARIANT"]
		if goos == "" || goarch == "" || variant == "" {
			continue
		}
		variants[goos+"/"+goarch] = Variant{Name: variant, Ext: entry["EXT"]}
	}

	return variants
}

func TestVariantForPlatform(t *testing.T) {
	is := is.New(t)

	variant, err := variantForPlatform("windows", "amd64")
	is.NoErr(err)
	is.Equal(variant, Variant{Name: "windows-x86_64", Ext: ".exe"})

	_, err = variantForPlatform("darwin", "amd64")
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "darwin/amd64"))
}

func TestVariantByName(t *testing.T) {
	is := is.New(t)

	variant, err := variantByName("macos-arm64")
	is.NoErr(err)
	is.Equal(variant, Variant{Name: "macos-arm64", Ext: ""})

	_, err = variantByName("plan9-arm64")
	is.True(err != nil)
}

// The updater asks for release assets by name, so the names it builds have to
// be the ones the release job publishes.
func TestReleaseNamesMatchWhatCIPublishes(t *testing.T) {
	is := is.New(t)

	data, err := os.ReadFile(filepath.Join("..", "..", ".gitlab-ci.yml"))
	is.NoErr(err)

	assetPrefix := regexp.MustCompile(`CLI_ASSET_NAME:\s*"([^"]*)"`).FindStringSubmatch(string(data))
	is.True(assetPrefix != nil)
	is.Equal(assetPrefix[1], DefaultAssetPrefix)

	binaryName := regexp.MustCompile(`CLI_BIN_NAME:\s*"([^"]*)"`).FindStringSubmatch(string(data))
	is.True(binaryName != nil)
	is.Equal(binaryName[1], DefaultBinaryName)

	const tag = "v0.4.0"
	ci := strings.NewReplacer(
		"${CLI_ASSET_NAME}", assetPrefix[1],
		"${CLI_BIN_NAME}", binaryName[1],
		"${CI_COMMIT_TAG}", tag,
	).Replace(string(data))

	updater, err := New(Config{CurrentVersion: "0.3.2", Variant: "windows-x86_64"})
	is.NoErr(err)

	assetName := fmt.Sprintf("%s-%s-%s.zip", updater.assetPrefix, updater.variant.Name, tag)
	is.Equal(assetName, "godai-cli-windows-x86_64-v0.4.0.zip")
	is.True(strings.Contains(ci, assetName))
	is.True(strings.Contains(ci, ChecksumsName(tag)))

	is.Equal(updater.binaryName+updater.variant.Ext, "godai.exe")
	is.True(strings.Contains(ci, "/"+binaryName[1]+"${EXT}"))
}
