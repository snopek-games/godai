package addon

import (
	"slices"
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

		structured := callToolOK(t, "open_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})
		is.Equal(structured["success"], true)
		settleEditor(t)

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

		read = callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_open.gd",
		})
		is.Equal(read["content"], body)

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

		callToolErr(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_external.gd",
			"content":   "extends Node\n\nvar overwritten := true\n",
		}, "must read")

		callToolOK(t, "read_script", map[string]any{
			"file_path": "res://scripts/sf_external.gd",
		})
		structured := callToolOK(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_external.gd",
			"content":   "extends Node\n\nvar overwritten := true\n",
		})
		is.Equal(structured["success"], true)
	})

	// A file that isn't a Script gets a tab in the script editor just like a
	// script does: the tools have to keep track of which tab holds which file,
	// or they read and write the buffer of some other open file.
	t.Run("with_a_non_script_file_open", func(t *testing.T) {
		is := is.New(t)

		const notesPath = "res://scripts/sf_tab_notes.json"
		const notesBody = "{\n\t\"not\": \"a script\"\n}\n"
		const onePath = "res://scripts/sf_tab_one.gd"
		const twoPath = "res://scripts/sf_tab_two.gd"

		callToolOK(t, "create_script", map[string]any{
			"file_path":  onePath,
			"base_class": "Node",
		})
		callToolOK(t, "create_script", map[string]any{
			"file_path":  twoPath,
			"base_class": "Node2D",
		})
		writeProjectFileFromEditor(t, notesPath, notesBody)

		openNonScriptInScriptEditor(t, notesPath)
		callToolOK(t, "open_script", map[string]any{"file_path": onePath})
		callToolOK(t, "open_script", map[string]any{"file_path": twoPath})
		settleEditor(t)
		t.Cleanup(func() { closeScriptEditorFiles(t, notesPath, onePath, twoPath) })

		one := callToolOK(t, "read_script", map[string]any{"file_path": onePath})
		is.Equal(one["open_in_editor"], true)
		is.Equal(one["content"], "extends Node\n")

		two := callToolOK(t, "read_script", map[string]any{"file_path": twoPath})
		is.Equal(two["open_in_editor"], true)
		is.Equal(two["content"], "extends Node2D\n")

		body := "extends Node\n\nvar written := true\n"
		callToolOK(t, "write_script", map[string]any{
			"file_path": onePath,
			"content":   body,
		})

		// Only the script that was written to may have changed.
		buffers := scriptEditorBuffers(t)
		is.True(slices.Contains(buffers, notesBody))
		is.True(slices.Contains(buffers, body))
		is.True(slices.Contains(buffers, "extends Node2D\n"))
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

		callToolErr(t, "write_script", map[string]any{
			"file_path": "res://scripts/sf_stale.gd",
			"content":   "extends Node\n\nvar ai_edit := 1\n",
		}, "has changed since you last read")

		if content := readProjectFile(t, "scripts/sf_stale.gd"); content != "" {
			is.True(strings.Contains(content, "user_edit"))
		}

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
