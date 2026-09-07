extends GutTest

const MarkdownToBBCode = preload("res://addons/godai/ui/markdown_to_bbcode.gd")

var _md


func before_each() -> void:
	_md = MarkdownToBBCode.new()
	_md.base_font_size = 10


func _cell(p_bbcode: String) -> String:
	return "[cell border=%s padding=%s]%s[/cell]" % ["#" + _md.border_color.to_html(true), MarkdownToBBCode.TABLE_CELL_PADDING, p_bbcode]


func _code_block(p_bbcode: String) -> String:
	return "[table=1][cell bg=%s border=%s padding=%s][code]%s[/code][/cell][/table]" % [
		"#" + _md.code_block_bg.to_html(true), "#" + _md.border_color.to_html(true), MarkdownToBBCode.CODE_BLOCK_PADDING, p_bbcode]


func _parsed(p_bbcode: String) -> String:
	var label: RichTextLabel = autofree(RichTextLabel.new())
	label.bbcode_enabled = true
	label.text = p_bbcode
	return label.get_parsed_text()


func test_plain_text_and_newlines_pass_through() -> void:
	assert_eq(_md.convert("hello"), "hello")
	assert_eq(_md.convert("a\nb"), "a\nb")
	assert_eq(_md.convert("a\n\nb"), "a\n\nb")
	assert_eq(_md.convert(""), "")
	assert_eq(_md.convert("  indented"), "  indented")


func test_headings() -> void:
	assert_eq(_md.convert("# Title"), "[font_size=16][b]Title[/b][/font_size]")
	assert_eq(_md.convert("## Two"), "[font_size=14][b]Two[/b][/font_size]")
	assert_eq(_md.convert("### Three"), "[font_size=12][b]Three[/b][/font_size]")
	assert_eq(_md.convert("###### Six"), "[font_size=10][b]Six[/b][/font_size]")
	assert_eq(_md.convert("## **bold** heading"), "[font_size=14][b][b]bold[/b] heading[/b][/font_size]")


func test_heading_lookalikes_stay_literal() -> void:
	assert_eq(_md.convert("#hashtag"), "#hashtag")
	assert_eq(_md.convert("#"), "#")
	assert_eq(_md.convert("####### seven"), "####### seven")


func test_emphasis() -> void:
	assert_eq(_md.convert("**b**"), "[b]b[/b]")
	assert_eq(_md.convert("*i*"), "[i]i[/i]")
	assert_eq(_md.convert("_i_"), "[i]i[/i]")
	assert_eq(_md.convert("__u__"), "[u]u[/u]")
	assert_eq(_md.convert("<u>u</u>"), "[u]u[/u]")
	assert_eq(_md.convert("~~s~~"), "[s]s[/s]")
	assert_eq(_md.convert("***bi***"), "[b][i]bi[/i][/b]")
	assert_eq(_md.convert("**b *i* b**"), "[b]b [i]i[/i] b[/b]")
	assert_eq(_md.convert("*i **b** i*"), "[i]i [b]b[/b] i[/i]")
	assert_eq(_md.convert("a **b** c *d* e"), "a [b]b[/b] c [i]d[/i] e")


func test_emphasis_lookalikes_stay_literal() -> void:
	assert_eq(_md.convert("snake_case_name"), "snake_case_name")
	assert_eq(_md.convert("res://my_scene_file.gd"), "res://my_scene_file.gd")
	assert_eq(_md.convert("_leading and trailing_word"), "_leading and trailing_word")
	assert_eq(_md.convert("a * b * c"), "a * b * c")
	assert_eq(_md.convert("2 * 3 = 6"), "2 * 3 = 6")
	assert_eq(_md.convert("**unclosed"), "**unclosed")
	assert_eq(_md.convert("unopened**"), "unopened**")
	assert_eq(_md.convert("**a*"), "*[i]a[/i]")
	assert_eq(_md.convert("~single~"), "~single~")
	assert_eq(_md.convert("<u>unclosed"), "<u>unclosed")
	assert_eq(_md.convert("a < b > c"), "a < b > c")


