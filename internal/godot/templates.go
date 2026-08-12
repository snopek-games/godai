package godot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const templatesArchivePrefix = "templates"

var (
	ErrTemplatesNotInstalled = errors.New("the export templates for that version aren't installed")
	ErrTemplatesInstalled    = errors.New("the export templates for that version are already installed")
)

func GetExportTemplatesPath() (string, error) {
	dataPath, err := GetEditorDataPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataPath, "export_templates"), nil
}

func (m *EngineManager) TemplatesDir() string {
	return m.templatesDir
}

func (m *EngineManager) TemplatesPath(version EngineVersion) string {
	return filepath.Join(m.templatesDir, version.TemplateDirName())
}

func (m *EngineManager) TemplatesInstalled(version EngineVersion) bool {
	entries, err := os.ReadDir(m.TemplatesPath(version))
	return err == nil && len(entries) > 0
}

func (m *EngineManager) InstallTemplates(ctx context.Context, version EngineVersion, opts DownloadOptions) error {
	installDir := m.TemplatesPath(version)
	if m.TemplatesInstalled(version) && !opts.Replace {
		return fmt.Errorf("%w: %s", ErrTemplatesInstalled, version)
	}

	unpackDir, cleanup, err := m.downloadAndUnpack(ctx, version,
		TemplatesAssetName(version), templatesArchivePrefix, m.templatesDir, opts)
	if err != nil {
		return err
	}
	defer cleanup()

	// Whatever was there only goes once the new templates are unpacked and
	// ready to be moved into their place.
	if err := os.RemoveAll(installDir); err != nil {
		return err
	}

	if err := os.Rename(unpackDir, installDir); err != nil {
		return fmt.Errorf("installing the export templates for %s: %w", version, err)
	}

	return nil
}

func (m *EngineManager) RemoveTemplates(version EngineVersion) error {
	if !m.TemplatesInstalled(version) {
		return fmt.Errorf("%w: %s", ErrTemplatesNotInstalled, version)
	}
	return os.RemoveAll(m.TemplatesPath(version))
}
