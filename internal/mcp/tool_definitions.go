package mcp

import (
	"embed"
	"fmt"
	"sync"

	"gitlab.com/snopek-games/godai/internal/core"
)

//go:embed local_tools.json
var toolsFS embed.FS

var GetLocalToolDefinitions = sync.OnceValue(func() map[string]*core.ToolDefinition {
	b, err := toolsFS.ReadFile("local_tools.json")
	if err != nil {
		panic(fmt.Errorf("unable to read local_tools.json: %w", err))
	}
	tools, err := core.LoadToolDefinitions(b)
	if err != nil {
		panic(err)
	}
	return tools
})
