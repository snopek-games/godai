package core

import (
	"os"
	"strings"
	"testing"

	"github.com/matryer/is"

	"gitlab.com/snopek-games/godai/internal/isolationtest"
)

func TestEditorLogFileDisabledByDefault(t *testing.T) {
	is := is.New(t)

	t.Setenv(editorLogEnv, "")
	f, path, err := editorLogFile("/some/project")
	is.NoErr(err)
	is.True(f == nil)
	is.Equal(path, "")
}

func TestEditorLogCaptureAndTail(t *testing.T) {
	is := is.New(t)

	isolationtest.Isolate(t)
	t.Setenv(editorLogEnv, "1")

	f, path, err := editorLogFile("/some/project")
	is.NoErr(err)
	is.True(f != nil)

	// An empty log adds nothing to the timeout message.
	is.Equal(editorLogTail(path), "")

	_, err = f.WriteString("importing...\nERROR: cannot listen on port 12120\n")
	is.NoErr(err)
	is.NoErr(f.Close())

	tail := editorLogTail(path)
	is.True(strings.Contains(tail, path)) // the tail names the log file
	is.True(strings.Contains(tail, "ERROR: cannot listen on port 12120"))

	// A relaunch truncates the previous session's log.
	f, _, err = editorLogFile("/some/project")
	is.NoErr(err)
	is.NoErr(f.Close())
	is.Equal(editorLogTail(path), "")
}

func TestEditorLogTailKeepsOnlyTheEnd(t *testing.T) {
	is := is.New(t)

	path := t.TempDir() + "/editor.log"
	long := strings.Repeat("early noise\n", 1000) + "final line\n"
	is.NoErr(os.WriteFile(path, []byte(long), 0o644))

	tail := editorLogTail(path)
	is.True(len(tail) < 2200)                            // trimmed, not the whole file
	is.True(strings.HasSuffix(tail, "final line"))       // the end survives
	is.True(strings.Contains(tail, "):\nearly noise\n")) // cut on a line boundary
}

func TestEditorLogTailWithNoLog(t *testing.T) {
	is := is.New(t)

	is.Equal(editorLogTail(""), "")
	is.Equal(editorLogTail("/does/not/exist.log"), "")
}
