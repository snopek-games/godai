package godot

import (
	"context"
	"encoding/json"
	"errors"
	"gitlab.com/snopek-games/godai/internal/jsonrpc"
	"log/slog"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var ErrConnectionClosed = errors.New("the connection to the editor closed")

const (
	pongWait   = 10 * time.Second
	pingPeriod = (pongWait * 9) / 10
)

type Connection struct {
	ws               *websocket.Conn
	port             int
	pid              int
	writeMutex       sync.Mutex
	requestMutex     sync.Mutex
	lastRequestID    int
	pendingResponses map[int]chan *jsonrpc.Response
	done             bool
	doneCh           chan struct{}
	// Set when we hung up, or asked the editor to, so the read error that
	// follows isn't reported as a failure.
	closing bool
}

func NewConnection(ws *websocket.Conn, port int, pid int) *Connection {
	return &Connection{
		ws:               ws,
		port:             port,
		pid:              pid,
		pendingResponses: map[int]chan *jsonrpc.Response{},
		doneCh:           make(chan struct{}),
	}
}

func (c *Connection) GetPort() int {
	return c.port
}

func (c *Connection) GetPID() int {
	return c.pid
}

func (c *Connection) Run() error {
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		c.ws.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})
	go c.pingLoop()

	for {
		var resp jsonrpc.Response
		if err := c.ws.ReadJSON(&resp); err != nil {
			if c.isClosing() {
				slog.Debug("editor connection closed", "port", c.port)
			} else {
				slog.Error("read error from editor", "port", c.port, "error", err)
			}
			c.cleanUp()
			return err
		}

		idStr, ok := resp.GetID()
		if !ok {
			slog.Error("unable to parse response ID from editor", "id", string(resp.ID), "port", c.port)
			continue
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			slog.Error("unable to parse response ID from editor", "id", idStr, "port", c.port, "error", err)
			continue
		}

		c.requestMutex.Lock()
		ch, ok := c.pendingResponses[id]
		if ok {
			delete(c.pendingResponses, id)
		}
		c.requestMutex.Unlock()

		if ok {
			ch <- &resp
			close(ch)
		}
	}
}

func (c *Connection) Close() error {
	c.requestMutex.Lock()
	c.closing = true
	c.requestMutex.Unlock()

	err := c.ws.Close()
	c.cleanUp()
	return err
}

func (c *Connection) ExpectClose(expected bool) {
	c.requestMutex.Lock()
	defer c.requestMutex.Unlock()
	c.closing = expected
}

func (c *Connection) isClosing() bool {
	c.requestMutex.Lock()
	defer c.requestMutex.Unlock()
	return c.closing
}

func (c *Connection) CallMethod(ctx context.Context, name string, rawParams any) (*jsonrpc.Response, error) {
	respCh := make(chan *jsonrpc.Response, 1)

	c.requestMutex.Lock()
	if c.done {
		c.requestMutex.Unlock()
		return nil, ErrConnectionClosed
	}
	c.lastRequestID++
	id := c.lastRequestID
	c.pendingResponses[c.lastRequestID] = respCh
	c.requestMutex.Unlock()

	params, err := json.Marshal(rawParams)
	if err != nil {
		return nil, err
	}

	req := jsonrpc.NewRequest(strconv.Itoa(id), name, params)

	if err := c.writeJSON(req); err != nil {
		c.requestMutex.Lock()
		delete(c.pendingResponses, id)
		c.requestMutex.Unlock()
		return nil, err
	}

	for {
		select {
		case resp, ok := <-respCh:
			if !ok {
				return nil, ErrConnectionClosed
			}
			return resp, nil

		case <-ctx.Done():
			c.requestMutex.Lock()
			delete(c.pendingResponses, id)
			c.requestMutex.Unlock()
			return nil, ctx.Err()
		}
	}
}

func (c *Connection) SendNotification(ctx context.Context, name string, rawParams any) error {
	var params []byte
	if rawParams == nil {
		params = []byte("{}")
	} else {
		var err error
		params, err = json.Marshal(rawParams)
		if err != nil {
			return err
		}
	}

	req := jsonrpc.NewRequest("", name, params)

	return c.writeJSON(req)
}

func (c *Connection) writeJSON(req *jsonrpc.Request) error {
	c.writeMutex.Lock()
	defer c.writeMutex.Unlock()

	return c.ws.WriteJSON(req)
}

func (c *Connection) pingLoop() {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-c.doneCh:
			slog.Debug("stopping editor ping loop normally", "port", c.port)
			return
		case <-ticker.C:
		}

		err := c.ws.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(time.Second))
		if err != nil {
			slog.Error("editor ping failed, stopping ping loop", "port", c.port, "error", err)
			return
		}
	}
}

func (c *Connection) cleanUp() {
	c.requestMutex.Lock()
	defer c.requestMutex.Unlock()

	if c.done {
		// Prevent double clean-up.
		return
	}

	for id, ch := range c.pendingResponses {
		close(ch)
		delete(c.pendingResponses, id)
	}

	c.done = true
	close(c.doneCh)
}
