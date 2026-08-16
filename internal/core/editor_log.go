package core

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
)

const editorLogEnv = "GODAI_EDITOR_LOG"

// editorLogFile returns nil with no error when GODAI_EDITOR_LOG is unset.
func editorLogFile(projectPath string) (*os.File, string, error) {
	if os.Getenv(editorLogEnv) == "" {
		return nil, "", nil
	}

	cachePath, err := GetCachePath()
	if err != nil {
		return nil, "", err
	}
	dir := filepath.Join(cachePath, "editor-logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}

	h := fnv.New32a()
	h.Write([]byte(projectPath))
	path := filepath.Join(dir, fmt.Sprintf("%s-%08x.log", filepath.Base(projectPath), h.Sum32()))

	f, err := os.Create(path)
	if err != nil {
		return nil, "", err
	}
	return f, path, nil
}

const editorLogTailBytes = 2000

func editorLogTail(path string) string {
	if path == "" {
		return ""
	}
	blob, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	blob = bytes.TrimSpace(blob)
	if len(blob) == 0 {
		return ""
	}
	if len(blob) > editorLogTailBytes {
		blob = blob[len(blob)-editorLogTailBytes:]
		if i := bytes.IndexByte(blob, '\n'); i >= 0 {
			blob = blob[i+1:]
		}
	}
	return fmt.Sprintf("\nEditor output (%s):\n%s", path, blob)
}
