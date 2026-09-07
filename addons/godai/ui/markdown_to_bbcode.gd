extends RefCounted

## This Markdown to BBCode parser avoids regexes and prioritizes not losing/hiding
## content over everything else (even if that means formatting doesn't look as pretty).
##
## It is loosely based on Appendix A of the CommonMark Spec:
##
##   https://spec.commonmark.org/0.31.2/#appendix-a-parsing-strategy
##
## It also tries to:
##
## - Ensure URLs are safe for `OS.open_shell()`
## - Escape any BBCode in the input, so it won't be rendered by RichTextLabel.

const HEADING_SCALES := [1.6, 1.4, 1.2, 1.1, 1.0, 1.0]
const CODE_BLOCK_PADDING := "8,6,8,6"
const TABLE_CELL_PADDING := "6,2,6,2"
const FENCE_MARKERS := ["```", "~~~"]
const DIGITS := "0123456789"
const ZERO_WIDTH_SPACE := "\u200b"

var base_font_size := 14
var code_block_bg := Color(1, 1, 1, 0.08)
var border_color := Color(1, 1, 1, 0.2)

var _lines: PackedStringArray
var _line := 0
var _out: PackedStringArray


func configure_from(p_label: RichTextLabel) -> void:
	base_font_size = p_label.get_theme_font_size("normal_font_size")
	var text_color := p_label.get_theme_color("default_color")
	code_block_bg = Color(text_color, 0.08)
	border_color = Color(text_color, 0.2)


func convert(p_markdown: String) -> String:
	_lines = p_markdown.split("\n")
	_line = 0
	_out = PackedStringArray()
	while _line < _lines.size():
		if not (_try_code_fence() or _try_heading() or _try_table() or _try_hr() or _try_list()):
			_out.append(_inline(_lines[_line]))
			_line += 1
	return "\n".join(_out)


func _try_code_fence() -> bool:
	var marker := _fence_marker(_lines[_line])
	if marker.is_empty():
		return false
	_line += 1
	var code := PackedStringArray()
	while _line < _lines.size() and _fence_marker(_lines[_line]) != marker:
		code.append(_escape(_lines[_line]))
		_line += 1
	_line += 1
	_out.append("[table=1][cell bg=%s border=%s padding=%s][code]%s[/code][/cell][/table]" % [
		_html_color(code_block_bg), _html_color(border_color), CODE_BLOCK_PADDING, "\n".join(code)])
	return true


func _fence_marker(p_line: String) -> String:
	var stripped := p_line.strip_edges(true, false)
	for marker in FENCE_MARKERS:
		if stripped.begins_with(marker):
			return marker
	return ""


func _try_heading() -> bool:
	var line := _lines[_line]
	var level := 0
	while level < line.length() and line[level] == "#":
		level += 1
	if level == 0 or level > HEADING_SCALES.size() or level >= line.length() or line[level] != " ":
		return false
	var size := int(round(base_font_size * HEADING_SCALES[level - 1]))
	_out.append("[font_size=%d][b]%s[/b][/font_size]" % [size, _inline(line.substr(level).strip_edges())])
	_line += 1
	return true


func _try_hr() -> bool:
	var compact := _lines[_line].replace(" ", "")
	if compact.length() < 3 or not "-*_".contains(compact[0]):
		return false
	for ch in compact:
		if ch != compact[0]:
			return false
	_out.append("[hr]")
	_line += 1
	return true


func _try_table() -> bool:
	if _line + 1 >= _lines.size() or not _lines[_line].contains("|") or not _is_table_delimiter_row(_lines[_line + 1]):
		return false
	var header := _split_table_row(_lines[_line])
	var columns := header.size()
	var cells := PackedStringArray()
	for cell in header:
		cells.append(_table_cell("[b]%s[/b]" % _inline(cell)))
	_line += 2
	while _line < _lines.size() and _lines[_line].contains("|"):
		var row := _split_table_row(_lines[_line])
		if row.size() > columns:
			row[columns - 1] = " | ".join(row.slice(columns - 1))
		for i in columns:
			cells.append(_table_cell(_inline(row[i]) if i < row.size() else ""))
		_line += 1
	_out.append("[table=%d]%s[/table]" % [columns, "".join(cells)])
	return true


func _table_cell(p_bbcode: String) -> String:
	return "[cell border=%s padding=%s]%s[/cell]" % [_html_color(border_color), TABLE_CELL_PADDING, p_bbcode]


