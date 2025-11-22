package editor

import (
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Manager struct {
	basePort     int
	portCount    int
	retryDelay   time.Duration
	doneCh       chan struct{}
	conns        []*Connection
	connsMutex   sync.RWMutex
	onConnect    func(*Connection) error
	onDisconnect func(*Connection)
}

func NewManager(basePort int, portCount int, retryDuration time.Duration, onConnect func(*Connection) error, onDisconnect func(*Connection)) *Manager {
	return &Manager{
		basePort:     basePort,
		portCount:    portCount,
		retryDelay:   retryDuration,
		doneCh:       make(chan struct{}),
		conns:        make([]*Connection, 0, portCount),
		onConnect:    onConnect,
		onDisconnect: onDisconnect,
	}
}

func (m *Manager) Start() {
	for i := 0; i < m.portCount; i++ {
		port := m.basePort + i
		go m.connectionLoop(port)
	}
}

func (m *Manager) Stop() {
	close(m.doneCh)
}

func (m *Manager) connectionLoop(port int) {
	slog.Debug("starting connection loop", "port", port)

	var url string = "ws://localhost:" + strconv.Itoa(port)
	for {
		select {
		case <-m.doneCh:
			slog.Debug("stopping connection loop", "port", port)
			return
		default:
		}

		ws, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			slog.Debug("failed to connect", "port", port, "error", err)
			time.Sleep(m.retryDelay)
			continue
		}

		slog.Info("connected to editor", "port", port)
		conn := NewConnection(ws, port)

		// Start the connection's run loop.
		connErrCh := make(chan error, 1)
		go func() {
			err := conn.Run()
			connErrCh <- err
		}()

		// Do any on connection setup, and discard the connection if there's errors.
		if m.onConnect != nil {
			if err = m.onConnect(conn); err != nil {
				slog.Error("error setting up editor connection", "port", port, "error", err)
				conn.Close()
				continue
			}
		}

		// Add and wait until run loop is completed.
		m.addConn(conn)
		err = <-connErrCh

		slog.Info("disconnected from editor", "port", port, "error", err)
		m.removeConn(conn)
		if m.onDisconnect != nil {
			m.onDisconnect(conn)
		}

		time.Sleep(m.retryDelay)
	}
}

func (m *Manager) addConn(conn *Connection) {
	m.connsMutex.Lock()
	defer m.connsMutex.Unlock()

	m.conns = append(m.conns, conn)
}

func (m *Manager) removeConn(conn *Connection) {
	m.connsMutex.Lock()
	defer m.connsMutex.Unlock()

	// Filter out the removed connection.
	newConns := make([]*Connection, 0, m.portCount)
	for _, c := range m.conns {
		if c != conn {
			newConns = append(newConns, c)
		}
	}
	m.conns = newConns
}

func (m *Manager) GetFirstConn() *Connection {
	m.connsMutex.RLock()
	defer m.connsMutex.RUnlock()

	if len(m.conns) == 0 {
		return nil
	}
	return m.conns[0]
}
