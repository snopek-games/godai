package godot

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const maxArchiveEntrySize = 4 << 30

// extractZip unpacks archivePath into destDir. Entries outside stripPrefix are
// skipped, and the prefix is dropped from the ones that remain.
func extractZip(archivePath, destDir, stripPrefix string) error {
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filepath.Base(archivePath), err)
	}
	defer archive.Close()

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}

	for _, file := range archive.File {
		name, ok := entryName(file.Name, stripPrefix)
		if !ok {
			continue
		}

		target, err := safeJoin(destDir, name)
		if err != nil {
			return fmt.Errorf("reading %s: %w", filepath.Base(archivePath), err)
		}

		if err := extractEntry(file, target, destDir); err != nil {
			return fmt.Errorf("unpacking %s: %w", file.Name, err)
		}
	}

	return nil
}

func entryName(rawName, stripPrefix string) (string, bool) {
	name := path.Clean(filepath.ToSlash(rawName))

	if stripPrefix != "" {
		rest, ok := strings.CutPrefix(name, stripPrefix)
		if !ok {
			return "", false
		}
		name = strings.TrimPrefix(rest, "/")
	}

	if name == "" || name == "." || name == "/" {
		return "", false
	}
	return name, true
}

func extractEntry(file *zip.File, target, destDir string) error {
	info := file.FileInfo()

	switch {
	case info.IsDir():
		return os.MkdirAll(target, dirMode(info.Mode()))
	case info.Mode()&os.ModeSymlink != 0:
		return extractSymlink(file, target, destDir)
	case !info.Mode().IsRegular():
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}

	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	dest, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fileMode(info.Mode()))
	if err != nil {
		return err
	}
	defer dest.Close()

	written, err := io.Copy(dest, io.LimitReader(source, maxArchiveEntrySize+1))
	if err != nil {
		return err
	}
	if written > maxArchiveEntrySize {
		return fmt.Errorf("larger than the %d byte limit", int64(maxArchiveEntrySize))
	}

	return dest.Close()
}

// The macOS builds ship as an app bundle, which uses symlinks inside its
// frameworks.
func extractSymlink(file *zip.File, target, destDir string) error {
	source, err := file.Open()
	if err != nil {
		return err
	}
	defer source.Close()

	raw, err := io.ReadAll(io.LimitReader(source, 4096))
	if err != nil {
		return err
	}
	linkTarget := string(raw)

	resolved := linkTarget
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(filepath.Dir(target), filepath.FromSlash(linkTarget))
	}
	if _, err := safeJoin(destDir, mustRel(destDir, resolved)); err != nil {
		return fmt.Errorf("link points outside the archive: %s", linkTarget)
	}

	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	os.Remove(target)
	return os.Symlink(linkTarget, target)
}

func mustRel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return ".."
	}
	return rel
}

func safeJoin(root, name string) (string, error) {
	target := filepath.Join(root, filepath.FromSlash(name))
	if target != root && !strings.HasPrefix(target, root+string(os.PathSeparator)) {
		return "", fmt.Errorf("entry %q would be written outside %s", name, root)
	}
	return target, nil
}

// Archives built on Windows carry no permissions at all.
func fileMode(mode os.FileMode) os.FileMode {
	if perm := mode.Perm(); perm != 0 {
		return perm
	}
	return 0o644
}

func dirMode(mode os.FileMode) os.FileMode {
	if perm := mode.Perm(); perm != 0 {
		return perm | 0o100
	}
	return 0o755
}
