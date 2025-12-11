package godot

import (
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
)

type ConnectionManagerConfig struct {
	BasePort     int
	PortCount    int
	RetryDelay   time.Duration
	OnConnect    func(*Connection) error
	OnDisconnect func(*Connection)
}

type ConnectionManager struct {
	config ConnectionManagerConfig
	doneCh chan struct{}
	done   bool
}

func NewConnectionManager(config ConnectionManagerConfig) *ConnectionManager {
	return &ConnectionManager{
		config: config,
		doneCh: make(chan struct{}),
	}
}

func (m *ConnectionManager) Start() error {
	if m.done {
		return errors.New("cannot restart a ConnectionManager")
	}
	for i := 0; i < m.config.PortCount; i++ {
		port := m.config.BasePort + i
		go m.connectionLoop(port)
	}
	return nil
}

func (m *ConnectionManager) Stop() {
	if !m.done {
		m.done = true
		close(m.doneCh)
	}
}

func (m *ConnectionManager) connectionLoop(port int) {
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
			time.Sleep(m.config.RetryDelay)
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
		if m.config.OnConnect != nil {
			if err = m.config.OnConnect(conn); err != nil {
				slog.Error("error setting up editor connection", "port", port, "error", err)
				conn.Close()
				continue
			}
		}

		// Wait until run loop is completed.
		err = <-connErrCh

		slog.Info("disconnected from editor", "port", port, "error", err)
		if m.config.OnDisconnect != nil {
			m.config.OnDisconnect(conn)
		}

		time.Sleep(m.config.RetryDelay)
	}
}
