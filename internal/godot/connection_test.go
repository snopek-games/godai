package godot

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"gitlab.com/snopek-games/godai/internal/jsonrpc"
)

func TestCallMethodOnADroppedConnection(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	hangUp := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		// Take the request, then die without answering it — the editor being
		// quit or crashing while a tool call is running.
		var req jsonrpc.Request
		ws.ReadJSON(&req)
		ws.Close()
		close(hangUp)
	}))
	defer server.Close()

	ws, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dialing the mock editor: %v", err)
	}

	conn := NewConnection(ws, 0, 0)
	go conn.Run()
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := conn.CallMethod(ctx, "test_method", map[string]any{})
	if resp != nil {
		t.Fatalf("expected no response, got %v", resp)
	}
	if !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("expected ErrConnectionClosed, got %v", err)
	}

	<-hangUp
	if _, err := conn.CallMethod(ctx, "test_method", map[string]any{}); !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("expected ErrConnectionClosed calling a closed connection, got %v", err)
	}
}

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

	conn := NewConnection(ws, 0, 0)
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