func test_unicode_text_is_formatted_like_ascii() -> void:
	assert_eq(_md.convert("Powiedz mi o tym scenariuszu"), "Powiedz mi o tym scenariuszu")
	assert_eq(_md.convert("**Scenariusz** zawiera `węzeł` i _ścieżkę_ do [dokumentacji](https://godai.sh/pl)"), "[b]Scenariusz[/b] zawiera [code]węzeł[/code] i [i]ścieżkę[/i] do [url=https://godai.sh/pl]dokumentacji[/url]")
	assert_eq(_md.convert("## Główne węzły\n- Gracz\n  - Wróg"), "[font_size=14][b]Główne węzły[/b][/font_size]\n[ul]Gracz[ul]Wróg[/ul][/ul]")
	assert_eq(_md.convert("| Węzeł | Opis |\n|---|---|\n| Gracz | Ktoś |"), "[table=2]%s%s%s%s[/table]" % [_cell("[b]Węzeł[/b]"), _cell("[b]Opis[/b]"), _cell("Gracz"), _cell("Ktoś")])
	assert_eq(_md.convert("**🎮 gra** i `日本語` oraz *Ελληνικά*"), "[b]🎮 gra[/b] i [code]日本語[/code] oraz [i]Ελληνικά[/i]")
	assert_eq(_md.convert("Zażółć [b]gęślą[/b] jaźń"), "Zażółć [lb]b]gęślą[lb]/b] jaźń")


func test_unicode_words_keep_intraword_underscores() -> void:
	assert_eq(_md.convert("zmienna_żółw_ptak"), "zmienna_żółw_ptak")
	assert_eq(_md.convert("ź_ó_ź"), "ź_ó_ź")
	assert_eq(_md.convert("日本_語_テスト"), "日本_語_テスト")
	assert_eq(_md.convert("ę _kursywa_ ą"), "ę [i]kursywa[/i] ą")


func test_backslash_escapes() -> void:
	assert_eq(_md.convert("\\*not italic\\*"), "*not italic*")
	assert_eq(_md.convert("\\["), "[lb]")
	assert_eq(_md.convert("trailing\\"), "trailing\\")


func test_inline_code() -> void:
	assert_eq(_md.convert("`x`"), "[code]x[/code]")
	assert_eq(_md.convert("use `**not bold**` here"), "use [code]**not bold**[/code] here")
	assert_eq(_md.convert("``a ` b``"), "[code]a ` b[/code]")
	assert_eq(_md.convert("`unclosed"), "`unclosed")
	assert_eq(_md.convert("`a``"), "`a``")


func test_fenced_code_block() -> void:
	assert_eq(_md.convert("```gdscript\nvar x = [1]\n# not a heading\n```"), _code_block("var x = [lb]1]\n# not a heading"))
	assert_eq(_md.convert("~~~\n**raw**\n~~~"), _code_block("**raw**"))
	assert_eq(_md.convert("before\n```\ncode\n```\nafter"), "before\n%s\nafter" % _code_block("code"))


func test_unterminated_fence_keeps_everything() -> void:
	assert_eq(_md.convert("```\nstill code\nmore"), _code_block("still code\nmore"))


func test_lists() -> void:
	assert_eq(_md.convert("- a\n- b"), "[ul]a\nb[/ul]")
	assert_eq(_md.convert("* a\n+ b"), "[ul]a\nb[/ul]")
	assert_eq(_md.convert("1. a\n2. b"), "[ol]a\nb[/ol]")
	assert_eq(_md.convert("1) a\n10) b"), "[ol]a\nb[/ol]")
	assert_eq(_md.convert("- **a**\n- `b`"), "[ul][b]a[/b]\n\u200b[code]b[/code][/ul]")
	assert_eq(_md.convert("- a\n  - nested\n- b"), "[ul]a[ul]nested[/ul]\nb[/ul]")
	assert_eq(_md.convert("- a\n  1. n1\n     - deep\n  2. n2\n- b"), "[ul]a[ol]n1[ul]deep[/ul]\nn2[/ol]\nb[/ul]")
	assert_eq(_md.convert("- a\n\t- tabbed"), "[ul]a[ul]tabbed[/ul][/ul]")
	assert_eq(_md.convert("- a\n    - four\n  - two\n- b"), "[ul]a[ul]four[/ul][ul]two[/ul]\nb[/ul]")
	assert_eq(_md.convert("  - indented start\n- top"), "[ul]indented start[/ul]\n[ul]top[/ul]")
	assert_eq(_md.convert("- a\n\ntext"), "[ul]a[/ul]\n\ntext")
	assert_eq(_md.convert("- a\n1. b\n- c"), "[ul]a[/ul]\n[ol]b[/ol]\n[ul]c[/ul]")
	assert_eq(_md.convert("- [ ] task"), "[ul][lb] ] task[/ul]")


