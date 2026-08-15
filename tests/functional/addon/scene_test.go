package addon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestNoSceneOpen(t *testing.T) {
	requireManagedProject(t)
	closeAllScenes(t)

	t.Run("get_current_scene", func(t *testing.T) {
		callToolErr(t, "get_current_scene", nil, "No scene open")
	})

	t.Run("get_current_scene_tree", func(t *testing.T) {
		callToolErr(t, "get_current_scene_tree", nil, "No scene open")
	})

	t.Run("get_selected_nodes", func(t *testing.T) {
		callToolErr(t, "get_selected_nodes", nil, "No scene open")
	})

	t.Run("get_node_properties", func(t *testing.T) {
		callToolErr(t, "get_node_properties", map[string]any{
			"node_paths": []string{"."},
		}, "No scene open")
	})

	t.Run("set_node_properties", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Test",
			"nodes": map[string]any{
				".": map[string]any{"name": "Whatever"},
			},
		}, "No scene open")
	})

	t.Run("add_node", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties":  map[string]any{},
		}, "No scene open")
	})

	t.Run("remove_node", func(t *testing.T) {
		callToolErr(t, "remove_node", map[string]any{
			"node_path": ".",
		}, "No scene open")
	})

	t.Run("get_node_groups", func(t *testing.T) {
		callToolErr(t, "get_node_groups", map[string]any{
			"node_paths": []string{"."},
		}, "No scene open")
	})

	t.Run("add_to_group", func(t *testing.T) {
		callToolErr(t, "add_to_group", map[string]any{
			"node_path": ".",
			"groups":    []string{"foo"},
		}, "No scene open")
	})

	t.Run("remove_from_group", func(t *testing.T) {
		callToolErr(t, "remove_from_group", map[string]any{
			"node_path": ".",
			"groups":    []string{"foo"},
		}, "No scene open")
	})

	t.Run("connect_signal", func(t *testing.T) {
		callToolErr(t, "connect_signal", map[string]any{
			"from_node": ".",
			"signal":    "ready",
			"to_node":   ".",
			"method":    "queue_free",
		}, "No scene open")
	})

	t.Run("disconnect_signal", func(t *testing.T) {
		callToolErr(t, "disconnect_signal", map[string]any{
			"from_node": ".",
			"signal":    "ready",
			"to_node":   ".",
			"method":    "queue_free",
		}, "No scene open")
	})

	t.Run("attach_script", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"node_path":   ".",
			"script_path": "res://scripts/whatever.gd",
		}, "No scene open")
	})

	t.Run("detach_script", func(t *testing.T) {
		callToolErr(t, "detach_script", map[string]any{
			"node_path": ".",
		}, "No scene open")
	})

	t.Run("instantiate_scene", func(t *testing.T) {
		callToolErr(t, "instantiate_scene", map[string]any{
			"parent_path": ".",
			"scene_path":  "res://scenes/whatever.tscn",
		}, "No scene open")
	})
}

func TestCreateScene(t *testing.T) {
	t.Run("missing_file_path", func(t *testing.T) {
		callToolErr(t, "create_scene", map[string]any{
			"root_node_type": "Node2D",
		}, "'file_path' is required")
	})

	t.Run("missing_root_node_type", func(t *testing.T) {
		callToolErr(t, "create_scene", map[string]any{
			"file_path": "res://scenes/nope.tscn",
		}, "'root_node_type' is required")
	})

	t.Run("unknown_root_node_type", func(t *testing.T) {
		callToolErr(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/nope.tscn",
			"root_node_type": "NoSuchNodeType",
		}, "Unknown 'root_node_type'")
	})

	t.Run("not_a_node_type", func(t *testing.T) {
		callToolErr(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/nope.tscn",
			"root_node_type": "Resource",
		}, "'Resource' is not a Node type")
	})

	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/main_scene.tscn",
			"root_node_type": "Node2D",
		})
		is.Equal(structured["success"], true)

		scene := callToolOK(t, "get_current_scene", nil)
		is.Equal(scene["scene_path"], "res://scenes/main_scene.tscn")
		is.Equal(scene["root_node_type"], "Node2D")
		is.Equal(scene["root_node_name"], "MainScene") // root node name should be the file name, in Pascal case

		if projectDir != "" {
			_, err := os.Stat(filepath.Join(projectDir, "scenes", "main_scene.tscn"))
			is.NoErr(err)
		}
	})

	t.Run("already_exists", func(t *testing.T) {
		callToolErr(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/main_scene.tscn",
			"root_node_type": "Node2D",
		}, "already exists")
	})

	t.Run("relative_path_and_new_directory", func(t *testing.T) {
		is := is.New(t)

		// No res:// prefix, and a directory that doesn't exist yet.
		structured := callToolOK(t, "create_scene", map[string]any{
			"file_path":      "scenes/sub_dir/second_scene.tscn",
			"root_node_type": "Node3D",
		})
		is.Equal(structured["success"], true)

		scene := callToolOK(t, "get_current_scene", nil)
		is.Equal(scene["scene_path"], "res://scenes/sub_dir/second_scene.tscn")
	})
}

