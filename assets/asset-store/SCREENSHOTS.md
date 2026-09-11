Screenshots
===========

1. In Editor Settings, set the "Main Font Size" and "Code Font Size" to `18`

2. Run this editor script to make the bottom panel exactly 1280x720:

```gdscript
# Resizes the editor's bottom panel to an exact pixel size.
const TARGET_SIZE := Vector2(1280, 720)

func resize_bottom_panel(target_size: Vector2) -> void:
	# Find the EditorBottomPanel node
	var stack: Array[Node] = [EditorInterface.get_base_control()]
	var panel: Control = null
	while stack.size() > 0:
		var n: Node = stack.pop_back()
		if n.get_class() == "EditorBottomPanel":
			panel = n
			break
		for c in n.get_children():
			stack.append(c)
	if panel == null:
		push_error("EditorBottomPanel not found")
		return

	var vsplit: SplitContainer = panel.get_parent()          # controls height
	var hsplit: SplitContainer = vsplit.get_parent().get_parent()  # controls width

	# Iteratively correct split offsets until the panel matches the target size.
	# (Container layout updates are deferred, so we wait a frame between tries.)
	for i in range(10):
		await panel.get_tree().process_frame
		var cur := panel.size
		var err_w := cur.x - target_size.x
		var err_h := cur.y - target_size.y
		if abs(err_w) < 0.5 and abs(err_h) < 0.5:
			break
		hsplit.split_offset += err_w
		vsplit.split_offset += err_h

	await panel.get_tree().process_frame
	print("Bottom panel size: ", panel.size)
```

3. Take the screenshot!

