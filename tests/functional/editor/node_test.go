package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestAddNode(t *testing.T) {
	// Create a fresh scene to work in.
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://scenes/add_node_test.tscn",
		"root_node_type": "Node2D",
	})

	t.Run("missing_parent_path", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"node_type": "Node2D",
		}, "'parent_path' is required")
	})

	t.Run("missing_node_type", func(t *testing.T) {
		callToolErr(t, "add_node", map[string]any{
			"parent_path": ".",
		}, "'node_type' is required")
	})

	t.Run("omitted_properties", func(t *testing.T) {
		is := is.New(t)

		// 'properties' is optional; omitting it adds the node with defaults.
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
				// A String property (no str_to_var conversion)...
				"name": "MyChild",
				// ... and a Vector2 property (str_to_var conversion).
				"position": "Vector2(10, 20)",
			},
		})
		is.Equal(structured["success"], true)
		is.Equal(structured["node_path"], "MyChild")

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.True(nodeProps != nil)
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

		callToolErr(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Node2D",
			"properties": map[string]any{
				"name":     "NotCreated",
				"position": "Vector2(1 2)",
			},
		}, "Cannot parse")

		// The node must not have been created.
		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"NotCreated"},
		})
		nodeProps, _ := props["NotCreated"].(map[string]any)
		is.Equal(len(nodeProps), 0)
	})
}

