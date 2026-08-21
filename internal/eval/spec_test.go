package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func writeTask(t *testing.T, root, id string, tags ...string) {
	t.Helper()

	dir := filepath.Join(root, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"id": id, "tags": tags})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "task.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "instruction.md"), []byte("do the thing"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadTasksRejectsUnknownIDs(t *testing.T) {
	is := is.New(t)

	root := t.TempDir()
	writeTask(t, root, "real-task")
	writeTask(t, root, "other-task")

	_, err := LoadTasks(root, nil, []string{"real-task", "typo-task"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "unknown task ids: typo-task"))

	specs, err := LoadTasks(root, nil, []string{"real-task"})
	is.NoErr(err)
	is.Equal(len(specs), 1)
	is.Equal(specs[0].ID, "real-task")
}

func TestLoadTasksKnownIDFilteredByTagsIsNotUnknown(t *testing.T) {
	is := is.New(t)

	root := t.TempDir()
	writeTask(t, root, "real-task", "quick")

	_, err := LoadTasks(root, []string{"slow"}, []string{"real-task"})
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "no tasks matched")) // filtered by tag, not reported as unknown
}
