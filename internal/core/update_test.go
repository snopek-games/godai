package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.com/snopek-games/godai/internal/godot"

	"github.com/gorilla/websocket"
	"github.com/matryer/is"
)

// recordingEditor stands in for the addon's MCP server, so we can see what the
// notification actually looks like on the wire.
func recordingEditor(t *testing.T) (*godot.Connection, chan map[string]any) {
	t.Helper()
	is := is.New(t)

	received := make(chan map[string]any, 4)
	upgrader := websocket.Upgrader{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var message map[string]any
			if err := ws.ReadJSON(&message); err != nil {
				return
			}
			received <- message
		}
	}))
	t.Cleanup(server.Close)

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	is.NoErr(err)

	conn := godot.NewConnection(ws, 1)
	t.Cleanup(func() { conn.Close() })

	return conn, received
}

func TestSendUpdateNotification(t *testing.T) {
	is := is.New(t)

	conn, received := recordingEditor(t)
	s, err := New(Config{})
	is.NoErr(err)

	// Nothing has been found yet, so the editor shouldn't hear anything. The
	// notification that follows is what proves this one sent nothing.
	s.sendUpdateNotification(context.Background(), conn)

	s.updateMutex.Lock()
	s.updateAvailable = "0.4.0"
	s.updateMutex.Unlock()

	s.sendUpdateNotification(context.Background(), conn)

	select {
	case message := <-received:
		is.Equal(message["method"], updateAvailableNotification)

		params, ok := message["params"].(map[string]any)
		is.True(ok)
		is.Equal(params["latest_version"], "0.4.0")
		is.Equal(params["current_version"], Version)
	case <-time.After(5 * time.Second):
		t.Fatal("the editor was never told about the update")
	}
}

func TestCheckForUpdateIsOptOut(t *testing.T) {
	is := is.New(t)

	// With checking turned off, nothing is looked up and nothing is announced.
	s, err := New(Config{UpdateCheckInterval: 0})
	is.NoErr(err)
	s.CheckForUpdate(context.Background())

	s.updateMutex.RLock()
	defer s.updateMutex.RUnlock()
	is.Equal(s.updateAvailable, "")
}
