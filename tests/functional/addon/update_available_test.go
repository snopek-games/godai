package addon

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

// readMCPStatus returns the text of the status bar at the bottom of the Godai
// panel, which is where an available update gets mentioned.
const readMCPStatus = `
var queue: Array[Node] = [EditorInterface.get_base_control()]
var panel: Node = null
while queue.size() > 0:
	var node: Node = queue.pop_front()
	var node_script = node.get_script()
	if node_script != null and node_script.resource_path.ends_with("godai_panel.gd"):
		panel = node
		break
	for child in node.get_children():
		queue.append(child)

if panel == null:
	print("STATUS: <the Godai panel wasn't found>")
	return FAILED

print("STATUS: " + panel.get_node("%MCPStatusLabel").text)
`

func mcpStatusText(t *testing.T, ctx context.Context) string {
	t.Helper()
	is := is.New(t)

	result, err := client.CallTool(ctx, "execute_editor_script", map[string]any{"code": readMCPStatus})
	is.NoErr(err)
	is.True(!result.IsError)

	var script struct {
		Output []string `json:"output"`
	}
	is.NoErr(json.Unmarshal([]byte(result.Text()), &script))

	for _, line := range script.Output {
		if status, ok := strings.CutPrefix(strings.TrimSpace(line), "STATUS: "); ok {
			return status
		}
	}

	t.Fatalf("the script didn't report the status bar text: %s", result.Text())
	return ""
}

func waitForMCPStatus(t *testing.T, ctx context.Context, want func(string) bool) string {
	t.Helper()

	var status string
	for range 20 {
		status = mcpStatusText(t, ctx)
		if want(status) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return status
}

func TestUpdateAvailableShowsInTheStatusBar(t *testing.T) {
	is := is.New(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	before := mcpStatusText(t, ctx)
	is.True(!strings.Contains(before, "is available"))

	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()

		err := client.Notify(cleanupCtx, "notifications/godai/update_available", map[string]any{
			"current_version": "",
			"latest_version":  "",
		})
		is.NoErr(err)

		status := waitForMCPStatus(t, cleanupCtx, func(s string) bool {
			return !strings.Contains(s, "is available")
		})
		is.True(!strings.Contains(status, "is available"))
	})

	err := client.Notify(ctx, "notifications/godai/update_available", map[string]any{
		"current_version": "0.0.1",
		"latest_version":  "9.9.9",
	})
	is.NoErr(err)

	status := waitForMCPStatus(t, ctx, func(s string) bool {
		return strings.Contains(s, "9.9.9")
	})

	is.True(strings.Contains(status, "godai 9.9.9 is available"))
}
