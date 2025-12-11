package godot

import (
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
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
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}

	if m.server != nil {
		m.server.Close()
		m.server = nil

		err := <-m.errCh
		return err
	}

	return errors.New("cannot stop editor that hasn't been started")
}

func setupTestLogger() {
	// Show the debug log level. Won't appear without `-v` option to `go test`.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)
}

type connectionList struct {
	conns []*Connection
	mutex sync.RWMutex
}

func newConnectionList(initialCapacity int) *connectionList {
	return &connectionList{
		conns: make([]*Connection, 0, initialCapacity),
	}
}

func (l *connectionList) addConn(conn *Connection) error {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.conns = append(l.conns, conn)

	return nil
}

func (l *connectionList) removeConn(conn *Connection) {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	// Filter out the removed connection.
	newConns := make([]*Connection, 0, len(l.conns)-1)
	for _, c := range l.conns {
		if c != conn {
			newConns = append(newConns, c)
		}
	}
	l.conns = newConns
}

func (l *connectionList) GetFirstConn() *Connection {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	if len(l.conns) == 0 {
		return nil
	}
	return l.conns[0]
}

func (l *connectionList) GetConnections() []*Connection {
	l.mutex.RLock()
	defer l.mutex.RUnlock()

	connsCopy := make([]*Connection, len(l.conns))
	copy(connsCopy, l.conns)

	return connsCopy
}

func TestConnectionManagerSingle(t *testing.T) {
	setupTestLogger()

	is := is.New(t)

	s := NewMockEditor("localhost:13000")
	l := newConnectionList(1)
	m := NewConnectionManager(ConnectionManagerConfig{
		BasePort:     13000,
		PortCount:    1,
		RetryDelay:   1 * time.Second,
		OnConnect:    l.addConn,
		OnDisconnect: l.removeConn,
	})
	var conn *Connection

	m.Start()
	defer m.Stop()

	time.Sleep(2 * time.Second)

	// Shouldn't have any connections - the editor is stopped.
	conn = l.GetFirstConn()
	is.True(conn == nil)

	// Start editor - we should get a connection now!
	s.Start()
	defer s.Stop()
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn != nil)

	// Stop the editor - connection should be lost.
	err := s.Stop()
	slog.Debug("editor stopped", "error", err)
	is.True(errors.Is(err, http.ErrServerClosed))
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn == nil)

	// Start editor again - should reconnect!
	s.Start()
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn != nil)

	m.Stop()
}

func TestConnectionManagerMultiple(t *testing.T) {
	setupTestLogger()

	is := is.New(t)

	s1 := NewMockEditor("localhost:13010")
	s2 := NewMockEditor("localhost:13013")

	l := newConnectionList(2)
	m := NewConnectionManager(ConnectionManagerConfig{
		BasePort:     13010,
		PortCount:    10,
		RetryDelay:   1 * time.Second,
		OnConnect:    l.addConn,
		OnDisconnect: l.removeConn,
	})
	var conn *Connection
	var listc []*Connection

	m.Start()
	defer m.Stop()

	time.Sleep(2 * time.Second)

	// Shouldn't have any connections.
	conn = l.GetFirstConn()
	is.True(conn == nil)

	s1.Start()
	defer s1.Stop()
	time.Sleep(2 * time.Second)
	conn = l.GetFirstConn()
	is.True(conn != nil)
	is.Equal(conn.GetPort(), 13010)
	listc = l.GetConnections()
	is.Equal(len(listc), 1)
	is.Equal(listc[0].GetPort(), 13010)

	s2.Start()
	defer s2.Stop()
	time.Sleep(2 * time.Second)
	// First connection is still s1.
	conn = l.GetFirstConn()
	is.True(conn != nil)
	is.Equal(conn.GetPort(), 13010)
	// But the second one should be in there.
	listc = l.GetConnections()
	is.Equal(len(listc), 2)
	is.Equal(listc[0].GetPort(), 13010)
	is.Equal(listc[1].GetPort(), 13013)

	s1.Stop()
	time.Sleep(2 * time.Second)
	// Now s2 has become the first.
	conn = l.GetFirstConn()
	is.True(conn != nil)
	is.Equal(conn.GetPort(), 13013)
	listc = l.GetConnections()
	is.Equal(len(listc), 1)
	is.Equal(listc[0].GetPort(), 13013)
}