func _is_table_delimiter_row(p_line: String) -> bool:
	var cells := _split_table_row(p_line)
	if cells.is_empty():
		return false
	for cell in cells:
		var dashes: String = cell.trim_prefix(":").trim_suffix(":")
		if dashes.is_empty() or dashes.count("-") != dashes.length():
			return false
	return true


func _split_table_row(p_line: String) -> PackedStringArray:
	var row := p_line.strip_edges().trim_prefix("|").trim_suffix("|")
	var cells := PackedStringArray()
	for cell in row.split("|"):
		cells.append(cell.strip_edges())
	return cells


func _try_list() -> bool:
	if _list_item_text_start(_lines[_line]) < 0:
		return false
	var items := []
	while _line < _lines.size():
		var line := _lines[_line]
		var start := _list_item_text_start(line)
		if start < 0:
			if not line.strip_edges().is_empty():
				break
			var next_item := _next_non_blank_line(_line)
			if next_item < 0 or _list_item_text_start(_lines[next_item]) < 0:
				break
			_line = next_item
			continue
		items.append({
			indent = line.length() - line.strip_edges(true, false).length(),
			tag = _list_tag(line),
			text = _list_item_bbcode(line.substr(start).strip_edges()),
		})
		_line += 1
	var lists := PackedStringArray()
	var i := 0
	while i < items.size():
		var list := _list_bbcode(items, i)
		lists.append(list[0])
		i = list[1]
	_out.append("\n".join(lists))
	return true


func _next_non_blank_line(p_from: int) -> int:
	var i := p_from
	while i < _lines.size() and _lines[i].strip_edges().is_empty():
		i += 1
	return i if i < _lines.size() else -1


func _list_item_bbcode(p_text: String) -> String:
	var bbcode := _inline(p_text)
	# If inline code is the first thing in a bullet point, it'll mess up the font
	# used for the bullet, so we inject a zero-width space to prevent that.
	if bbcode.begins_with("[code]"):
		return ZERO_WIDTH_SPACE + bbcode
	return bbcode


func _list_bbcode(p_items: Array, p_from: int) -> Array:
	var indent: int = p_items[p_from].indent
	var tag: String = p_items[p_from].tag
	var lines := PackedStringArray()
	var i := p_from
	while i < p_items.size() and p_items[i].indent == indent and p_items[i].tag == tag:
		var item: String = p_items[i].text
		i += 1
		while i < p_items.size() and p_items[i].indent > indent:
			var nested := _list_bbcode(p_items, i)
			item += nested[0]
			i = nested[1]
		lines.append(item)
	return ["[%s]%s[/%s]" % [tag, "\n".join(lines), tag], i]


func _list_tag(p_line: String) -> String:
	return "ol" if DIGITS.contains(p_line.strip_edges().left(1)) else "ul"


func _list_item_text_start(p_line: String) -> int:
	var i := 0
	while i < p_line.length() and " \t".contains(p_line[i]):
		i += 1
	if i < p_line.length() and "-*+".contains(p_line[i]):
		i += 1
	else:
		var digits_start := i
		while i < p_line.length() and DIGITS.contains(p_line[i]):
			i += 1
		if i == digits_start or i >= p_line.length() or not ".)".contains(p_line[i]):
			return -1
		i += 1
	if i >= p_line.length() or p_line[i] != " " or p_line.substr(i).strip_edges().is_empty():
		return -1
	return i + 1


func _inline(p_text: String) -> String:
	var out := ""
	var i := 0
	while i < p_text.length():
		var m := _match_inline(p_text, i)
		if m.is_empty():
			out += _escape(p_text[i])
			i += 1
		else:
			out += m[0]
			i = m[1]
	return out


func _match_inline(p_text: String, p_at: int) -> Array:
	match p_text[p_at]:
		"`":
			return _match_code_span(p_text, p_at)
		"*":
			return _match_emphasis(p_text, p_at, "*", "b")
		"_":
			return _match_emphasis(p_text, p_at, "_", "u")
		"~":
			if _char_run_at(p_text, p_at) == "~~":
				return _match_delimited(p_text, p_at, "~~", "[s]", "[/s]")
		"[":
			return _match_link(p_text, p_at)
		"<":
			return _match_angle_bracket(p_text, p_at)
		"\\":
			if p_at + 1 < p_text.length():
				return [_escape(p_text[p_at + 1]), p_at + 2]
	return []


