package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/fakebin"
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
	projectPath := canonicalTempDir(t)

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

	other := canonicalTempDir(t)
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

func TestOpenProjectFailsAsSoonAsGodotExits(t *testing.T) {
	is := is.New(t)

	projectPath := canonicalTempDir(t)
	is.NoErr(os.WriteFile(filepath.Join(projectPath, "project.godot"), []byte("config_version=5\n\n[application]\n\nconfig/name=\"Exits\"\n"), 0o644))

	// Answers the version probe like a real Godot, then dies on launch.
	godotBin, err := fakebin.Write(filepath.Join(t.TempDir(), "godot"),
		"[ \"$1\" = --version ] && { echo 4.6.stable.official.89cf1416a; exit 0; }\nexit 1\n",
		"if \"%1\"==\"--version\" (echo 4.6.stable.official.89cf1416a) else (exit /b 1)\r\n")
	is.NoErr(err)

	s, err := New(Config{
		Scope:               ScopeGlobal,
		EditorInstancesPath: t.TempDir(),
		EditorRetryDelay:    time.Second,
		GodotPath:           godotBin,
	})
	is.NoErr(err)

	is.NoErr(s.Start(context.Background()))
	defer s.Close()

	started := time.Now()
	_, err = s.OpenProject(context.Background(), projectPath, OpenProjectOptions{Wait: 30 * time.Second})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "the Godot editor exited before connecting")) // not the timeout
	is.True(strings.Contains(err.Error(), "exit status 1"))
	is.True(time.Since(started) < 10*time.Second) // long before the wait would have run out
}
