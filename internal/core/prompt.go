package core

import (
	"context"
	"errors"
)

var (
	ErrPromptUnsupported = errors.New("this front end cannot prompt the user")
	ErrPromptDeclined    = errors.New("the user declined the prompt")
)

type Prompter interface {
	Prompt(ctx context.Context, message string, schema map[string]any) (map[string]any, error)
}

type NoPrompter struct{}

func (NoPrompter) Prompt(context.Context, string, map[string]any) (map[string]any, error) {
	return nil, ErrPromptUnsupported
}
