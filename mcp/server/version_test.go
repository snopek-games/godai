package server

import (
	"regexp"
	"testing"
)

// Make sure the version parsed out of the embedded plugin.cfg is sane, so a
// broken or reorganized plugin.cfg gets caught by CI rather than shipping a
// server that panics on startup.
func TestGodaiVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(GodaiVersion) {
		t.Errorf("GodaiVersion %q doesn't look like a semantic version", GodaiVersion)
	}
}
