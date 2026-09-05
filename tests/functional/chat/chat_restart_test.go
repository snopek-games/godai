// Package chat contains end-to-end tests that drive the editor's in-panel
// chat against a stubbed LLM API, using the same eval hooks godai-eval
// uses to submit the prompt. Unlike the addon suite, which speaks MCP HTTP to
// a shared editor, each of these tests launches a private editor: connecting
// an MCP client would perturb the chat under test, and the editor may not
// survive its own chat (restart_editor).
package chat

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/matryer/is"
	"gitlab.com/snopek-games/godai/internal/isolation"
	"gitlab.com/snopek-games/godai/tests/functional/internal/harness"
)

const (
	chatRestartPrompt   = "Please restart the editor."
	chatRestartDoneText = "The editor was restarted successfully."
)

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Fprintln(os.Stderr, "SKIP: functional tests don't run in -short mode")
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// A scripted LLM API: a chat that hasn't run the restart tool yet gets a
// restart_editor tool call, and one whose transcript already holds the tool's
// result gets a closing text message. Keying on the chat content rather than a
// call counter keeps it idempotent: the pre-restart editor's follow-up request
// (which dies with that editor) and the resumed editor's identical replay both
// get the same answer.
type stubProvider struct {
	name         string
	path         string
	hasResult    string // substring of a request body that means the tool has been answered
	toolCallBody map[string]any
	doneBody     map[string]any
}

var stubAnthropic = stubProvider{
	name:      "anthropic",
	path:      "/v1/messages",
	hasResult: `"tool_result"`,
	toolCallBody: map[string]any{
		"type": "message", "role": "assistant", "stop_reason": "tool_use",
		"content": []map[string]any{{
			"type":  "tool_use",
			"id":    "toolu_restart",
			"name":  "restart_editor",
			"input": map[string]any{"skip_save": true},
		}},
	},
	doneBody: map[string]any{
		"type": "message", "role": "assistant", "stop_reason": "end_turn",
		"content": []map[string]any{{"type": "text", "text": chatRestartDoneText}},
	},
}

var stubOpenAI = stubProvider{
	name:      "openai_chat_completions",
	path:      "/v1/chat/completions",
	hasResult: `"role":"tool"`,
	toolCallBody: map[string]any{
		"choices": []map[string]any{{
			"index": 0, "finish_reason": "tool_calls",
			"message": map[string]any{
				"role": "assistant", "content": nil,
				"tool_calls": []map[string]any{{
					"id": "call_restart", "type": "function",
					"function": map[string]any{"name": "restart_editor", "arguments": `{"skip_save": true}`},
				}},
			},
		}},
	},
	doneBody: map[string]any{
		"choices": []map[string]any{{
			"index": 0, "finish_reason": "stop",
			"message": map[string]any{"role": "assistant", "content": chatRestartDoneText},
		}},
	},
}

