package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestLocalFilePathsBecomeResPaths(t *testing.T) {
	is := is.New(t)

	godaiStdout(t, "editor-tool", "create_script", "local_path.gd",
		"--content", "extends Node\n")

	read := godaiStdout(t, "editor-tool", "read_script", "local_path.gd")
	is.True(strings.Contains(read, "extends Node")) // read back via the relative path

	read = godaiStdout(t, "editor-tool", "read_script", filepath.Join(projectPath, "local_path.gd"))
	is.True(strings.Contains(read, "extends Node")) // read back via the absolute path

	out, err := command("editor-tool", "read_script", filepath.Join(t.TempDir(), "nope.gd")).CombinedOutput()
	is.True(err != nil) // a path outside the project is rejected
	is.True(strings.Contains(string(out), "outside the project"))
}
