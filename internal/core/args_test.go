package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/matryer/is"
)

func TestSplitProjectPath(t *testing.T) {
	is := is.New(t)

	projectPath, args, err := SplitProjectPath(json.RawMessage(
		`{"project_path": "/tmp/project", "file_path": "res://player.gd"}`))
	is.NoErr(err)
	is.Equal(projectPath, "/tmp/project")

	forwarded, err := args.Raw()
	is.NoErr(err)

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

	_, args, err := SplitProjectPath(json.RawMessage(
		`{"project_path":"/p","id":12345678901234567890,"ratio":1.50,"nested":{"n":9007199254740993}}`))
	is.NoErr(err)

	forwarded, err := args.Raw()
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
			_, _, err := SplitProjectPath(json.RawMessage(tc.arguments))
			if err == nil {
				t.Fatalf("SplitProjectPath(%s) should have failed", tc.arguments)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
			}
		})
	}
}

func TestToolResultErrorMessage(t *testing.T) {
	is := is.New(t)

	// The editor sends the whole structured result as the text content too.
	result := ToolResult{
		Content:           []ContentBlock{{Type: "text", Text: `{"errors":["No scene open"]}`}},
		StructuredContent: json.RawMessage(`{"errors":["No scene open"]}`),
		IsError:           true,
	}
	is.Equal(result.ErrorMessage(), "No scene open")

	plain := ToolResult{Content: []ContentBlock{{Type: "text", Text: "went wrong"}}, IsError: true}
	is.Equal(plain.ErrorMessage(), "went wrong")

	empty := ToolResult{}
	is.Equal(empty.ErrorMessage(), "")
}

func TestToolResultAttachmentDecodesTheMatchingBlock(t *testing.T) {
	is := is.New(t)

	result, err := parseToolResult(json.RawMessage(`{"content": [{"type": "text", "text": "ok"}, {"type": "image", "data": "aGVsbG8=", "mimeType": "image/png"}], "structuredContent": {"success": true}}`))
	is.NoErr(err)

	data, err := result.Attachment("image/png")
	is.NoErr(err)
	is.Equal(string(data), "hello")
	is.Equal(result.Text(), "ok") // the image block adds nothing to the text

	_, err = result.Attachment("image/jpeg")
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "no image/jpeg attachment"))
}

func TestToolResultAttachmentRejectsBadBase64(t *testing.T) {
	is := is.New(t)

	result, err := parseToolResult(json.RawMessage(`{"content": [{"type": "image", "data": "not base64!", "mimeType": "image/png"}]}`))
	is.NoErr(err)

	_, err = result.Attachment("image/png")
	is.True(err != nil)
	is.True(strings.Contains(err.Error(), "base64"))
}