func (p stubProvider) newServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != p.path {
			http.NotFound(w, r)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reply := p.toolCallBody
		if strings.Contains(string(body), p.hasResult) {
			reply = p.doneBody
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(reply)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestChatRestartResumesAfterEditorRestart(t *testing.T) {
	for _, provider := range []stubProvider{stubAnthropic, stubOpenAI} {
		t.Run(provider.name, func(t *testing.T) {
			testChatRestartResumesAfterEditorRestart(t, provider)
		})
	}
}

func testChatRestartResumesAfterEditorRestart(t *testing.T, provider stubProvider) {
	is := is.New(t)

	godotBin, err := harness.FindGodot()
	if err != nil {
		t.Skipf("no Godot binary: %v", err)
	}

	stub := provider.newServer(t)

	dir, err := os.MkdirTemp("", "godai-chat-restart-*")
	is.NoErr(err)
	t.Cleanup(func() {
		if t.Failed() || os.Getenv("GODAI_TEST_KEEP") != "" {
			t.Logf("Test project kept at %s (editor log: %s)", dir, filepath.Join(dir, "editor.log"))
		} else {
			os.RemoveAll(dir)
		}
	})

	is.NoErr(harness.CreateTestProject(dir, harness.ProjectOptions{
		Name:         "Godai Chat Restart Test",
		InstallAddon: true,
	}))

	// Dot-prefixed so the Godot importer ignores them.
	promptPath := filepath.Join(dir, ".godai-eval-prompt.txt")
	streamPath := filepath.Join(dir, ".godai-eval-stream.jsonl")
	is.NoErr(os.WriteFile(promptPath, []byte(chatRestartPrompt), 0o644))

	cmd, _, err := harness.LaunchEditor(godotBin, dir, harness.EditorOptions{
		Verbose: os.Getenv("GODAI_TEST_VERBOSE") != "",
		ExtraEnv: []string{
			"GODAI_API_PROVIDER=" + provider.name,
			"GODAI_API_URL=" + stub.URL + "/v1/",
			"GODAI_EVAL_PROMPT_FILE=" + promptPath,
			"GODAI_EVAL_STREAM_FILE=" + streamPath,
		},
	})
	is.NoErr(err)
	t.Cleanup(func() { harness.StopEditor(cmd) })

	// The chat is self-driving (EvalRun submits the prompt as soon as the
	// editor is up), so the first editor's exit is the signal that the restart
	// happened. The wait covers the initial project import too.
	exited := make(chan struct{})
	go func() {
		cmd.Wait()
		close(exited)
	}()
	select {
	case <-exited:
	case <-time.After(harness.OpenTimeout(180*time.Second) + 60*time.Second):
		t.Fatal("the editor did not exit after the chat requested a restart")
	}

	// The relaunched editor is a fresh process the harness didn't start; its
	// instance file is how we find (and later stop) it. The exited editor
	// deleted its own instance file on the way down, so the next one to appear
	// belongs to the relaunched editor.
	instancesDir := isolation.GodaiInstancesDir(harness.IsolationDir(dir))
	inst, err := harness.WaitForInstance(instancesDir, harness.OpenTimeout(180*time.Second))
	is.NoErr(err)
	restartedPid := inst.Pid
	t.Cleanup(func() {
		if !harness.Alive(restartedPid) {
			return
		}
		proc, err := os.FindProcess(restartedPid)
		if err != nil {
			return
		}
		harness.AskToStop(proc)
		for range 150 {
			if !harness.Alive(restartedPid) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		proc.Kill()
	})

	raw := waitForCompletedChatSession(t, filepath.Join(dir, ".godot", "godai-chat-sessions"), 120*time.Second)

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var header struct {
		Version int `json:"version"`
	}
	is.NoErr(json.Unmarshal([]byte(lines[0]), &header))
	is.Equal(header.Version, 1) // the session file starts with its format version

	var roles []string
	for _, line := range lines[1:] {
		var msg struct {
			Role string `json:"role"`
		}
		is.NoErr(json.Unmarshal([]byte(line), &msg))
		roles = append(roles, msg.Role)
	}
	is.Equal(roles, []string{"user", "assistant", "user", "assistant"}) // prompt, tool_use, tool_result, closing text

	text := string(raw)
	is.True(strings.Contains(text, chatRestartPrompt))
	is.True(strings.Contains(text, "restart_editor"))
	is.True(strings.Contains(text, `"tool_result"`))
	is.True(strings.Count(text, chatRestartDoneText) == 1) // resumed exactly once

	_, err = os.Stat(filepath.Join(dir, ".godot", "godai-resume-session.json"))
	is.True(os.IsNotExist(err)) // the resume marker is consumed

	stream, err := os.ReadFile(streamPath)
	is.NoErr(err)
	is.True(strings.Contains(string(stream), `"subtype":"editor_teardown"`)) // first segment flushed on the way down
	is.True(strings.Contains(string(stream), `"subtype":"success"`))         // the resumed segment finished the run
}

func waitForCompletedChatSession(t *testing.T, sessionsDir string, timeout time.Duration) []byte {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		entries, _ := os.ReadDir(sessionsDir)
		for _, entry := range entries {
			id := strings.TrimSuffix(entry.Name(), ".jsonl")
			if entry.IsDir() || strings.HasSuffix(id, "-mcp") || strings.HasSuffix(id, "-cli") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(sessionsDir, entry.Name()))
			if err == nil && strings.Contains(string(b), chatRestartDoneText) {
				return b
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("no chat session completed with the stub's closing message")
	return nil
}
