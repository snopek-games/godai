package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/matryer/is"
)

func writeInstanceFile(t *testing.T, dir, instanceID, projectPath string, port int) {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"instance_id":  instanceID,
		"pid":          os.Getpid(),
		"project_path": projectPath,
		"secret":       "secret-" + instanceID,
		"port":         port,
	})
	if err != nil {
		t.Fatalf("marshalling instance: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, instanceID+".json"), b, 0o644); err != nil {
		t.Fatalf("writing instance file: %v", err)
	}
}

func TestRunningEditorIsFoundBeforeItConnects(t *testing.T) {
	is := is.New(t)

	instancesDir := t.TempDir()
	projectPath := t.TempDir()

	// Port 1 has nothing listening on it, so no connection is ever registered.
	writeInstanceFile(t, instancesDir, "advertised", projectPath, 1)

	s, err := New(Config{
		Scope:               ScopeGlobal,
		EditorInstancesPath: instancesDir,
		EditorRetryDelay:    time.Second,
	})
	is.NoErr(err)

	is.True(!s.hasRunningEditorForProject(projectPath)) // nothing is scanning yet

	is.NoErr(s.Start(context.Background()))
	defer s.Close()

	is.True(!s.hasEditorForProject(projectPath))       // no connection has been made
	is.True(s.hasRunningEditorForProject(projectPath)) // but the editor is running

	other := t.TempDir()
	is.True(!s.hasRunningEditorForProject(other))
}

func TestOpenProjectTimeoutIsReportedAsATimeout(t *testing.T) {
	is := is.New(t)

	instancesDir := t.TempDir()
	projectPath := t.TempDir()

	writeInstanceFile(t, instancesDir, "advertised", projectPath, 1)

	s, err := New(Config{
		Scope:               ScopeGlobal,
		EditorInstancesPath: instancesDir,
		EditorRetryDelay:    time.Second,
	})
	is.NoErr(err)

	is.NoErr(s.Start(context.Background()))
	defer s.Close()

	_, err = s.OpenProject(context.Background(), projectPath, OpenProjectOptions{Wait: 50 * time.Millisecond})
	is.True(errors.Is(err, context.DeadlineExceeded))
}
