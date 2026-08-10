package core

import (
	"errors"
	"fmt"
)

var (
	ErrNoEditor      = errors.New("no Godot editor connected")
	ErrNotConfigured = errors.New("not configured")
	ErrToolFailed    = errors.New("the editor reported an error")
)

type UserError struct {
	Message   string
	Solutions []string
	Err       error
}

func NewUserError(message string, err error, solutions []string) *UserError {
	return &UserError{
		Message:   message,
		Solutions: solutions,
		Err:       err,
	}
}

func (e *UserError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *UserError) Unwrap() error {
	return e.Err
}
