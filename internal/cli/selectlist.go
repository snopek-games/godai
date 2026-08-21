package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"

	"gitlab.com/snopek-games/godai/internal/cli/output"
	"gitlab.com/snopek-games/godai/internal/core"

	"golang.org/x/term"
)

var errNoRawTerminal = errors.New("terminal can't do raw input")

type listKey int

const (
	keyNone listKey = iota
	keyUp
	keyDown
	keyEnter
	keyCancel
)

// One read can hold several keystrokes (a paste, or fast typing), so the
// buffer is walked sequence by sequence. A 0x1b not opening an arrow-style
// sequence is a bare Esc press.
func parseListKeys(buf []byte) []listKey {
	keys := []listKey{}
	for i := 0; i < len(buf); i++ {
		if buf[i] == 0x1b && i+2 < len(buf) && buf[i+1] == '[' {
			switch buf[i+2] {
			case 'A':
				keys = append(keys, keyUp)
			case 'B':
				keys = append(keys, keyDown)
			}
			i += 2
			continue
		}

		switch buf[i] {
		case '\r', '\n':
			keys = append(keys, keyEnter)
		case 0x03, 0x04, 0x1b:
			keys = append(keys, keyCancel)
		case 'k':
			keys = append(keys, keyUp)
		case 'j':
			keys = append(keys, keyDown)
		}
	}
	return keys
}

type listSelector struct {
	out     io.Writer
	name    string
	options []string
	index   int
	color   bool
}

func (s *listSelector) moveUp() {
	s.index = (s.index + len(s.options) - 1) % len(s.options)
}

func (s *listSelector) moveDown() {
	s.index = (s.index + 1) % len(s.options)
}

// Raw mode stops the terminal translating \n, so every line needs its own \r.
func (s *listSelector) render() {
	for i, option := range s.options {
		line := "    " + option
		if i == s.index {
			line = "  " + output.Paint(s.color, output.Cyan, "> "+option)
		}
		fmt.Fprintf(s.out, "\x1b[2K%s\r\n", line)
	}
}

func (s *listSelector) rewind() {
	fmt.Fprintf(s.out, "\x1b[%dA", len(s.options))
}

func (s *listSelector) clear() {
	fmt.Fprint(s.out, "\x1b[J")
}

func (s *listSelector) finish(chosen string) {
	s.clear()
	fmt.Fprintf(s.out, "  %s %s\r\n", output.Paint(s.color, output.Green, s.name+":"), output.Paint(s.color, output.Cyan, chosen))
}

func selectFromList(name string, options []string, defaultValue string) (string, error) {
	if !output.TermSupportsANSI(os.Stderr) {
		return "", errNoRawTerminal
	}

	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return "", errNoRawTerminal
	}
	defer term.Restore(fd, oldState)

	s := &listSelector{out: os.Stderr, name: name, options: options, color: useColor()}
	if i := slices.Index(options, defaultValue); i >= 0 {
		s.index = i
	}

	fmt.Fprint(s.out, "\x1b[?25l")
	defer fmt.Fprint(s.out, "\x1b[?25h")

	s.render()
	buf := make([]byte, 64)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil {
			s.rewind()
			s.clear()
			return "", core.ErrPromptDeclined
		}

		moved := false
		for _, key := range parseListKeys(buf[:n]) {
			switch key {
			case keyUp:
				s.moveUp()
				moved = true
			case keyDown:
				s.moveDown()
				moved = true
			case keyEnter:
				s.rewind()
				s.finish(s.options[s.index])
				return s.options[s.index], nil
			case keyCancel:
				s.rewind()
				s.clear()
				return "", core.ErrPromptDeclined
			}
		}

		if moved {
			s.rewind()
			s.render()
		}
	}
}
