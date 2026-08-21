package core

import (
	"errors"
	"fmt"
)

var (
	ErrNoEditor              = errors.New("no Godot editor connected")
	ErrNotConfigured         = errors.New("not configured")
	ErrToolFailed            = errors.New("the editor reported an error")
	ErrEditorVersionMismatch = errors.New("the editor that's open is a different version of Godot")
	ErrAddonVersionMismatch  = errors.New("the editor is running a different version of the godai addon")
)

type UserError struct {
	Message   string
	Solutions []string
	Err       error
	ShowErr   bool
}

func NewUserError(message string, err error, solutions []string) *UserError {
	return &UserError{
		Message:   message,
		Solutions: solutions,
		Err:       err,
	}
}

func (e *UserError) Error() string {
	return e.FullMessage()
}

func (e *UserError) FullMessage() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *UserError) Unwrap() error {
	return e.Err
}

func (e *UserError) ShowErrToUser() *UserError {
	e.ShowErr = true
	return e
}

func (e *UserError) UserMessage() string {
	if e.ShowErr {
		return e.FullMessage()
	}
	return e.Message
}