func _match_code_span(p_text: String, p_at: int) -> Array:
	var ticks := _char_run_at(p_text, p_at)
	var close := _find_exact_char_run(p_text, ticks, p_at + ticks.length())
	if close == -1:
		return []
	var code := p_text.substr(p_at + ticks.length(), close - p_at - ticks.length())
	return ["[code]%s[/code]" % _escape(code), close + ticks.length()]


func _match_emphasis(p_text: String, p_at: int, p_char: String, p_double_tag: String) -> Array:
	var run := _char_run_at(p_text, p_at)
	match run.length():
		1:
			return _match_delimited(p_text, p_at, run, "[i]", "[/i]")
		2:
			return _match_delimited(p_text, p_at, run, "[%s]" % p_double_tag, "[/%s]" % p_double_tag)
		3:
			return _match_delimited(p_text, p_at, run, "[%s][i]" % p_double_tag, "[/i][/%s]" % p_double_tag)
	return []


func _match_delimited(p_text: String, p_at: int, p_delim: String, p_open: String, p_close: String) -> Array:
	var inner_start := p_at + p_delim.length()
	if inner_start >= p_text.length() or p_text[inner_start] == " ":
		return []
	var underscore := p_delim[0] == "_"
	if underscore and p_at > 0 and _is_word_char(p_text[p_at - 1]):
		return []
	var close := _find_exact_char_run(p_text, p_delim, inner_start + 1)
	while close != -1:
		var after := close + p_delim.length()
		var closes_word := underscore and after < p_text.length() and _is_word_char(p_text[after])
		if p_text[close - 1] != " " and not closes_word:
			break
		close = _find_exact_char_run(p_text, p_delim, close + 1)
	if close == -1:
		return []
	var inner := p_text.substr(inner_start, close - inner_start)
	return [p_open + _inline(inner) + p_close, close + p_delim.length()]


func _match_link(p_text: String, p_at: int) -> Array:
	var text_end := p_text.find("](", p_at)
	if text_end == -1:
		return []
	var url_end := p_text.find(")", text_end + 2)
	if url_end == -1:
		return []
	var url := p_text.substr(text_end + 2, url_end - text_end - 2)
	if not _is_safe_link(url):
		return []
	var link_text := p_text.substr(p_at + 1, text_end - p_at - 1)
	return ["[url=%s]%s[/url]" % [url, _inline(link_text)], url_end + 1]


func _match_angle_bracket(p_text: String, p_at: int) -> Array:
	if p_text.substr(p_at, 3) == "<u>":
		var close := p_text.find("</u>", p_at + 3)
		if close == -1:
			return []
		return ["[u]%s[/u]" % _inline(p_text.substr(p_at + 3, close - p_at - 3)), close + 4]
	var close := p_text.find(">", p_at)
	if close == -1:
		return []
	var url := p_text.substr(p_at + 1, close - p_at - 1)
	if not _is_safe_link(url):
		return []
	return ["[url]%s[/url]" % url, close + 1]


func _is_safe_link(p_url: String) -> bool:
	var lower := p_url.to_lower()
	if not (lower.begins_with("http://") or lower.begins_with("https://")):
		return false
	for ch in "[] \t\"":
		if p_url.contains(ch):
			return false
	return true


func _is_word_char(p_char: String) -> bool:
	return DIGITS.contains(p_char) or p_char.is_valid_unicode_identifier()


# The longest stretch of one repeated character starting at p_at, e.g. "***".
func _char_run_at(p_text: String, p_at: int) -> String:
	var end := p_at
	while end < p_text.length() and p_text[end] == p_text[p_at]:
		end += 1
	return p_text.substr(p_at, end - p_at)


# Next occurrence of p_run from p_from that isn't part of a longer run, or -1.
func _find_exact_char_run(p_text: String, p_run: String, p_from: int) -> int:
	var at := p_text.find(p_run, p_from)
	while at != -1:
		var after := at + p_run.length()
		var before_ok := at == 0 or p_text[at - 1] != p_run[0]
		var after_ok := after >= p_text.length() or p_text[after] != p_run[0]
		if before_ok and after_ok:
			return at
		at = p_text.find(p_run, at + 1)
	return -1


func _escape(p_text: String) -> String:
	return p_text.replace("[", "[lb]")


func _html_color(p_color: Color) -> String:
	return "#" + p_color.to_html(true)
