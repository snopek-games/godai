package godot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gitlab.com/snopek-games/godai/mcp/jsonrpc"
)

func TestConnectionConcurrentWrites(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()

		for {
			var req jsonrpc.Request
			if err := ws.ReadJSON(&req); err != nil {
				return
			}
			if len(req.ID) == 0 {
				continue
			}
			resp := jsonrpc.NewResponse(req.ID)
			resp.SetResult(map[string]any{})
			if err := ws.WriteJSON(resp); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dialing the mock editor: %v", err)
	}

	conn := NewConnection(ws, 0)
	go conn.Run()
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 20 {
				if _, err := conn.CallMethod(ctx, "test_method", map[string]any{}); err != nil {
					t.Errorf("calling a method: %v", err)
					return
				}
			}
		}()
		go func() {
			defer wg.Done()
			for range 20 {
				if err := conn.SendNotification(ctx, "test_notification", nil); err != nil {
					t.Errorf("sending a notification: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}
