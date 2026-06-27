package addon

import "godai/tests/functional/internal/harness"

// The MCP client and its result types live in the shared harness package. These
// aliases let the rest of the editor test files keep using the short names.
type (
	ToolCallResult = harness.ToolCallResult
	ToolDef        = harness.ToolDef
)
