package addon

import (
	"slices"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestAddNode(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/add_node_test.tscn",
		"root_node_type": "Node2D",
	})

	t.Run("default_parent_path_is_the_scene_root", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_node", map[string]any{
			"node_type":  "Node2D",
			"properties": map[string]any{"name": "DefaultParentChild"},
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["node_path"], "DefaultParentChild")
	})

	t.Run("missing_node_type", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"parent_path": ".",
		}, "'node_type' is required")
	})

	t.Run("omitted_properties", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node",
		})
		is.Equal(structured["success"], true)
	})

	t.Run("bad_parent_path", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"parent_path": "NoSuchParent",
			"node_type":   "Node2D",
			"properties":  map[string]any{},
		}, "Cannot find node at 'parent_path'")
	})

	t.Run("unknown_node_type", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "NoSuchNodeType",
			"properties":  map[string]any{},
		}, "Failed to create 'NoSuchNodeType'")
	})

	t.Run("not_a_node_type", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Resource",
			"properties":  map[string]any{},
		}, "'Resource' is not a Node type")
	})

	t.Run("success_with_properties", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties": map[string]any{
				"name":     "MyChild",
				"position": "Vector2(10, 20)",
			},
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["node_path"], "MyChild")

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.True(nodeProps != nil) // the created node is in the scene
		is.Equal(nodeProps["position"], "Vector2(10, 20)")
	})

	t.Run("nested_parent", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": "MyChild",
			"node_type":   "Node",
			"properties":  map[string]any{},
		})
		is.Equal(structured["success"], true)
		nodePath, _ := structured["node_path"].(string)
		is.True(strings.HasPrefix(nodePath, "MyChild/"))
	})

	t.Run("invalid_property_value", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties": map[string]any{
				"name":     "PartiallyCreated",
				"position": "Vector2(1 2)",
			},
		})
		is.Equal(structured["success"], true) // a bad property value doesn't fail the call
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "Cannot parse"))

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"PartiallyCreated"},
		})
		nodeProps, _ := props["PartiallyCreated"].(map[string]any)
		is.Equal(nodeProps["name"], "PartiallyCreated") // created despite the bad position value
	})

	t.Run("duplicate_name_warns", func(t *testing.T) {
		is := is.New(t)

		// "MyChild" already exists, so Godot renames the new node; that must
		// come back as a warning carrying the actual name.
		structured := callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties":  map[string]any{"name": "MyChild"},
		})
		is.Equal(structured["success"], true)
		warnings, _ := structured["warnings"].([]any)
		is.Equal(len(warnings), 1)
		is.True(strings.Contains(asStrings(warnings)[0], "name"))

		nodePath, _ := structured["node_path"].(string)
		is.True(nodePath != "MyChild") // Godot renamed the duplicate
		is.True(strings.Contains(asStrings(warnings)[0], nodePath))
	})
}

