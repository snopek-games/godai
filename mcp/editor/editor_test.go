package editor

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/matryer/is"
)

type MockEditor struct {
	addr     string
	upgrader websocket.Upgrader
	errCh    chan error
	server   *http.Server
	conn     *websocket.Conn
}

func NewMockEditor(addr string) *MockEditor {
	m := &MockEditor{
		addr: addr,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
		errCh: make(chan error, 1),
	}
	return m
}

func (m *MockEditor) handleRequest(w http.ResponseWriter, r *http.Request) {
	conn, err := m.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("unable to upgrade to WebSocket", "error", err)
	}
	m.conn = conn
	// @todo Do something with this connection
}

func (m *MockEditor) Start() {
	if m.server != nil {
		return
	}
	m.server = &http.Server{
		Addr:    m.addr,
		Handler: http.HandlerFunc(m.handleRequest),
	}
	go func() {
		slog.Info("starting editor", "addr", m.addr)
		err := m.server.ListenAndServe()
		m.errCh <- err
	}()
}

func (m *MockEditor) Stop() error {
	if m.server != nil {
		m.server.Close()
		m.server = nil

		m.conn.Close()
		m.conn = nil

		err := <-m.errCh
		return err
	}

	return errors.New("cannot stop editor that hasn't been started")
}

func TestManagerConnectSingle(t *testing.T) {
	is := is.New(t)
	is.True(true)

	// Show the debug log level. Won't appear without `-v` option to `go test`.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	s := NewMockEditor("localhost:12000")
	m := NewManager(12000, 1, 1*time.Second, nil, nil)
	var conn *Connection

	m.Start()
	time.Sleep(2 * time.Second)

	// Shouldn't have any connections - the editor is stopped.
	conn = m.GetFirstConn()
	is.True(conn == nil)

	// Start editor - we should get a connection now!
	s.Start()
	defer s.Stop()
	time.Sleep(2 * time.Second)
	conn = m.GetFirstConn()
	is.True(conn != nil)

	// Stop the editor - connection should be lost.
	err := s.Stop()
	slog.Debug("editor stopped", "error", err)
	is.True(errors.Is(err, http.ErrServerClosed))
	time.Sleep(2 * time.Second)
	conn = m.GetFirstConn()
	is.True(conn == nil)

	// Start editor again - should reconnect!
	s.Start()
	time.Sleep(2 * time.Second)
	conn = m.GetFirstConn()
	is.True(conn != nil)

	m.Stop()
}
