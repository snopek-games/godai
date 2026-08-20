package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"gitlab.com/snopek-games/godai/internal/cli/schemaflag"
	"gitlab.com/snopek-games/godai/internal/core"
)

func checkResPaths(names []string, specs []schemaflag.Spec) ([]schemaflag.Spec, error) {
	resPaths := make([]schemaflag.Spec, 0, len(names))
	for _, name := range names {
		spec, found := findSpec(specs, name)
		if !found {
			return nil, fmt.Errorf("cliResPathArguments names %q, which is not in the input schema", name)
		}
		if spec.Kind != schemaflag.KindString && spec.Kind != schemaflag.KindStringList {
			return nil, fmt.Errorf("cliResPathArguments names %q, which is not a string or list of strings", name)
		}
		resPaths = append(resPaths, spec)
	}
	return resPaths, nil
}

func normalizeResPathArgs(args core.Args, resPaths []schemaflag.Spec, projectPath string) error {
	for _, spec := range resPaths {
		raw, ok := args[spec.Property]
		if !ok {
			continue
		}

		// A value of the wrong shape is left for the editor's own type check,
		// where the message names the tool's argument.
		if spec.Kind == schemaflag.KindStringList {
			var values []string
			if err := json.Unmarshal(raw, &values); err != nil {
				continue
			}
			for i, value := range values {
				normalized, err := resPath(value, projectPath, spec.Flag)
				if err != nil {
					return err
				}
				values[i] = normalized
			}
			if err := args.Set(spec.Property, values); err != nil {
				return err
			}
			continue
		}

		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		normalized, err := resPath(value, projectPath, spec.Flag)
		if err != nil {
			return err
		}
		if err := args.Set(spec.Property, normalized); err != nil {
			return err
		}
	}
	return nil
}

func resPath(value, projectPath, flag string) (string, error) {
	if value == "" || strings.Contains(value, "://") {
		return value, nil
	}

	abs, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	abs = resolveExistingSymlinks(abs)

	root := projectPath
	if real, err := filepath.EvalSymlinks(projectPath); err == nil {
		root = real
	}

	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", newUsageError("--%s: %s is outside the project (%s); pass a path inside the project, or a res:// path", flag, value, projectPath)
	}
	if rel == "." {
		return "res://", nil
	}
	return "res://" + filepath.ToSlash(rel), nil
}

// Resolves symlinks in as much of the path as exists, so a file that is yet to
// be created still compares against the project's real location.
func resolveExistingSymlinks(abs string) string {
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}

	dir := filepath.Dir(abs)
	if dir == abs {
		return abs
	}
	return filepath.Join(resolveExistingSymlinks(dir), filepath.Base(abs))
}
