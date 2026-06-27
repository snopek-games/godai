package server

import (
	"errors"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestUserVisibleError(t *testing.T) {
	is := is.New(t)

	// With a wrapped error, Error() appends it and Unwrap() exposes it.
	wrapped := errors.New("boom")
	e := newUserVisibleError("something failed", wrapped, nil)
	is.Equal(e.Error(), "something failed: boom")
	is.Equal(e.Unwrap(), wrapped)
	is.True(errors.Is(e, wrapped)) // errors.Is sees through Unwrap

	// Without a wrapped error, Error() is just the message.
	plain := newUserVisibleError("plain message", nil, nil)
	is.Equal(plain.Error(), "plain message")
	is.True(plain.Unwrap() == nil)
}

func TestUserVisibleErrorMakeToolResult(t *testing.T) {
	is := is.New(t)

	// Without solutions: a single error text block.
	r := newUserVisibleError("nope", nil, nil).makeToolResult()
	is.True(r.IsError)
	is.Equal(len(r.Content), 1)
	is.Equal(r.Content[0].Text, "nope")

	// With solutions: a second block listing each one.
	r = newUserVisibleError("nope", nil, []string{"do X", "do Y"}).makeToolResult()
	is.True(r.IsError)
	is.Equal(len(r.Content), 2)
	is.True(strings.Contains(r.Content[1].Text, "do X"))
	is.True(strings.Contains(r.Content[1].Text, "do Y"))
}