func test_loose_lists_stay_one_list() -> void:
	assert_eq(_md.convert("1. a\n\n2. b\n\n\n3. c"), "[ol]a\nb\nc[/ol]")
	assert_eq(_md.convert("1. a\n  - n\n\n2. b\n  - m"), "[ol]a[ul]n[/ul]\nb[ul]m[/ul][/ol]")
	assert_eq(_md.convert("- a\n\n- b\n\ntext"), "[ul]a\nb[/ul]\n\ntext")
	assert_eq(_md.convert("- a\n\n"), "[ul]a[/ul]\n\n")


func test_list_items_starting_with_code_get_a_normal_font_bullet() -> void:
	assert_eq(_md.convert("- `x` first\n- then `y`"), "[ul]\u200b[code]x[/code] first\nthen [code]y[/code][/ul]")
	assert_eq(_md.convert("1. `x`"), "[ol]\u200b[code]x[/code][/ol]")


func test_list_lookalikes_stay_literal() -> void:
	assert_eq(_md.convert("-1 is negative"), "-1 is negative")
	assert_eq(_md.convert("1.5 is a float"), "1.5 is a float")
	assert_eq(_md.convert("-"), "-")
	assert_eq(_md.convert("- "), "- ")


func test_horizontal_rules() -> void:
	assert_eq(_md.convert("---"), "[hr]")
	assert_eq(_md.convert("* * *"), "[hr]")
	assert_eq(_md.convert("_____"), "[hr]")
	assert_eq(_md.convert("--"), "--")
	assert_eq(_md.convert("-*-"), "-*-")


func test_tables() -> void:
	var out: String = _md.convert("| a | b |\n|---|:-:|\n| 1 | 2 |")
	assert_eq(out, "[table=2]%s%s%s%s[/table]" % [_cell("[b]a[/b]"), _cell("[b]b[/b]"), _cell("1"), _cell("2")])
	out = _md.convert("a | b\n--- | ---\n1 | 2")
	assert_eq(out, "[table=2]%s%s%s%s[/table]" % [_cell("[b]a[/b]"), _cell("[b]b[/b]"), _cell("1"), _cell("2")])


func test_ragged_tables_keep_all_text() -> void:
	var out: String = _md.convert("| a | b |\n|---|---|\n| 1 |\n| 2 | 3 | 4 |\nafter")
	assert_eq(out, "[table=2]%s%s%s%s%s%s[/table]\nafter" % [
		_cell("[b]a[/b]"), _cell("[b]b[/b]"), _cell("1"), _cell(""), _cell("2"), _cell("3 | 4")])


func test_table_lookalikes_stay_literal() -> void:
	assert_eq(_md.convert("a | b\nnot a delimiter"), "a | b\nnot a delimiter")
	assert_eq(_md.convert("| only |"), "| only |")


func test_links() -> void:
	assert_eq(_md.convert("[x](https://a.b)"), "[url=https://a.b]x[/url]")
	assert_eq(_md.convert("[x](http://a.b/p?q=1&r=2)"), "[url=http://a.b/p?q=1&r=2]x[/url]")
	assert_eq(_md.convert("[x](HTTPS://A.B)"), "[url=HTTPS://A.B]x[/url]")
	assert_eq(_md.convert("[**x**](https://a.b)"), "[url=https://a.b][b]x[/b][/url]")
	assert_eq(_md.convert("<https://a.b>"), "[url]https://a.b[/url]")
	assert_eq(_md.convert("see [x](https://a.b) now"), "see [url=https://a.b]x[/url] now")


func test_non_http_links_are_shown_but_not_clickable() -> void:
	for url in ["javascript:alert(1)", "file:///etc/passwd", "res://x.gd", "mailto:a@b.c", "ftp://a.b", "//a.b", "docs/x.md", "https:/a.b", "http:a.b"]:
		var out: String = _md.convert("[x](%s)" % url)
		assert_false(out.contains("[url"), "%s must not become a link" % url)
		assert_eq(_parsed(out), "[x](%s)" % url, "%s must still be visible" % url)
		out = _md.convert("<%s>" % url)
		assert_false(out.contains("[url"), "<%s> must not become a link" % url)
		assert_eq(_parsed(out), "<%s>" % url, "<%s> must still be visible" % url)


func test_links_with_tag_breaking_urls_are_rejected() -> void:
	for url in ["https://a.b/x]y", "https://a.b/[b]", "https://a b", "https://a.b/\"x"]:
		var out: String = _md.convert("[x](%s)" % url)
		assert_false(out.contains("[url"), "%s must not become a link" % url)
		assert_eq(_parsed(out), "[x](%s)" % url, "%s must still be visible" % url)