func TestNodeProperties(t *testing.T) {
	setupSceneWithChild(t, "res://scenes/node_properties_test.tscn")

	t.Run("set_missing_action", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"nodes": map[string]any{},
		}, "'action' is required")
	})

	t.Run("set_missing_nodes", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Move node",
		}, "'nodes' is required")
	})

	t.Run("set_node_entry_not_an_object", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": map[string]any{
				"MyChild": "position",
			},
		}, "MyChild: must map property names to values")
	})

	t.Run("set_nodes_wrong_type", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": []any{
				map[string]any{"node_path": "MyChild"},
			},
		}, "'nodes' must be an object, but got an array")
	})

	t.Run("get_missing_node_paths", func(t *testing.T) {
		callToolErr(t, "get_node_properties", map[string]any{}, "'node_paths' is required")
	})

	t.Run("set_and_get", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": map[string]any{
				"MyChild": map[string]any{
					"position": "Vector2(42, 24)",
				},
			},
		})
		is.Equal(structured["success"], true)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.Equal(nodeProps["position"], "Vector2(42, 24)")
	})

	t.Run("set_nonexistent_node", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Set on missing node",
			"nodes": map[string]any{
				"NoSuchNode": map[string]any{
					"position": "Vector2(1, 1)",
				},
			},
		}, "NoSuchNode: cannot find node")
	})

	t.Run("set_one_nonexistent_node_still_sets_others", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move nodes",
			"nodes": map[string]any{
				"MyChild":    map[string]any{"position": "Vector2(99, 99)"},
				"NoSuchNode": map[string]any{"position": "Vector2(1, 1)"},
			},
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "NoSuchNode: cannot find node"))

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.Equal(nodeProps["position"], "Vector2(99, 99)") // the existing node was still set
	})

	t.Run("get_nonexistent_node", func(t *testing.T) {
		callToolErr(t, "get_node_properties", map[string]any{
			"node_paths": []string{"NoSuchNode"},
		}, "Cannot find node in 'node_paths'")
	})

	t.Run("get_one_nonexistent_node", func(t *testing.T) {
		callToolErr(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild", "NoSuchNode"},
		}, "Cannot find node in 'node_paths'")
	})

	t.Run("get_multiple_nodes", func(t *testing.T) {
		is := is.New(t)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{".", "MyChild"},
		})
		is.Equal(len(props), 2)
	})

	t.Run("string_property_raw", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Label",
			"properties": map[string]any{
				"name": "MyLabel",
				"text": "Hello World",
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyLabel"},
		})
		nodeProps, _ := props["MyLabel"].(map[string]any)
		is.Equal(nodeProps["text"], "Hello World") // plain strings come back unquoted
	})

	t.Run("embedded_resource", func(t *testing.T) {
		is := is.New(t)

		// An embedded resource can be created with Object(...) syntax.
		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "MeshInstance3D",
			"properties": map[string]any{
				"name": "MyMesh",
				"mesh": `Object(SphereMesh,"radius":2.0)`,
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyMesh"},
		})
		nodeProps, _ := props["MyMesh"].(map[string]any)
		is.Equal(nodeProps["mesh"], "Object(SphereMesh)") // summarized without its properties

		props = getNodeProps(t, map[string]any{
			"node_paths": []string{"MyMesh:mesh"},
		})
		meshProps, _ := props["MyMesh:mesh"].(map[string]any)
		is.Equal(meshProps["radius"], "2.0")

		props = getNodeProps(t, map[string]any{
			"node_paths": []string{"MyMesh:mesh:radius"},
		})
		is.Equal(props["MyMesh:mesh:radius"], "2.0")
	})

	t.Run("embedded_resource_without_properties", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "MeshInstance3D",
			"properties": map[string]any{
				"name": "MyBoxMesh",
				"mesh": "Object(BoxMesh)",
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyBoxMesh"},
		})
		nodeProps, _ := props["MyBoxMesh"].(map[string]any)
		is.Equal(nodeProps["mesh"], "Object(BoxMesh)")

		// Everything at its default, so nothing to report.
		props = getNodeProps(t, map[string]any{
			"node_paths": []string{"MyBoxMesh:mesh"},
		})
		meshProps, _ := props["MyBoxMesh:mesh"].(map[string]any)
		is.Equal(len(meshProps), 0)
	})

	t.Run("set_sub_property", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Resize sphere",
			"nodes": map[string]any{
				"MyMesh": map[string]any{
					"mesh:radius": "3.5",
				},
			},
		})
		is.Equal(structured["success"], true)

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Resize sphere",
			"nodes": map[string]any{
				"MyMesh:mesh": map[string]any{
					"height": "4.0",
				},
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyMesh:mesh:radius", "MyMesh:mesh:height"},
		})
		is.Equal(props["MyMesh:mesh:radius"], "3.5")
		is.Equal(props["MyMesh:mesh:height"], "4.0")
	})

	t.Run("set_vector_component", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": map[string]any{
				"MyChild": map[string]any{
					"position:x": "7.0",
				},
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild:position:x"},
		})
		is.Equal(props["MyChild:position:x"], "7.0")
	})

	t.Run("external_resource", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "create_resource", map[string]any{
			"file_path":     "res://resources/np_label_settings.tres",
			"resource_type": "LabelSettings",
			"properties": map[string]any{
				"font_size": "32",
			},
		})

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Assign label settings",
			"nodes": map[string]any{
				"MyLabel": map[string]any{
					"label_settings": `Resource("res://resources/np_label_settings.tres")`,
				},
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyLabel", "MyLabel:label_settings:font_size"},
		})
		nodeProps, _ := props["MyLabel"].(map[string]any)
		is.Equal(nodeProps["label_settings"], `Resource("res://resources/np_label_settings.tres")`)
		is.Equal(props["MyLabel:label_settings:font_size"], "32")
	})

	t.Run("invalid_value_still_sets_others", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": map[string]any{
				"MyChild": map[string]any{
					"position": "Vector2(9, 9)",
					"rotation": "garbage(",
				},
			},
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], `MyChild / rotation: "garbage" is not a variant type`))

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.Equal(nodeProps["position"], "Vector2(9, 9)") // the valid property was still set
	})

	t.Run("unknown_property", func(t *testing.T) {
		is := is.New(t)

		// An unknown property is attempted anyway (a script could handle it
		// dynamically), so the failure comes from verification.
		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Set unknown property",
			"nodes": map[string]any{
				"MyChild": map[string]any{"no_such_prop": "1"},
			},
		})
		is.Equal(structured["success"], false)
		errs, _ := structured["errors"].([]any)
		is.Equal(len(errs), 1)
		is.True(strings.Contains(asStrings(errs)[0], "has no property named 'no_such_prop'"))
	})

	t.Run("get_bad_property_path", func(t *testing.T) {
		is := is.New(t)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild:no_such_prop"},
		})
		errProps, _ := props["MyChild:no_such_prop"].(map[string]any)
		errMsg, _ := errProps["error"].(string)
		is.True(strings.Contains(errMsg, "no property named 'no_such_prop'"))
	})

	t.Run("modified_only_by_default", func(t *testing.T) {
		is := is.New(t)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)

		// "position" was changed by earlier subtests; "rotation" is still at its default.
		_, hasPosition := nodeProps["position"]
		is.True(hasPosition) // changed earlier, so reported
		_, hasRotation := nodeProps["rotation"]
		is.True(!hasRotation) // still at its default, so omitted
	})

	t.Run("include_defaults", func(t *testing.T) {
		is := is.New(t)

		props := getNodeProps(t, map[string]any{
			"node_paths":       []string{"MyChild"},
			"include_defaults": true,
		})
		nodeProps, _ := props["MyChild"].(map[string]any)

		_, hasRotation := nodeProps["rotation"]
		is.True(hasRotation) // still at its default, but reported now
	})

	// Neither saved nor shown in the inspector: derived from "transform"
	// ("global_position", "rotation_degrees"), or unrelated to the node's state
	// ("multiplayer", "owner").
	hiddenProps := []string{"global_position", "global_transform", "rotation_degrees", "multiplayer", "owner"}

	t.Run("hidden_properties_omitted", func(t *testing.T) {
		is := is.New(t)

		for _, includeDefaults := range []bool{false, true} {
			props := getNodeProps(t, map[string]any{
				"node_paths":       []string{"MyChild"},
				"include_defaults": includeDefaults,
			})
			nodeProps, _ := props["MyChild"].(map[string]any)
			for _, name := range hiddenProps {
				_, has := nodeProps[name]
				is.True(!has) // hidden even with include_defaults
			}
		}
	})

	t.Run("hidden_property_by_name", func(t *testing.T) {
		is := is.New(t)

		// The scene root sits at the origin, so MyChild's global position is
		// just its position.
		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild:position", "MyChild:global_position"},
		})
		is.Equal(props["MyChild:global_position"], props["MyChild:position"])
	})

	t.Run("unset_string_property_omitted", func(t *testing.T) {
		is := is.New(t)

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)

		// Godot tracks no default for these, but an empty one still isn't worth
		// reporting; the node's name always is.
		_, hasSceneFilePath := nodeProps["scene_file_path"]
		is.True(!hasSceneFilePath) // empty, so omitted
		is.Equal(nodeProps["name"], "MyChild")
	})
}

