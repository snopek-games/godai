package mcp

import "testing"

func TestNegotiateProtocolVersion(t *testing.T) {
	for _, v := range []string{"2025-06-18", "2025-11-25"} {
		if got := negotiateProtocolVersion(v); got != v {
			t.Errorf("supported version: got %q, want %q", got, v)
		}
	}

	if got := negotiateProtocolVersion("2099-01-01"); got != ProtocolVersion {
		t.Errorf("unsupported version: got %q, want fallback %q", got, ProtocolVersion)
	}

	if got := negotiateProtocolVersion(""); got != ProtocolVersion {
		t.Errorf("empty version: got %q, want fallback %q", got, ProtocolVersion)
	}

	if !supportedProtocolVersions[ProtocolVersion] {
		t.Errorf("ProtocolVersion %q is not in supportedProtocolVersions", ProtocolVersion)
	}
}
