package mcp

import (
	"strings"
	"testing"

	"gitlab.com/snopek-games/godai/internal/core"

	"github.com/matryer/is"
)

func TestToolResultForError(t *testing.T) {
	is := is.New(t)

	// Without solutions: a single error text block.
	r := toolResultForError(core.NewUserError("nope", nil, nil))
	is.True(r.IsError)
	is.Equal(len(r.Content), 1)
	is.Equal(r.Content[0].Text, "nope")

	// With solutions: a second block listing each one.
	r = toolResultForError(core.NewUserError("nope", nil, []string{"do X", "do Y"}))
	is.True(r.IsError)
	is.Equal(len(r.Content), 2)
	is.True(strings.Contains(r.Content[1].Text, "do X"))
	is.True(strings.Contains(r.Content[1].Text, "do Y"))
}