func TestRemoveNode(t *testing.T) {
	setupSceneWithChild(t, "res://scenes/remove_node_test.tscn")

	t.Run("missing_node_path", func(t *testing.T) {
		callToolErr(t, "remove_node", map[string]any{}, "'node_path' is required")
	})

	t.Run("nonexistent", func(t *testing.T) {
		callToolErr(t, "remove_node", map[string]any{
			"node_path": "NoSuchNode",
		}, "Cannot find node at 'node_path'")
	})

	t.Run("scene_root", func(t *testing.T) {
		callToolErr(t, "remove_node", map[string]any{
			"node_path": ".",
		}, "Cannot remove scene root")
	})

	t.Run("success", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "remove_node", map[string]any{
			"node_path": "MyChild",
		})
		is.Equal(structured["success"], true)
		notes, _ := structured["notes"].([]any)
		is.True(anyLineContains(notes, "save_scene"))

		callToolErr(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild"},
		}, "Cannot find node in 'node_paths'")
	})
}

func TestNodeGroups(t *testing.T) {
	setupSceneWithChild(t, "res://scenes/groups_test.tscn")

	t.Run("add_and_get", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_to_group", map[string]any{
			"node_path": "MyChild",
			"groups":    []string{"enemies", "mobs"},
		})
		is.Equal(structured["success"], true)

		groups := getNodeGroups(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		got, _ := groups["MyChild"].([]any)
		is.Equal(len(got), 2)
		is.True(slices.Contains(asStrings(got), "enemies"))
		is.True(slices.Contains(asStrings(got), "mobs"))
	})

	t.Run("remove", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "remove_from_group", map[string]any{
			"node_path": "MyChild",
			"groups":    []string{"mobs"},
		})

		groups := getNodeGroups(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		got := asStrings(groups["MyChild"].([]any))
		is.Equal(got, []string{"enemies"})
	})

	t.Run("add_idempotent", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "add_to_group", map[string]any{
			"node_path": "MyChild",
			"groups":    []string{"enemies"},
		})
		is.Equal(structured["success"], true)

		groups := getNodeGroups(t, map[string]any{
			"node_paths": []string{"MyChild"},
		})
		got := asStrings(groups["MyChild"].([]any))
		is.Equal(got, []string{"enemies"})
	})

	t.Run("get_missing_node", func(t *testing.T) {
		callToolErr(t, "get_node_groups", map[string]any{
			"node_paths": []string{"NoSuchNode"},
		}, "Cannot find node in 'node_paths'")
	})

	t.Run("get_one_missing_node", func(t *testing.T) {
		callToolErr(t, "get_node_groups", map[string]any{
			"node_paths": []string{"MyChild", "NoSuchNode"},
		}, "Cannot find node in 'node_paths'")
	})

	t.Run("get_missing_node_paths", func(t *testing.T) {
		callToolErr(t, "get_node_groups", map[string]any{}, "'node_paths' is required")
	})

	t.Run("add_missing_node", func(t *testing.T) {
		callToolErr(t, "add_to_group", map[string]any{
			"node_path": "NoSuchNode",
			"groups":    []string{"enemies"},
		}, "Cannot find node at 'node_path'")
	})

	t.Run("add_missing_node_path", func(t *testing.T) {
		callToolErr(t, "add_to_group", map[string]any{
			"groups": []string{"enemies"},
		}, "'node_path' is required")
	})

	t.Run("add_groups_required", func(t *testing.T) {
		callToolErr(t, "add_to_group", map[string]any{
			"node_path": "MyChild",
		}, "'groups' is required")
	})

	t.Run("persists_to_scene", func(t *testing.T) {
		is := is.New(t)
		callToolOK(t, "save_scene", nil)
		if content := readProjectFile(t, "scenes/groups_test.tscn"); content != "" {
			is.True(strings.Contains(content, `groups=["enemies"]`))
		}
	})
}