func test_link_lookalikes_stay_literal() -> void:
	assert_eq(_md.convert("[x]"), "[lb]x]")
	assert_eq(_md.convert("[x](unclosed"), "[lb]x](unclosed")
	assert_eq(_md.convert("[x] (https://a.b)"), "[lb]x] (https://a.b)")
	assert_eq(_md.convert("array[0]"), "array[lb]0]")


func test_bbcode_in_input_is_escaped() -> void:
	assert_eq(_md.convert("[b]x[/b]"), "[lb]b]x[lb]/b]")
	assert_eq(_md.convert("[url=https://x]y[/url]"), "[lb]url=https://x]y[lb]/url]")
	assert_eq(_md.convert("[img]res://icon.png[/img]"), "[lb]img]res://icon.png[lb]/img]")
	assert_eq(_md.convert("[color=red]x[/color]"), "[lb]color=red]x[lb]/color]")
	assert_eq(_md.convert("[lb]"), "[lb]lb]")
	assert_eq(_md.convert("[rb]"), "[lb]rb]")
	assert_eq(_md.convert("[[b]]"), "[lb][lb]b]]")


func test_bbcode_in_input_is_escaped_inside_every_construct() -> void:
	assert_eq(_md.convert("`[b]x[/b]`"), "[code][lb]b]x[lb]/b][/code]")
	assert_eq(_md.convert("```\n[b]x[/b]\n```"), _code_block("[lb]b]x[lb]/b]"))
	assert_eq(_md.convert("# [b]x[/b]"), "[font_size=16][b][lb]b]x[lb]/b][/b][/font_size]")
	assert_eq(_md.convert("- [b]x[/b]"), "[ul][lb]b]x[lb]/b][/ul]")
	assert_eq(_md.convert("**[b]x[/b]**"), "[b][lb]b]x[lb]/b][/b]")
	assert_eq(_md.convert("[[b]x](https://a.b)"), "[url=https://a.b][lb]b]x[/url]")
	assert_eq(_md.convert("| [b]x |\n|---|\n| [/b] |"), "[table=1]%s%s[/table]" % [_cell("[b][lb]b]x[/b]"), _cell("[lb]/b]")])


func test_rendered_output_shows_input_bbcode_literally() -> void:
	assert_eq(_parsed(_md.convert("[b]x[/b] and **bold**")), "[b]x[/b] and bold")
	assert_eq(_parsed(_md.convert("[color=red]red[/color] `[code]`")), "[color=red]red[/color] [code]")
	assert_eq(_parsed(_md.convert("[img]res://icon.png[/img]")), "[img]res://icon.png[/img]")


func test_only_converter_tags_appear_in_output() -> void:
	var inputs := ["[b]x[/b]", "**[i]y[/i]**", "`[u]`", "# [s]", "- [color=red]", "| [a] |\n|--|\n| [b] |", "[x](https://a.b/[c])", "```\n[lb]\n```"]
	var converter_tags := ["[lb]", "[b]", "[/b]", "[i]", "[/i]", "[u]", "[/u]", "[s]", "[/s]", "[code]", "[/code]", "[url=", "[url]", "[/url]", "[font_size=", "[/font_size]", "[ul]", "[/ul]", "[ol]", "[/ol]", "[hr]", "[table=", "[/table]", "[cell ", "[/cell]"]
	for input in inputs:
		var out: String = _md.convert(input)
		var at: int = out.find("[")
		while at != -1:
			var recognized := false
			for tag in converter_tags:
				if out.substr(at).begins_with(tag):
					recognized = true
			assert_true(recognized, "unexpected tag at %d in %s" % [at, out])
			at = out.find("[", at + 1)


func test_nothing_is_lost() -> void:
	var cases := {
		"**bold** and *italic* and `code`": "bold and italic and code",
		"# Heading\ntext": "Heading\ntext",
		"[link](https://a.b)": "link",
		"[link](javascript:x)": "[link](javascript:x)",
		"- a\n- b": "a\nb",
		"---": "",
		"```\nx = [1]\n```": "x = [1]",
		"| a |\n|---|\n| b |": "ab",
		"**unclosed *mess `here": "**unclosed *mess `here",
		"[b]not bold[/b]": "[b]not bold[/b]",
		"**Zażółć** `gęślą` _jaźń_ 🎮 日本語": "Zażółć gęślą jaźń 🎮 日本語",
	}
	for input in cases:
		var plain: String = _parsed(_md.convert(input)).replace("\t", "").strip_edges()
		assert_eq(plain, cases[input], "for input %s" % input)
