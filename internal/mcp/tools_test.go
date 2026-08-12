package mcp

import "testing"

func TestLocalToolsAndDefinitionsMatch(t *testing.T) {
	server := &Server{localTools: map[string]*Tool{}}
	server.setupLocalTools()

	for name, tool := range server.localTools {
		if tool.Definition == nil {
			t.Errorf("%s has no definition", name)
		}
	}

	for name := range GetLocalToolDefinitions() {
		if _, ok := server.localTools[name]; !ok {
			t.Errorf("%s is defined in local_tools.json but never registered", name)
		}
	}
}
