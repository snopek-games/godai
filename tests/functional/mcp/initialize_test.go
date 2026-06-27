package mcp

import (
	"godai/mcp/server"
	"testing"

	"github.com/matryer/is"
)

func TestInitialize(t *testing.T) {
	is := is.New(t)

	result, err := client.Initialize(testContext(t))
	is.NoErr(err)
	is.Equal(result.ServerInfo.Name, server.GodaiMcpName)
	is.Equal(result.ServerInfo.Title, server.GodaiMcpTitle)
	is.True(result.ServerInfo.Version != "" && result.ServerInfo.Version != "unknown")
	is.Equal(result.ProtocolVersion, server.ProtocolVersion)
	is.Equal(result.Instructions, server.GodaiMcpInstructions)
	_, ok := result.Capabilities["tools"]
	is.True(ok)
}

func TestInitializeVersionNegotiation(t *testing.T) {
	t.Run("supported_version_is_echoed", func(t *testing.T) {
		is := is.New(t)
		result, err := client.InitializeWithVersion(testContext(t), "2025-06-18")
		is.NoErr(err)
		is.Equal(result.ProtocolVersion, "2025-06-18")
	})

	t.Run("unsupported_version_falls_back_to_preferred", func(t *testing.T) {
		is := is.New(t)
		result, err := client.InitializeWithVersion(testContext(t), "1999-01-01")
		is.NoErr(err)
		is.Equal(result.ProtocolVersion, server.ProtocolVersion)
	})
}
