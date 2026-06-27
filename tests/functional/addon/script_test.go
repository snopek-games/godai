package addon

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestScriptFiles(t *testing.T) {
	t.Run("create_and_read", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "create_script", map[string]any{
			"file_path":  "res://scripts/sf_basic.gd",
			"base_class": "Node2D",
		})
		is.Equal(structured["success"], true)

		read := callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_basic.gd",
		})
		is.Equal(read["content"], "extends Node2D\n")
		is.Equal(read["open_in_editor"], false)

		if content := readProjectFile(t, "scripts/sf_basic.gd"); content != "" {
			is.Equal(content, "extends Node2D\n")
		}
	})

	t.Run("create_with_content", func(t *testing.T) {
		is := is.New(t)

		body := "extends Node\n\nfunc hello() -> String:\n\treturn \"hi\"\n"
		callToolOK(t, "create_script", map[string]any{
			"file_path": "res://scripts/sf_content.gd",
			"content":   body,
		})

		read := callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_content.gd",
		})
		is.Equal(read["content"], body)
	})

	t.Run("create_already_exists", func(t *testing.T) {
		callToolErr(t, "create_script", map[string]any{
			"file_path": "res://scripts/sf_basic.gd",
		}, "already exists")
	})

	t.Run("read_nonexistent", func(t *testing.T) {
		callToolErr(t, "read_script", map[string]any{
			"file_path": "res://scripts/no_such.gd",
		}, "doesn't exist")
	})

	t.Run("write_while_closed", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_script", map[string]any{
			"file_path": "res://scripts/sf_closed.gd",
		})

		body := "extends Node\n\nvar written := true\n"
		structured := callToolOK(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_closed.gd",
			"content":   body,
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["open_in_editor"], false)
		is.Equal(structured["saved"], true)

		if content := readProjectFile(t, "scripts/sf_closed.gd"); content != "" {
			is.Equal(content, body)
		}
	})

	t.Run("write_nonexistent", func(t *testing.T) {
		callToolErr(t, "write_script", map[string]any{
			"file_path": "res://scripts/no_such.gd",
			"content":   "extends Node\n",
		}, "use create_script")
	})

	t.Run("open_write_and_save", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})

		// Open it in the editor.
		structured := callToolOK(t, "open_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})
		is.Equal(structured["success"], true)
		settleEditor(t)

		// read_script now reflects the open editor buffer.
		read := callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})
		is.Equal(read["open_in_editor"], true)

		// Writing updates the editor buffer, but does NOT save to disk.
		body := "extends Node\n\nvar from_editor := 1\n"
		written := callToolOK(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
			"content":   body,
		})
		is.Equal(written["open_in_editor"], true)
		is.Equal(written["saved"], false)

		// The buffer reflects the new content...
		read = callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})
		is.Equal(read["content"], body)

		// ... and save_script flushes it to disk.
		saved := callToolOK(t, "save_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})
		is.Equal(saved["success"], true)
		is.Equal(saved["saved"], true)

		if content := readProjectFile(t, "scripts/sf_open.gd"); content != "" {
			is.Equal(content, body)
		}
	})

	t.Run("save_not_open", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_script", map[string]any{
			"file_path": "res://scripts/sf_notopen.gd",
		})
		structured := callToolOK(t, "save_script", map[string]any{
			"file_path": "res://scripts/sf_notopen.gd",
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["saved"], false)
	})

	t.Run("write_requires_prior_read", func(t *testing.T) {
		is := is.New(t)

		// A script created outside of these tools: the AI has never seen it.
		runEditorScript(t, `var f = FileAccess.open("res://scripts/sf_external.gd", FileAccess.WRITE)
f.store_string("extends Node\n")
f.close()
EditorInterface.get_resource_filesystem().update_file("res://scripts/sf_external.gd")
return OK`)

		// Writing without reading first is refused.
		callToolErr(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_external.gd",
			"content":   "extends Node\n\nvar overwritten := true\n",
		}, "must read")

		// After reading it, writing is allowed.
		callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_external.gd",
		})
		structured := callToolOK(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_external.gd",
			"content":   "extends Node\n\nvar overwritten := true\n",
		})
		is.Equal(structured["success"], true)
	})

	t.Run("write_rejects_stale", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_script", map[string]any{
			"file_path": "res://scripts/sf_stale.gd",
		})
		callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_stale.gd",
		})

		// Simulate the user changing the file behind the AI's back.
		runEditorScript(t, `var f = FileAccess.open("res://scripts/sf_stale.gd", FileAccess.WRITE)
f.store_string("extends Node\n\nvar user_edit := 42\n")
f.close()
EditorInterface.get_resource_filesystem().update_file("res://scripts/sf_stale.gd")
return OK`)

		// The AI's write must be refused, since the file changed since it read.
		callToolErr(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_stale.gd",
			"content":   "extends Node\n\nvar ai_edit := 1\n",
		}, "has changed since you last read")

		// The user's edit is still intact on disk.
		if content := readProjectFile(t, "scripts/sf_stale.gd"); content != "" {
			is.True(strings.Contains(content, "user_edit"))
		}

		// Re-reading clears the staleness, and the write then succeeds.
		read := callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_stale.gd",
		})
		is.True(strings.Contains(read["content"].(string), "user_edit"))

		structured := callToolOK(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_stale.gd",
			"content":   "extends Node\n\nvar ai_edit := 1\n",
		})
		is.Equal(structured["success"], true)
	})
}
