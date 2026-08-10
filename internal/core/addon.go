package core

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gitlab.com/snopek-games/godai"
	"gitlab.com/snopek-games/godai/internal/godot"
	"gitlab.com/snopek-games/godai/internal/godot/variant"
)

var Version string = mustGetVersion()

func mustGetVersion() string {
	v, err := PluginVersion(godai.AddonFS)
	if err != nil {
		panic(fmt.Errorf("unable to read Godai version from embedded plugin.cfg: %w", err))
	}
	return v
}

func PluginVersion(fsys fs.FS) (string, error) {
	f, err := fsys.Open("addons/godai/plugin.cfg")
	if err != nil {
		return "", err
	}
	defer f.Close()

	r := io.Reader(f)
	cf, err := godot.ReadConfigFile(r)
	if err != nil {
		return "", err
	}

	v, ok := cf.GetString("plugin", "version")
	if !ok {
		return "", errors.New("plugin version not found")
	}

	return v, nil
}

func installAddon(project *godot.Project, forceReplace bool) error {
	projectPath := project.GetPath()
	addonRelPath := filepath.Join("addons", "godai")
	installedPath := filepath.Join(projectPath, addonRelPath)

	exists, err := dirExists(installedPath)
	if err != nil {
		return err
	}

	if exists {
		shouldReplace := false
		if forceReplace {
			shouldReplace = true
			slog.Info("debug enabled; forcing replacement of Godai addon", "projectPath", projectPath)
		} else {
			myVersion, err := PluginVersion(godai.AddonFS)
			if err != nil {
				return err
			}
			installedVersion, err := PluginVersion(os.DirFS(projectPath))
			if err != nil {
				// We consider an error reading the installed version to be a reason to replace it.
				// So, we don't return the error here, just make sure the version won't match.
				installedVersion = "error"
			}
			shouldReplace = (myVersion != installedVersion)
			if shouldReplace {
				slog.Info("installed Godai addon version doesn't match embedded",
					"projectPath", projectPath,
					"installedVersion", installedVersion,
					"embeddedVersion", myVersion,
				)
			}
		}

		if !shouldReplace {
			return nil
		}

		err = os.RemoveAll(installedPath)
		if err != nil {
			return err
		}
	}

	slog.Info("installing Godai addon", "projectPath", projectPath)

	return fs.WalkDir(godai.AddonFS, addonRelPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(addonRelPath, p)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(installedPath, rel)
		if d.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}

		r, err := godai.AddonFS.Open(p)
		if err != nil {
			return err
		}
		defer r.Close()

		w, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		defer w.Close()

		if _, err := io.Copy(w, r); err != nil {
			return err
		}

		return nil
	})
}

// Attempt a safe edit of the project file.
func enableAddon(project *godot.Project) error {
	configPath := filepath.Join(project.GetPath(), "project.godot")

	f, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(f)))

	section := ""
	found := false
	out := strings.Builder{}

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 0 {
			if line[0] == '[' && line[len(line)-1] == ']' {
				section = line[1 : len(line)-1]
			} else if section == "editor_plugins" && strings.HasPrefix(line, "enabled=PackedStringArray(") {
				parser := variant.NewParser(strings.NewReader(line))
				stmt, err := parser.ParseStatement()
				if err != nil {
					return err
				}

				values := stmt.Value.(variant.PackedStringArray)
				for _, v := range values {
					if v == "res://addons/godai/plugin.cfg" {
						// We already have the plugin, so we're good.
						return nil
					}
				}
				values = append(values, "res://addons/godai/plugin.cfg")

				buf := &strings.Builder{}
				writer := variant.NewWriter(buf)
				if err := writer.WriteAssignment("enabled", values); err != nil {
					return err
				}
				writer.Flush()
				line = buf.String()

				if len(line) > 0 && line[len(line)-1] == '\n' {
					line = line[:len(line)-1]
				}

				found = true
			}
		}
		out.WriteString(line)
		out.WriteString("\n")
	}
	if err := scanner.Err(); err != nil {
		return err
	}

	if !found {
		out.WriteString("[editor_plugins]\n\n")
		out.WriteString("enabled=PackedStringArray(\"res://addons/godai/plugin.cfg\")\n\n")
	}

	if err := os.WriteFile(configPath, []byte(out.String()), 0o755); err != nil {
		return err
	}

	return nil
}
