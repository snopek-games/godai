package server

import (
	"bufio"
	"context"
	"encoding/json"
	"godai/mcp/editor"
	"godai/mcp/jsonrpc"
	"log"
	"os"
	"time"
)

const ProtocolVersion string = "2025-06-18"

// @todo Should we read this from the plugin.cfg?
const GodaiVersion string = "0.1.0"

type Server struct {
	jsonrpcDispatcher *jsonrpc.Dispatcher
	editor            *editor.Connection
	scanner           *bufio.Scanner
}

func NewServer() *Server {
	d := jsonrpc.NewDispatcher()
	editor := editor.NewConnection("ws://localhost:9080", 1*time.Second)
	scanner := bufio.NewScanner(os.Stdin)

	s := &Server{
		jsonrpcDispatcher: d,
		editor:            editor,
		scanner:           scanner,
	}

	d.Register("initialize", s.rpcInitialize)
	d.Register("notifications/initialized", s.rpcClientInitialized)
	d.Register("tools/list", s.rpcListTools)
	d.Register("tools/call", s.rpcCallTool)

	return s

}

func (s *Server) rpcInitialize(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	type appInfo struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	type initializeParams struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities,omitempty"`
		ClientInfo      appInfo        `json:"clientInfo"`
	}

	type initializeResult struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities,omitempty"`
		ServerInfo      appInfo        `json:"serverInfo"`
	}

	var params initializeParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

	response := initializeResult{
		ProtocolVersion: ProtocolVersion,
		Capabilities: map[string]any{
			"tools": map[string]any{},
		},
		ServerInfo: appInfo{
			Name:    "Godai",
			Version: GodaiVersion,
		},
	}

	return response, nil
}

func (s *Server) rpcClientInitialized(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	return nil, nil
}

func (s *Server) rpcListTools(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	type toolOut struct {
		Name         string          `json:"name"`
		Title        string          `json:"title"`
		Description  string          `json:"description"`
		InputSchema  json.RawMessage `json:"inputSchema"`
		OutputSchema json.RawMessage `json:"outputSchema,omitempty"`
	}

	list := []toolOut{}
	for name, tool := range editor.GetDefaultTools() {
		out := toolOut{
			Name:         name,
			Title:        name,
			Description:  tool.GetDescription(),
			InputSchema:  tool.GetInputSchema(),
			OutputSchema: tool.GetOutputSchema(),
		}
		list = append(list, out)
	}

	var resp struct {
		Tools []toolOut `json:"tools"`
	}
	resp.Tools = list

	return resp, nil
}

func (s *Server) rpcCallTool(ctx context.Context, rawParams json.RawMessage) (any, *jsonrpc.Error) {
	type callToolParams struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	var params callToolParams
	if err := json.Unmarshal(rawParams, &params); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InvalidParamsErrorCode, "Invalid parameters", nil)
	}

	// @todo Check if this is a local tool

	resp, err := s.editor.CallMethod(ctx, "tools/call", params)
	if err != nil {
		return nil, jsonrpc.NewError(jsonrpc.InternalErrorCode, "Unable to call method on Godot editor", nil)
	}
	if resp.Error != nil {
		return nil, resp.Error
	}

	return resp.Result, nil
}

func (s *Server) Run(ctx context.Context) {
	s.editor.Start(ctx)

	// @todo How to break this loop if ctx is canceled?

	for s.scanner.Scan() {
		input := s.scanner.Bytes()
		output, err := s.jsonrpcDispatcher.Handle(ctx, input)
		if err != nil {
			log.Printf("error handling input: %v\n", err)
			continue
		}
		if len(output) > 0 {
			os.Stdout.Write(output)
			os.Stdout.Write([]byte("\n"))
		}
	}
}
