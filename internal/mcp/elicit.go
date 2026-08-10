package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"gitlab.com/snopek-games/godai/internal/core"
)

type elicitPrompter struct {
	server *Server
}

func (p *elicitPrompter) Prompt(ctx context.Context, message string, schema map[string]any) (map[string]any, error) {
	if !p.server.clientSupportsFormElicitation() {
		return nil, core.ErrPromptUnsupported
	}

	params := struct {
		Mode            string         `json:"mode"`
		Message         string         `json:"message"`
		RequestedSchema map[string]any `json:"requestedSchema"`
	}{
		Mode:            "form",
		Message:         message,
		RequestedSchema: schema,
	}

	// This waits on the server to respond.
	resp, err := p.server.sendRequestToClient("elicitation/create", params)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("elicitation failed: %s", resp.Error.Message)
	}

	var result struct {
		Action  string `json:"action"`
		Content map[string]any
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, err
	}

	if result.Action != "accept" {
		return nil, core.ErrPromptDeclined
	}

	return result.Content, nil
}