func TestOpenScene(t *testing.T) {
	t.Run("missing_file_path", func(t *testing.T) {
		callToolErr(t, "open_scene", map[string]any{}, "'file_path' is required")
	})

	t.Run("nonexistent", func(t *testing.T) {
		callToolErr(t, "open_scene", map[string]any{
			"file_path": "res://no_such_scene.tscn",
		}, "doesn't exist")
	})

	t.Run("success", func(t *testing.T) {
		requireManagedProject(t)
		is := is.New(t)

		// No res:// prefix, to also cover the prefixing code path.
		structured := callToolOK(t, "open_scene", map[string]any{
			"file_path": "fixtures/scene_with_script.tscn",
		})
		is.Equal(structured["success"], true)

		scene := callToolOK(t, "get_current_scene", nil)
		is.Equal(scene["scene_path"], "res://fixtures/scene_with_script.tscn")
		is.Equal(scene["root_node_name"], "SceneWithScript")
	})

	t.Run("scene_tree", func(t *testing.T) {
		requireManagedProject(t)
		is := is.New(t)

		callToolOK(t, "open_scene", map[string]any{
			"file_path": "res://fixtures/scene_with_script.tscn",
		})

		tree := callToolOK(t, "get_current_scene_tree", nil)
		is.Equal(tree["name"], "SceneWithScript")
		is.Equal(tree["type"], "Node")
		is.Equal(tree["path"], ".")
		is.Equal(tree["script"], "res://fixtures/simple_node.gd")

		children, _ := tree["children"].([]any)
		is.Equal(len(children), 1)

		child, _ := children[0].(map[string]any)
		is.Equal(child["name"], "Child")
		is.Equal(child["type"], "Node2D")
		is.Equal(child["path"], "Child")
		_, hasScript := child["script"]
		is.True(!hasScript)

		grandChildren, _ := child["children"].([]any)
		is.Equal(len(grandChildren), 1)

		grandChild, _ := grandChildren[0].(map[string]any)
		is.Equal(grandChild["path"], "Child/GrandChild")
		_, hasChildren := grandChild["children"]
		is.True(!hasChildren)
	})
}

func TestGetSelectedNodes(t *testing.T) {
	setupSceneWithChild(t, "res://scenes/get_selected_test.tscn")

	t.Run("none_selected", func(t *testing.T) {
		is := is.New(t)

		runEditorScript(t, `EditorInterface.get_selection().clear()
await Engine.get_main_loop().process_frame
return OK`)

		structured := callToolOK(t, "get_selected_nodes", nil)
		paths, _ := structured["node_paths"].([]any)
		is.Equal(len(paths), 0)
	})

	t.Run("some_selected", func(t *testing.T) {
		is := is.New(t)

		runEditorScript(t, `var root = EditorInterface.get_edited_scene_root()
var selection = EditorInterface.get_selection()
selection.clear()
selection.add_node(root.get_node("MyChild"))
await Engine.get_main_loop().process_frame
return OK`)

		structured := callToolOK(t, "get_selected_nodes", nil)
		paths, _ := structured["node_paths"].([]any)
		is.Equal(len(paths), 1)
		is.Equal(paths[0], "MyChild")
	})
}

func TestInstantiateScene(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/instance_source.tscn",
		"root_node_type": "Node2D",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node2D",
		"properties":  map[string]any{"name": "SourceChild"},
	})
	callToolOK(t, "save_scene", nil)

	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/instance_target.tscn",
		"root_node_type": "Node2D",
	})

	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "instantiate_scene", map[string]any{
			"parent_path": ".",
			"scene_path":  "res://scenes/instance_source.tscn",
			"name":        "MyInstance",
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["node_path"], "MyInstance")

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyInstance"},
		})
		nodeProps, _ := props["MyInstance"].(map[string]any)
		is.True(len(nodeProps) > 0)

		callToolOK(t, "save_scene", nil)
		if content := readProjectFile(t, "scenes/instance_target.tscn"); content != "" {
			is.True(strings.Contains(content, "instance_source.tscn"))
			is.True(strings.Contains(content, "instance=ExtResource"))
		}
	})

	t.Run("into_itself", func(t *testing.T) {
		callToolErr(t, "instantiate_scene", map[string]any{
			"parent_path": ".",
			"scene_path":  "res://scenes/instance_target.tscn",
		}, "into itself")
	})

	t.Run("nonexistent_scene", func(t *testing.T) {
		callToolErr(t, "instantiate_scene", map[string]any{
			"parent_path": ".",
			"scene_path":  "res://scenes/no_such_scene.tscn",
		}, "doesn't exist")
	})

	t.Run("bad_parent", func(t *testing.T) {
		callToolErr(t, "instantiate_scene", map[string]any{
			"parent_path": "NoSuchParent",
			"scene_path":  "res://scenes/instance_source.tscn",
		}, "Cannot find node at 'parent_path'")
	})

	t.Run("not_a_scene", func(t *testing.T) {
		callToolOK(t, "create_resource", map[string]any{
			"file_path":     "res://resources/inst_not_scene.tres",
			"resource_type": "LabelSettings",
			"properties":    map[string]any{},
		})
		callToolErr(t, "instantiate_scene", map[string]any{
			"parent_path": ".",
			"scene_path":  "res://resources/inst_not_scene.tres",
		}, "is not a scene")
	})

	t.Run("missing_parent_path", func(t *testing.T) {
		callToolErr(t, "instantiate_scene", map[string]any{
			"scene_path": "res://scenes/instance_source.tscn",
		}, "'parent_path' is required")
	})

	t.Run("missing_scene_path", func(t *testing.T) {
		callToolErr(t, "instantiate_scene", map[string]any{
			"parent_path": ".",
		}, "'scene_path' is required")
	})
}

