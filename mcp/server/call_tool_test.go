package server

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestSplitProjectPath(t *testing.T) {
	is := is.New(t)

	projectPath, forwarded, err := splitProjectPath(json.RawMessage(
		`{"project_path": "/tmp/project", "file_path": "res://player.gd"}`))
	is.NoErr(err)
	is.Equal(projectPath, "/tmp/project")

	// 'project_path' selects the editor; it isn't a tool argument.
	var arguments map[string]any
	is.NoErr(json.Unmarshal(forwarded, &arguments))
	is.Equal(arguments, map[string]any{"file_path": "res://player.gd"})
}

// Forwarding decodes and re-encodes the arguments, which is where a number can
// quietly lose precision: every JSON number in a map[string]any becomes a
// float64, so anything past 2^53 comes back rounded.
func TestSplitProjectPathPreservesArgumentValues(t *testing.T) {
	is := is.New(t)

	_, forwarded, err := splitProjectPath(json.RawMessage(
		`{"project_path":"/p","id":12345678901234567890,"ratio":1.50,"nested":{"n":9007199254740993}}`))
	is.NoErr(err)

	for _, want := range []string{
		`"id":12345678901234567890`,
		`"ratio":1.50`,
		`"nested":{"n":9007199254740993}`,
	} {
		if !strings.Contains(string(forwarded), want) {
			t.Errorf("forwarded arguments %s do not contain %s", forwarded, want)
		}
	}
}

func TestSplitProjectPathErrors(t *testing.T) {
	for _, tc := range []struct {
		name       string
		arguments  string
		wantErrSub string
	}{
		{"unparsable", `not json`, "unable to parse tool arguments"},
		{"empty", ``, "unable to parse tool arguments"},
		{"missing", `{"file_path": "res://player.gd"}`, "project_path argument is required"},
		// Distinct from 'required', so the model can tell what to fix.
		{"wrong type", `{"project_path": 123}`, "project_path argument must be a string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := splitProjectPath(json.RawMessage(tc.arguments))
			if err == nil {
				t.Fatalf("splitProjectPath(%s) should have failed", tc.arguments)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
			}
		})
	}
}
