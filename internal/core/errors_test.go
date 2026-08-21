package core

import (
	"errors"
	"testing"

	"github.com/matryer/is"
)

func TestUserError(t *testing.T) {
	is := is.New(t)

	// With a wrapped error, Error() appends it and Unwrap() exposes it.
	wrapped := errors.New("boom")
	e := NewUserError("something failed", wrapped, nil)
	is.Equal(e.Error(), "something failed: boom")
	is.Equal(e.Unwrap(), wrapped)
	is.True(errors.Is(e, wrapped)) // errors.Is sees through Unwrap

	// Without a wrapped error, Error() is just the message.
	plain := NewUserError("plain message", nil, nil)
	is.Equal(plain.Error(), "plain message")
	is.True(plain.Unwrap() == nil)
}

func TestUserErrorSentinels(t *testing.T) {
	is := is.New(t)

	e := NewUserError("no editor for /p", ErrNoEditor, nil)
	is.True(errors.Is(e, ErrNoEditor))
	is.True(!errors.Is(e, ErrNotConfigured)) // other sentinels don't match
}