func TestNodeProperties(t *testing.T) {
	setupSceneWithChild(t, "res://scenes/node_properties_test.tscn")

	t.Run("set_missing_action", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"nodes": []any{},
		}, "'action' is required")
	})

	t.Run("set_and_get", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": []any{
				map[string]any{
					"node_path": "MyChild",
					"properties": map[string]any{
						"position": "Vector2(42, 24)",
					},
				},
			},
		})
		is.Equal(structured["MyChild"], true)

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.Equal(nodeProps["position"], "Vector2(42, 24)")
	})

	t.Run("set_nonexistent_node", func(t *testing.T) {
		is := is.New(t)

		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Set on missing node",
			"nodes": []any{
				map[string]any{
					"node_path": "NoSuchNode",
					"properties": map[string]any{
						"position": "Vector2(1, 1)",
					},
				},
			},
		})
		is.Equal(structured["NoSuchNode"], false)
	})

	t.Run("get_nonexistent_node", func(t *testing.T) {
		is := is.New(t)

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"NoSuchNode"},
		})
		nodeProps, ok := props["NoSuchNode"].(map[string]any)
		is.True(ok)
		is.Equal(len(nodeProps), 0)
	})

	t.Run("get_multiple_nodes", func(t *testing.T) {
		is := is.New(t)

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{".", "MyChild"},
		})
		is.Equal(len(props), 2)
	})

	t.Run("string_property_raw", func(t *testing.T) {
		is := is.New(t)

		// String properties are passed raw, with no extra quoting.
		callToolOK(t, "add_node", map[string]any{
			"parent_path": ".",
			"node_type":   "Label",
			"properties": map[string]any{
				"name": "MyLabel",
				"text": "Hello World",
			},
		})

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyLabel"},
		})
		nodeProps, _ := props["MyLabel"].(map[string]any)
		is.Equal(nodeProps["text"], "Hello World")
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

		// The resource property is summarized, not dumped.
		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyMesh"},
		})
		nodeProps, _ := props["MyMesh"].(map[string]any)
		is.Equal(nodeProps["mesh"], "Object(SphereMesh)")

		// A colon path in the node path drills into the resource...
		props = callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyMesh:mesh"},
		})
		meshProps, _ := props["MyMesh:mesh"].(map[string]any)
		is.Equal(meshProps["radius"], "2.0")

		// ... or all the way down to a single value.
		props = callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyMesh:mesh:radius"},
		})
		is.Equal(props["MyMesh:mesh:radius"], "2.0")
	})

	t.Run("set_sub_property", func(t *testing.T) {
		is := is.New(t)

		// A colon path in a property name sets a sub-property.
		structured := callToolOK(t, "set_node_properties", map[string]any{
			"action": "Resize sphere",
			"nodes": []any{
				map[string]any{
					"node_path": "MyMesh",
					"properties": map[string]any{
						"mesh:radius": "3.5",
					},
				},
			},
		})
		is.Equal(structured["MyMesh"], true)

		// A colon path in the node path works too.
		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Resize sphere",
			"nodes": []any{
				map[string]any{
					"node_path": "MyMesh:mesh",
					"properties": map[string]any{
						"height": "4.0",
					},
				},
			},
		})

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyMesh:mesh:radius", "MyMesh:mesh:height"},
		})
		is.Equal(props["MyMesh:mesh:radius"], "3.5")
		is.Equal(props["MyMesh:mesh:height"], "4.0")
	})

	t.Run("set_vector_component", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": []any{
				map[string]any{
					"node_path": "MyChild",
					"properties": map[string]any{
						"position:x": "7.0",
					},
				},
			},
		})

		props := callToolOK(t, "get_node_properties", map[string]any{
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

		// Assign a saved resource to a property by reference.
		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Assign label settings",
			"nodes": []any{
				map[string]any{
					"node_path": "MyLabel",
					"properties": map[string]any{
						"label_settings": `Resource("res://resources/np_label_settings.tres")`,
					},
				},
			},
		})

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyLabel", "MyLabel:label_settings:font_size"},
		})
		nodeProps, _ := props["MyLabel"].(map[string]any)
		is.Equal(nodeProps["label_settings"], `Resource("res://resources/np_label_settings.tres")`)
		is.Equal(props["MyLabel:label_settings:font_size"], "32")
	})

	t.Run("invalid_value_changes_nothing", func(t *testing.T) {
		is := is.New(t)

		callToolOK(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": []any{
				map[string]any{
					"node_path":  "MyChild",
					"properties": map[string]any{"position": "Vector2(5, 5)"},
				},
			},
		})

		// One valid and one invalid value: the whole call is rejected, and
		// the valid one must NOT be applied.
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Move node",
			"nodes": []any{
				map[string]any{
					"node_path": "MyChild",
					"properties": map[string]any{
						"position": "Vector2(9, 9)",
						"rotation": "garbage(",
					},
				},
			},
		}, "Cannot parse")

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.Equal(nodeProps["position"], "Vector2(5, 5)")
	})

	t.Run("unknown_property", func(t *testing.T) {
		callToolErr(t, "set_node_properties", map[string]any{
			"action": "Set unknown property",
			"nodes": []any{
				map[string]any{
					"node_path":  "MyChild",
					"properties": map[string]any{"no_such_prop": "1"},
				},
			},
		}, "has no property named 'no_such_prop'")
	})

	t.Run("get_bad_property_path", func(t *testing.T) {
		is := is.New(t)

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild:no_such_prop"},
		})
		errProps, _ := props["MyChild:no_such_prop"].(map[string]any)
		errMsg, _ := errProps["error"].(string)
		is.True(strings.Contains(errMsg, "no property named 'no_such_prop'"))
	})

	t.Run("modified_only", func(t *testing.T) {
		is := is.New(t)

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths":    []string{"MyChild"},
			"modified_only": true,
		})
		nodeProps, _ := props["MyChild"].(map[string]any)

		// "position" was changed by earlier subtests; "rotation" is still at
		// its default value.
		_, hasPosition := nodeProps["position"]
		is.True(hasPosition)
		_, hasRotation := nodeProps["rotation"]
		is.True(!hasRotation)
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

		props := callToolOK(t, "get_node_properties", map[string]any{
			"node_paths": []string{"MyChild"},
		})
		nodeProps, _ := props["MyChild"].(map[string]any)
		is.Equal(len(nodeProps), 0)
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

		groups := callToolOK(t, "get_node_groups", map[string]any{
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

		groups := callToolOK(t, "get_node_groups", map[string]any{
			"node_paths": []string{"MyChild"},
		})
		got := asStrings(groups["MyChild"].([]any))
		is.Equal(got, []string{"enemies"})
	})

	t.Run("add_idempotent", func(t *testing.T) {
		is := is.New(t)

		// Adding a group it's already in is a no-op success.
		structured := callToolOK(t, "add_to_group", map[string]any{
			"node_path": "MyChild",
			"groups":    []string{"enemies"},
		})
		is.Equal(structured["success"], true)

		groups := callToolOK(t, "get_node_groups", map[string]any{
			"node_paths": []string{"MyChild"},
		})
		got := asStrings(groups["MyChild"].([]any))
		is.Equal(got, []string{"enemies"})
	})

	t.Run("get_missing_node", func(t *testing.T) {
		is := is.New(t)
		groups := callToolOK(t, "get_node_groups", map[string]any{
			"node_paths": []string{"NoSuchNode"},
		})
		got, ok := groups["NoSuchNode"].([]any)
		is.True(ok)
		is.Equal(len(got), 0)
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
	// A scene with a source and target node.
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

		// The connection really exists on the node.
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

	// A compatible script (extends Node) and an incompatible one (Node3D).
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

		// The scene tree now reports the attached script.
		tree := callToolOK(t, "get_current_scene_tree", nil)
		children, _ := tree["children"].([]any)
		var host map[string]any
		for _, raw := range children {
			child, _ := raw.(map[string]any)
			if child["name"] == "ScriptHost" {
				host = child
			}
		}
		is.True(host != nil)
		is.Equal(host["script"], "res://scripts/ns_compatible.gd")
	})

	t.Run("attach_incompatible", func(t *testing.T) {
		callToolErr(t, "attach_script", map[string]any{
			"node_path":   "ScriptHost",
			"script_path": "res://scripts/ns_incompatible.gd",
		}, "not compatible")
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
				is.True(!hasScript)
			}
		}
	})

	t.Run("detach_no_script", func(t *testing.T) {
		callToolErr(t, "detach_script", map[string]any{
			"node_path": "ScriptHost",
		}, "has no script attached")
	})
}
