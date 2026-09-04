package addon

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
)

// print/push_warning/push_error in a running game are forwarded to the editor
// by the Godai autoload, so they must show up in get_log_messages.
func TestGetLogMessagesFromGame(t *testing.T) {
	is := is.New(t)

	const (
		printMarker   = "godai-game-print-6b1f2a"
		warningMarker = "godai-game-warning-6b1f2a"
		errorMarker   = "godai-game-error-6b1f2a"
	)

	// Otherwise the game opens a window on a developer's machine.
	callToolOK(t, "set_project_settings", map[string]any{
		"settings": map[string]any{"editor/run/main_run_args": "--headless"},
	})

	callToolOK(t, "create_script", map[string]any{
		"file_path": "res://fixtures/game_log_spam.gd",
		"content": fmt.Sprintf(`extends Node


func _ready() -> void:
	print(%q)
	push_warning(%q)
	push_error(%q)
`, printMarker, warningMarker, errorMarker),
	})
	callToolOK(t, "create_scene", map[string]any{
		"file_path":      "res://fixtures/game_log_spam.tscn",
		"root_node_type": "Node",
	})
	callToolOK(t, "attach_script", map[string]any{
		"node_path":   ".",
		"script_path": "res://fixtures/game_log_spam.gd",
	})
	callToolOK(t, "save_scene", nil)

	t.Cleanup(func() {
		client.CallTool(testContext(t), "stop_project", map[string]any{})
	})

	run := callToolOK(t, "run_project", map[string]any{
		"scene": "res://fixtures/game_log_spam.tscn",
	})
	is.Equal(run["success"], true)

	// The game takes a moment to boot and hand its output to the editor.
	markers := []string{printMarker, warningMarker, errorMarker}
	deadline := time.Now().Add(60 * time.Second)
	var messages []string
	for time.Now().Before(deadline) {
		structured := callToolOK(t, "get_log_messages", map[string]any{"count": float64(0)})
		raw, _ := structured["messages"].([]any)
		messages = asStrings(raw)
		if len(missingMarkers(messages, markers)) == 0 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if missing := missingMarkers(messages, markers); len(missing) > 0 {
		t.Fatalf("game output never reached get_log_messages; missing %v in:\n%s",
			missing, strings.Join(messages, "\n"))
	}

	stopped := callToolOK(t, "stop_project", nil)
	is.Equal(stopped["success"], true)
}

func missingMarkers(messages, markers []string) []string {
	var missing []string
	for _, marker := range markers {
		found := false
		for _, line := range messages {
			if strings.Contains(line, marker) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, marker)
		}
	}
	return missing
}