func TestNodeSignals(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/signals_test.tscn",
		"root_node_type": "Node2D",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node2D",
		"properties":  map[string]any{"name": "Source"},
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node",
		"properties":  map[string]any{"name": "Target"},
	})

	connectArgs := map[string]any{
		"from_node": "Source",
		"signal":    "ready",
		"to_node":   "Target",
		"method":    "queue_free",
	}

	t.Run("connect", func(t *testing.T) {
		is := is.New(t)
		structured := callToolOK(t, "connect_signal", connectArgs)
		is.Equal(structured["success"], true)

		runEditorScript(t, `var root = EditorInterface.get_edited_scene_root()
var source = root.get_node("Source")
var target = root.get_node("Target")
if source.is_connected("ready", Callable(target, "queue_free")):
	return OK
return FAILED`)
	})

	t.Run("already_connected", func(t *testing.T) {
		callToolErr(t, "connect_signal", connectArgs, "already connected")
	})

	t.Run("unknown_signal", func(t *testing.T) {
		callToolErr(t, "connect_signal", map[string]any{
			"from_node": "Source",
			"signal":    "no_such_signal",
			"to_node":   "Target",
			"method":    "queue_free",
		}, "has no signal named 'no_such_signal'")
	})

	t.Run("unknown_method", func(t *testing.T) {
		callToolErr(t, "connect_signal", map[string]any{
			"from_node": "Source",
			"signal":    "ready",
			"to_node":   "Target",
			"method":    "no_such_method",
		}, "has no method named 'no_such_method'")
	})

	t.Run("missing_from_node", func(t *testing.T) {
		callToolErr(t, "connect_signal", map[string]any{
			"from_node": "NoSuchNode",
			"signal":    "ready",
			"to_node":   "Target",
			"method":    "queue_free",
		}, "Cannot find 'from_node'")
	})

	t.Run("missing_signal_arg", func(t *testing.T) {
		callToolErr(t, "connect_signal", map[string]any{
			"from_node": "Source",
			"to_node":   "Target",
			"method":    "queue_free",
		}, "'signal' is required")
	})

	t.Run("persists_to_scene", func(t *testing.T) {
		is := is.New(t)
		callToolOK(t, "save_scene", nil)
		if content := readProjectFile(t, "scenes/signals_test.tscn"); content != "" {
			is.True(strings.Contains(content, `[connection signal="ready" from="Source" to="Target" method="queue_free"]`))
		}
	})

	t.Run("disconnect", func(t *testing.T) {
		is := is.New(t)
		structured := callToolOK(t, "disconnect_signal", connectArgs)
		is.Equal(structured["success"], true)

		runEditorScript(t, `var root = EditorInterface.get_edited_scene_root()
var source = root.get_node("Source")
var target = root.get_node("Target")
if source.is_connected("ready", Callable(target, "queue_free")):
	return FAILED
return OK`)
	})

	t.Run("disconnect_not_connected", func(t *testing.T) {
		callToolErr(t, "disconnect_signal", connectArgs, "is not connected")
	})
}

