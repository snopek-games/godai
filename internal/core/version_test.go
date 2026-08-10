package core

import (
	"regexp"
	"testing"
)

// Make sure the version parsed out of the embedded plugin.cfg is sane, so a
// broken or reorganized plugin.cfg gets caught by CI rather than shipping a
// binary that panics on startup.
func TestGodaiVersion(t *testing.T) {
	if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(Version) {
		t.Errorf("Version %q doesn't look like a semantic version", Version)
	}
}
