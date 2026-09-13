extends GutTest

const AttachmentViewDialogScene = preload("res://addons/godai/ui/attachment_view_dialog.tscn")

const DIALOG_SIZE := Vector2i(1000, 700)

var _dialog


func before_each() -> void:
	_dialog = AttachmentViewDialogScene.instantiate()
	add_child_autofree(_dialog)


# The headless root window is tiny, so the popup can't size itself from the
# screen; sizing the dialog by hand takes the same re-fit path a user resize does.
func _show(p_width: int, p_height: int) -> void:
	_dialog.show_image(Image.create(p_width, p_height, false, Image.FORMAT_RGB8))
	_dialog.size = DIALOG_SIZE
	await wait_process_frames(2)


func test_opens_fitted_to_the_view() -> void:
	await _show(4000, 2000)

	var view: Vector2 = _dialog.scroll_container.size
	assert_true(view.x > 0 and view.y > 0, "the view has been laid out")
	assert_true(_dialog.fit_to_view)
	assert_almost_eq(_dialog.zoom, minf(view.x / 4000.0, view.y / 2000.0), 0.0001)
	assert_true(_dialog.texture_rect.custom_minimum_size.x <= view.x)
	assert_true(_dialog.texture_rect.custom_minimum_size.y <= view.y)
	assert_eq(_dialog.texture_rect.texture_filter, CanvasItem.TEXTURE_FILTER_LINEAR_WITH_MIPMAPS)


func test_fit_never_enlarges_a_small_image() -> void:
	await _show(16, 8)

	assert_eq(_dialog.zoom, 1.0)
	assert_eq(_dialog.zoom_label.text, "100%")
	assert_eq(_dialog.texture_rect.texture_filter, CanvasItem.TEXTURE_FILTER_LINEAR)


func test_zoom_steps_through_the_levels_from_any_zoom() -> void:
	await _show(16, 8)
	_dialog.set_zoom(0.6)

	_dialog.zoom_in()
	assert_eq(_dialog.zoom, 0.75)
	_dialog.zoom_in()
	assert_eq(_dialog.zoom, 1.0)
	_dialog.zoom_out()
	_dialog.zoom_out()
	assert_eq(_dialog.zoom, 0.5)

	for i in 20:
		_dialog.zoom_in()
	assert_eq(_dialog.zoom, 4.0)
	assert_eq(_dialog.zoom_label.text, "400%")
	assert_eq(_dialog.texture_rect.custom_minimum_size, Vector2(64, 32))
	assert_eq(_dialog.texture_rect.texture_filter, CanvasItem.TEXTURE_FILTER_NEAREST)
	assert_false(_dialog.fit_to_view)


func test_manual_zoom_survives_a_resize_but_fit_follows_it() -> void:
	await _show(4000, 2000)
	var fitted: float = _dialog.zoom

	_dialog.set_zoom(1.0)
	_dialog.size += Vector2i(100, 100)
	await wait_process_frames(2)
	assert_eq(_dialog.zoom, 1.0)

	_dialog.fit()
	await wait_process_frames(2)
	assert_true(_dialog.fit_to_view)
	assert_true(_dialog.zoom > fitted, "a bigger view fits a bigger image")


func test_ctrl_wheel_zooms_around_the_cursor() -> void:
	await _show(4000, 2000)
	_dialog.set_zoom(1.0)
	await wait_process_frames(2)
	_dialog.scroll_container.scroll_horizontal = 1000
	_dialog.scroll_container.scroll_vertical = 500
	await wait_process_frames(1)

	var event := InputEventMouseButton.new()
	event.button_index = MOUSE_BUTTON_WHEEL_UP
	event.pressed = true
	event.ctrl_pressed = true
	event.position = Vector2(100, 50)
	_dialog.scroll_container.gui_input.emit(event)
	await wait_process_frames(2)

	assert_eq(_dialog.zoom, 1.5)
	assert_eq(_dialog.scroll_container.scroll_horizontal, roundi((1000 + 100) * 1.5 - 100))
	assert_eq(_dialog.scroll_container.scroll_vertical, roundi((500 + 50) * 1.5 - 50))

	event.ctrl_pressed = false
	_dialog.scroll_container.gui_input.emit(event)
	assert_eq(_dialog.zoom, 1.5, "a plain wheel scrolls instead of zooming")


func test_buttons_are_wired_through_the_scene() -> void:
	await _show(16, 8)

	_dialog.get_node("%ZoomInButton").pressed.emit()
	assert_eq(_dialog.zoom, 1.5)
	_dialog.get_node("%ZoomOutButton").pressed.emit()
	assert_eq(_dialog.zoom, 1.0)
	_dialog.get_node("%ZoomOutButton").pressed.emit()
	_dialog.get_node("%ActualSizeButton").pressed.emit()
	assert_eq(_dialog.zoom, 1.0)
	assert_false(_dialog.fit_to_view)
	_dialog.get_node("%FitButton").pressed.emit()
	assert_true(_dialog.fit_to_view)


func test_save_writes_the_original_image_as_png() -> void:
	var image := Image.create(16, 8, false, Image.FORMAT_RGB8)
	image.fill(Color.REBECCA_PURPLE)
	_dialog.show_image(image)
	var path := OS.get_cache_dir().path_join("godai-test-attachment-%d.png" % Time.get_ticks_usec())

	_dialog.save_dialog.file_selected.emit(path)

	var saved := Image.load_from_file(path)
	assert_not_null(saved)
	assert_eq(saved.get_size(), Vector2i(16, 8))
	assert_eq(saved.get_pixel(3, 3), Color.REBECCA_PURPLE)
	assert_false(saved.has_mipmaps(), "the saved file is the original, not the mipmapped display copy")
	DirAccess.remove_absolute(path)