func TestSaveScene(t *testing.T) {
	t.Run("no_scene_open", func(t *testing.T) {
		requireManagedProject(t)
		closeAllScenes(t)
		callToolErr(t, "save_scene", nil, "No scene open")
	})

	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_scene_test.tscn",
			"root_node_type": "Node2D",
		})
		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties":  map[string]any{"name": "SavedChild"},
		})

		structured := callToolOK(t, "save_scene", nil)
		is.Equal(structured["success"], true)
		is.Equal(structured["scene_path"], "res://scenes/save_scene_test.tscn")

		if content := readProjectFile(t, "scenes/save_scene_test.tscn"); content != "" {
			is.True(strings.Contains(content, `name="SavedChild"`))
		}
	})
}

func TestSaveSceneAs(t *testing.T) {
	t.Run("no_scene_open", func(t *testing.T) {
		requireManagedProject(t)
		closeAllScenes(t)
		callToolErr(t, "save_scene_as", map[string]any{
			"file_path": "res://scenes/save_as_nope.tscn",
		}, "No scene open")
	})

	t.Run("missing_file_path", func(t *testing.T) {
		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_as_missing_path.tscn",
			"root_node_type": "Node2D",
		})
		callToolErr(t, "save_scene_as", nil, "'file_path' is required")
	})

	t.Run("escapes_project", func(t *testing.T) {
		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_as_escape.tscn",
			"root_node_type": "Node2D",
		})
		callToolErr(t, "save_scene_as", map[string]any{
			"file_path": "../outside.tscn",
		}, "must be inside the project")
	})

	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_as_source.tscn",
			"root_node_type": "Node2D",
		})
		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties":  map[string]any{"name": "SavedAsChild"},
		})

		// No res:// prefix, to also cover the prefixing code path.
		structured := callToolOK(t, "save_scene_as", map[string]any{
			"file_path": "scenes/save_as_dest.tscn",
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["scene_path"], "res://scenes/save_as_dest.tscn")

		// The current scene's path follows the save-as to the new file.
		scene := callToolOK(t, "get_current_scene", nil)
		is.Equal(scene["scene_path"], "res://scenes/save_as_dest.tscn")

		if content := readProjectFile(t, "scenes/save_as_dest.tscn"); content != "" {
			is.True(strings.Contains(content, `name="SavedAsChild"`))
		}

		// A subsequent save_scene writes to the new path.
		saved := callToolOK(t, "save_scene", nil)
		is.Equal(saved["scene_path"], "res://scenes/save_as_dest.tscn")
	})

	t.Run("current_path_saves_in_place", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_as_in_place.tscn",
			"root_node_type": "Node2D",
		})
		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties":  map[string]any{"name": "InPlaceChild"},
		})

		// The scene's own path isn't a collision: it saves like save_scene.
		structured := callToolOK(t, "save_scene_as", map[string]any{
			"file_path": "res://scenes/save_as_in_place.tscn",
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["scene_path"], "res://scenes/save_as_in_place.tscn")

		if content := readProjectFile(t, "scenes/save_as_in_place.tscn"); content != "" {
			is.True(strings.Contains(content, `name="InPlaceChild"`))
		}
	})

	t.Run("already_exists", func(t *testing.T) {
		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_as_existing.tscn",
			"root_node_type": "Node2D",
		})
		callToolOK(t, "create_scene", map[string]any{
			"file_path":      "res://scenes/save_as_other.tscn",
			"root_node_type": "Node2D",
		})
		callToolErr(t, "save_scene_as", map[string]any{
			"file_path": "res://scenes/save_as_existing.tscn",
		}, "already exists")
	})
}
