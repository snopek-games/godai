package main

import (
	"context"
	"godai/mcp/server"
)

func main() {
	// @todo Make this cancel when receiving a signal via signal.NotifyContext()
	ctx := context.Background()

	server := server.NewServer()
	server.Run(ctx)
}