func TestNodeScript(t *testing.T) {
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/node_script_test.tscn",
		"root_node_type": "Node2D",
	})
	callToolOK(t, "add_node", map[string]any{
		"parent_path": ".",
		"node_type":   "Node",
		"properties":  map[string]any{"name": "ScriptHost"},
	})

	callToolOK(t, "create_script", map[string]any{
		"file_path":  "res://scripts/ns_compatible.gd",
		"base_class": "Node",
	})
	callToolOK(t, "create_script", map[string]any{
		"file_path":  "res://scripts/ns_incompatible.gd",
		"base_class": "Node3D",
	})

	t.Run("attach", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "attach_script", map[string]any{
			"node_path":   "ScriptHost",
			"script_path": "res://scripts/ns_compatible.gd",
		})
		is.Equal(structured["success"], true)

		tree := callToolOK(t, "get_current_scene_tree", nil)
		children, _ := tree["children"].([]any)
		var host map[string]any
		for _, raw := range children {
			child, _ := raw.(map[string]any)
			if child["name"] == "ScriptHost" {
				host = child
			}
		}
		is.True(host != nil) // ScriptHost is in the scene tree
		is.Equal(host["script"], "res://scripts/ns_compatible.gd")
	})

	t.Run("attach_incompatible", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"node_path":   "ScriptHost",
			"script_path": "res://scripts/ns_incompatible.gd",
		}, "not compatible")
	})

	t.Run("attach_incompatible_via_set_node_properties", func(t *testing.T) {
		is := is.New(t)

		// Setting 'script' attaches one too, so it gets the same check.
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Attach a script by property",
			"nodes": map[string]any{
				"ScriptHost": map[string]any{
					"script": `Resource("res://scripts/ns_incompatible.gd")`,
				},
			},
		}, "not compatible")

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"ScriptHost"},
		})
		nodeProps, _ := props["ScriptHost"].(map[string]any)
		is.Equal(nodeProps["script"], `Resource("res://scripts/ns_compatible.gd")`) // the incompatible script was not attached
	})

	t.Run("attach_and_detach_via_set_node_properties", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Detach a script by property",
			"nodes": map[string]any{
				"ScriptHost": map[string]any{"script": "null"},
			},
		})

		props := getNodeProps(t, map[string]any{
			"node_paths": []string{"ScriptHost"},
		})
		nodeProps, _ := props["ScriptHost"].(map[string]any)
		_, hasScript := nodeProps["script"]
		is.True(!hasScript) // "null" detached the script

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Attach a script by property",
			"nodes": map[string]any{
				"ScriptHost": map[string]any{
					"script": `Resource("res://scripts/ns_compatible.gd")`,
				},
			},
		})

		props = getNodeProps(t, map[string]any{
			"node_paths": []string{"ScriptHost"},
		})
		nodeProps, _ = props["ScriptHost"].(map[string]any)
		is.Equal(nodeProps["script"], `Resource("res://scripts/ns_compatible.gd")`)
	})

	t.Run("attach_nonexistent_script", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"node_path":   "ScriptHost",
			"script_path": "res://scripts/no_such_script.gd",
		}, "doesn't exist")
	})

	t.Run("attach_missing_node", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"node_path":   "NoSuchNode",
			"script_path": "res://scripts/ns_compatible.gd",
		}, "Cannot find node at 'node_path'")
	})

	t.Run("attach_missing_script_path", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"node_path": "ScriptHost",
		}, "'script_path' is required")
	})

	t.Run("attach_missing_node_path", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"script_path": "res://scripts/ns_compatible.gd",
		}, "'node_path' is required")
	})

	t.Run("detach_missing_node_path", func(t *testing.T) {
		callToolErr(t, "detach_script", map[string]any{}, "'node_path' is required")
	})

	t.Run("detach", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "detach_script", map[string]any{
			"node_path": "ScriptHost",
		})
		is.Equal(structured["success"], true)

		tree := callToolOK(t, "get_current_scene_tree", nil)
		children, _ := tree["children"].([]any)
		for _, raw := range children {
			child, _ := raw.(map[string]any)
			if child["name"] == "ScriptHost" {
				_, hasScript := child["script"]
				is.True(!hasScript) // the detached script is gone from the tree
			}
		}
	})

	t.Run("detach_no_script", func(t *testing.T) {
		callToolErr(t, "detach_script", map[string]any{
			"node_path": "ScriptHost",
		}, "has no script attached")
	})
}
