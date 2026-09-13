@tool
extends AcceptDialog

const ZOOM_LEVELS: Array[float] = [0.125, 0.25, 0.5, 0.75, 1.0, 1.5, 2.0, 3.0, 4.0]
const DIALOG_MARGIN := Vector2i(24, 130)
const SCREEN_RATIO := 0.75
const NEAREST_FROM_ZOOM := 2.0

@onready var scroll_container: ScrollContainer = %ScrollContainer
@onready var texture_rect: TextureRect = %TextureRect
@onready var zoom_label: Label = %ZoomLabel
@onready var save_dialog: FileDialog = %SaveDialog

var zoom := 1.0
var fit_to_view := true

var _image: Image
var _image_size: Vector2


func show_image(p_image: Image) -> void:
	_image = p_image
	var image := p_image.duplicate()
	image.generate_mipmaps()
	texture_rect.texture = ImageTexture.create_from_image(image)
	_image_size = Vector2(image.get_size())
	popup_centered_clamped(Vector2i(_image_size) + DIALOG_MARGIN, SCREEN_RATIO)
	fit()


func fit() -> void:
	fit_to_view = true
	var available := scroll_container.size
	if available.x <= 0 or available.y <= 0 or _image_size.x <= 0 or _image_size.y <= 0:
		return
	_apply_zoom(minf(1.0, minf(available.x / _image_size.x, available.y / _image_size.y)))


func set_zoom(p_zoom: float, p_anchor := Vector2(-1, -1)) -> void:
	fit_to_view = false
	var anchor := p_anchor if p_anchor.x >= 0 else scroll_container.size / 2.0
	var scroll := Vector2(scroll_container.scroll_horizontal, scroll_container.scroll_vertical)
	var image_point := (scroll + anchor) / zoom
	_apply_zoom(p_zoom)
	var new_scroll := image_point * p_zoom - anchor
	scroll_container.set_deferred(&"scroll_horizontal", roundi(new_scroll.x))
	scroll_container.set_deferred(&"scroll_vertical", roundi(new_scroll.y))


func zoom_in(p_anchor := Vector2(-1, -1)) -> void:
	for level in ZOOM_LEVELS:
		if level > zoom + 0.001:
			set_zoom(level, p_anchor)
			return


func zoom_out(p_anchor := Vector2(-1, -1)) -> void:
	for i in range(ZOOM_LEVELS.size() - 1, -1, -1):
		if ZOOM_LEVELS[i] < zoom - 0.001:
			set_zoom(ZOOM_LEVELS[i], p_anchor)
			return


func save_png(p_path: String) -> Error:
	return _image.save_png(p_path)


func _apply_zoom(p_zoom: float) -> void:
	zoom = p_zoom
	texture_rect.custom_minimum_size = (_image_size * zoom).floor()
	if zoom >= NEAREST_FROM_ZOOM:
		texture_rect.texture_filter = CanvasItem.TEXTURE_FILTER_NEAREST
	elif zoom < 1.0:
		texture_rect.texture_filter = CanvasItem.TEXTURE_FILTER_LINEAR_WITH_MIPMAPS
	else:
		texture_rect.texture_filter = CanvasItem.TEXTURE_FILTER_LINEAR
	zoom_label.text = "%d%%" % roundi(zoom * 100)


func _on_fit_button_pressed() -> void:
	fit()


func _on_actual_size_button_pressed() -> void:
	set_zoom(1.0)


func _on_zoom_in_button_pressed() -> void:
	zoom_in()


func _on_zoom_out_button_pressed() -> void:
	zoom_out()


func _on_save_button_pressed() -> void:
	save_dialog.popup_centered_ratio(0.6)


func _on_save_dialog_file_selected(p_path: String) -> void:
	var err := save_png(p_path)
	if err != OK:
		push_error("Unable to save the attachment to '%s': %s" % [p_path, error_string(err)])


func _on_scroll_container_resized() -> void:
	if fit_to_view:
		fit()


func _on_scroll_container_gui_input(p_event: InputEvent) -> void:
	if not (p_event is InputEventMouseButton and p_event.pressed and p_event.ctrl_pressed):
		return
	match p_event.button_index:
		MOUSE_BUTTON_WHEEL_UP:
			zoom_in(p_event.position)
		MOUSE_BUTTON_WHEEL_DOWN:
			zoom_out(p_event.position)
		_:
			return
	scroll_container.accept_event()
