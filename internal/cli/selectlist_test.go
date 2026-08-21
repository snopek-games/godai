package cli

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestParseListKeys(t *testing.T) {
	is := is.New(t)

	is.Equal(parseListKeys([]byte{0x1b, '[', 'A'}), []listKey{keyUp})
	is.Equal(parseListKeys([]byte{0x1b, '[', 'B'}), []listKey{keyDown})
	is.Equal(parseListKeys([]byte{'k'}), []listKey{keyUp})
	is.Equal(parseListKeys([]byte{'j'}), []listKey{keyDown})
	is.Equal(parseListKeys([]byte{'\r'}), []listKey{keyEnter})
	is.Equal(parseListKeys([]byte{'\n'}), []listKey{keyEnter})
	is.Equal(parseListKeys([]byte{0x03}), []listKey{keyCancel})
	is.Equal(parseListKeys([]byte{0x04}), []listKey{keyCancel})
	is.Equal(parseListKeys([]byte{0x1b}), []listKey{keyCancel})
	is.Equal(parseListKeys([]byte{'x'}), []listKey{}) // unrecognized bytes are ignored
	is.Equal(parseListKeys([]byte{0x1b, '[', 'C'}), []listKey{})
}

func TestParseListKeysSplitsARunOfKeystrokes(t *testing.T) {
	is := is.New(t)

	is.Equal(parseListKeys([]byte("\x1b[B\r")), []listKey{keyDown, keyEnter})
	is.Equal(parseListKeys([]byte("\x1b[B\x1b[B\x1b[A")), []listKey{keyDown, keyDown, keyUp})
	is.Equal(parseListKeys([]byte("jjk\r")), []listKey{keyDown, keyDown, keyUp, keyEnter})
}

func TestListSelectorWrapsAround(t *testing.T) {
	is := is.New(t)

	s := &listSelector{options: []string{"a", "b", "c"}}
	s.moveUp()
	is.Equal(s.index, 2)
	s.moveDown()
	is.Equal(s.index, 0)
	s.moveDown()
	is.Equal(s.index, 1)
}

func TestListSelectorRenderMarksTheSelection(t *testing.T) {
	is := is.New(t)

	var buf strings.Builder
	s := &listSelector{out: &buf, options: []string{"a", "b"}, index: 1}
	s.render()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\r\n"), "\r\n")
	is.Equal(len(lines), 2)
	is.True(strings.Contains(lines[0], "    a"))
	is.True(strings.Contains(lines[1], "  > b"))
	is.True(!strings.Contains(buf.String(), "\x1b[36m")) // no color codes when color is off

	buf.Reset()
	s.color = true
	s.render()
	is.True(strings.Contains(buf.String(), "\x1b[36m> b\x1b[0m")) // the selection is cyan
}

func TestListSelectorFinishShowsTheChoice(t *testing.T) {
	is := is.New(t)

	var buf strings.Builder
	s := &listSelector{out: &buf, name: "client", options: []string{"a"}}
	s.finish("a")
	is.True(strings.Contains(buf.String(), "  client: a\r\n"))
}
