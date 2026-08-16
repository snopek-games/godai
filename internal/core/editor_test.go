package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/internal/godot"

	"github.com/gorilla/websocket"
	"github.com/matryer/is"
)

func replyingEditor(t *testing.T, result string) *godot.Connection {
	t.Helper()
	is := is.New(t)

	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var request map[string]json.RawMessage
			if err := ws.ReadJSON(&request); err != nil {
				return
			}
			if err := ws.WriteJSON(map[string]json.RawMessage{
				"jsonrpc": json.RawMessage(`"2.0"`),
				"id":      request["id"],
				"result":  json.RawMessage(result),
			}); err != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	is.NoErr(err)

	conn := godot.NewConnection(ws, 1, 0)
	go conn.Run()
	t.Cleanup(func() { conn.Close() })

	return conn
}

func TestCallEditorToolPassesThroughUnparsableResults(t *testing.T) {
	is := is.New(t)

	const result = `{"content":[{"type":"text","text":42}]}`

	s, err := New(Config{EditorToolTimeout: 5 * time.Second})
	is.NoErr(err)
	s.addEditor("/p", replyingEditor(t, result))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	got, err := s.CallEditorTool(ctx, "/p", "some_tool", Args{}, CallOptions{})
	is.NoErr(err)
	is.True(got.Unparsed)
	is.Equal(string(got.Raw), result)
}

func TestCallEditorToolRefusesAMismatchedAddon(t *testing.T) {
	is := is.New(t)

	s, err := New(Config{EditorToolTimeout: 5 * time.Second})
	is.NoErr(err)
	s.addEditorWithAddonVersion("/p", replyingEditor(t, `{"content":[]}`), "0.0.1")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.CallEditorTool(ctx, "/p", "some_tool", Args{}, CallOptions{})
	is.True(errors.Is(err, ErrAddonVersionMismatch))
	is.True(strings.Contains(err.Error(), "0.0.1"))

	var userErr *UserError
	is.True(errors.As(err, &userErr))
	is.True(strings.Contains(strings.Join(userErr.Solutions, " "), "godai editor restart"))

	// Closing is the way out of the mismatch, so it goes through.
	_, err = s.CallEditorTool(ctx, "/p", "close_editor", Args{}, CallOptions{})
	is.NoErr(err)
}

func TestCallEditorToolRefusesAnAddonWithoutAVersion(t *testing.T) {
	is := is.New(t)

	s, err := New(Config{EditorToolTimeout: 5 * time.Second})
	is.NoErr(err)
	s.addEditorWithAddonVersion("/p", replyingEditor(t, `{"content":[]}`), "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = s.CallEditorTool(ctx, "/p", "some_tool", Args{}, CallOptions{})
	is.True(errors.Is(err, ErrAddonVersionMismatch))
}

func TestCloseHeadlessEditorsClosesOnesWeLaunched(t *testing.T) {
	is := is.New(t)

	s := newTestSession(t)
	conn, received := recordingEditor(t)
	s.addEditorWithHeadless("/p", conn, true)
	s.markHeadlessProject("/p")

	// The recording editor never replies, so the close call blocks until its
	// own timeout.
	go s.closeHeadlessEditors()

	select {
	case message := <-received:
		is.Equal(message["method"], "tools/call")
		params, ok := message["params"].(map[string]any)
		is.True(ok)
		is.Equal(params["name"], "close_editor")
	case <-time.After(5 * time.Second):
		t.Fatal("the headless editor we launched was never closed")
	}
}

// If the user replaced our headless editor with a windowed one for the same
// project, that replacement is theirs and has to survive our shutdown.
func TestCloseHeadlessEditorsLeavesVisibleEditorsAlone(t *testing.T) {
	s := newTestSession(t)
	conn, received := recordingEditor(t)
	s.addEditorWithHeadless("/p", conn, false)
	s.markHeadlessProject("/p")

	s.closeHeadlessEditors()

	select {
	case message := <-received:
		t.Fatalf("closed an editor that isn't headless: %v", message)
	case <-time.After(500 * time.Millisecond):
	}
}

func TestCloseHeadlessEditorsIgnoresUnmarkedEditors(t *testing.T) {
	s := newTestSession(t)
	conn, received := recordingEditor(t)
	s.addEditorWithHeadless("/p", conn, true)

	s.closeHeadlessEditors()

	select {
	case message := <-received:
		t.Fatalf("closed a headless editor we didn't launch: %v", message)
	case <-time.After(500 * time.Millisecond):
	}
}
