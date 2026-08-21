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
	is.True(!s.hasRunningEditorForProject(other)) // nothing advertised for this project
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

func TestAnEditorThatIsAlreadyOpenHasToBeTheVersionAskedFor(t *testing.T) {
	is := is.New(t)

	session := engineSessionFor(t, "4.4-stable", true)
	installEngineFor(t, "4.4-stable")
	installEngineFor(t, "4.5-stable")

	editor := &Editor{ProjectPath: "/games/platformer", GodotVersion: "4.5-stable"}

	err := session.checkEditorVersion(editor, OpenProjectOptions{})
	is.True(errors.Is(err, ErrEditorVersionMismatch)) // the version given for the run

	err = session.checkEditorVersion(editor, OpenProjectOptions{GodotVersion: "4.4"})
	is.True(errors.Is(err, ErrEditorVersionMismatch)) // the version given for the call

	is.NoErr(session.checkEditorVersion(editor, OpenProjectOptions{GodotVersion: "4.5"}))

	// An editor too old to say which version it is can't be argued with.
	is.NoErr(session.checkEditorVersion(&Editor{ProjectPath: "/games/platformer"}, OpenProjectOptions{GodotVersion: "4.4"}))
}

func TestALinkedEngineOfUnknownVersionIsTakenAtItsWord(t *testing.T) {
	is := is.New(t)

	session := engineSession(t, "")
	linked := installEngineFor(t, "4.4-stable")

	manager, err := session.EngineManager()
	is.NoErr(err)
	_, err = manager.Link(context.Background(), "my-build", linked)
	is.NoErr(err)

	editor := &Editor{ProjectPath: "/games/platformer", GodotVersion: "4.5-stable"}
	is.NoErr(session.checkEditorVersion(editor, OpenProjectOptions{GodotVersion: "my-build"}))
}
