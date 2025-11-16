package editor

import (
	"context"
	"encoding/json"
	"godai/mcp/jsonrpc"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Connection struct {
	url string

	connMutex  sync.RWMutex
	conn       *websocket.Conn
	retryDelay time.Duration

	requestMutex     sync.Mutex
	lastRequestID    int
	pendingResponses map[int]chan *jsonrpc.Response
}

func NewConnection(url string, retryDelay time.Duration) *Connection {
	return &Connection{
		url:              url,
		retryDelay:       retryDelay,
		pendingResponses: map[int]chan *jsonrpc.Response{},
	}
}

func (c *Connection) Start(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				log.Printf("%s: stopping connect loop\n", c.url)
				return
			default:
			}

			if c.isConnected() {
				time.Sleep(c.retryDelay)
				continue
			}

			log.Printf("%s: attempting to connect\n", c.url)
			conn, _, err := websocket.DefaultDialer.Dial(c.url, nil)
			if err != nil {
				log.Printf("%s: connect failed: %v (retrying in %s)\n", c.url, err, c.retryDelay)
				time.Sleep(c.retryDelay)
				continue
			}

			log.Printf("%s: connected!", c.url)
			c.setConn(conn)

			go c.readLoop(conn)
		}
	}()
}

func (c *Connection) CallMethod(ctx context.Context, name string, rawParams any) (*jsonrpc.Response, error) {
	respCh := make(chan *jsonrpc.Response, 1)

	c.requestMutex.Lock()
	c.lastRequestID++
	id := c.lastRequestID
	c.pendingResponses[c.lastRequestID] = respCh
	c.requestMutex.Unlock()

	params, err := json.Marshal(rawParams)
	if err != nil {
		return nil, err
	}

	req := jsonrpc.NewRequest(strconv.Itoa(id), name, params)

	if err := c.conn.WriteJSON(req); err != nil {
		c.requestMutex.Lock()
		delete(c.pendingResponses, id)
		c.requestMutex.Unlock()
		return nil, err
	}

	select {
	case resp := <-respCh:

		return resp, nil
	case <-ctx.Done():
		c.requestMutex.Lock()
		delete(c.pendingResponses, id)
		c.requestMutex.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Connection) SendNotification(name string, params json.RawMessage) error {
	return nil
}

func (c *Connection) readLoop(conn *websocket.Conn) {
	for {
		var resp jsonrpc.Response
		if err := c.conn.ReadJSON(&resp); err != nil {
			log.Printf("%s: read error: %v\n", c.url, err)
			c.clearConn(conn)
			return
		}

		var idStr string
		if err := json.Unmarshal(resp.ID, &idStr); err != nil {
			log.Printf("%s: unable to parse response ID: %v\n", c.url, err)
			continue
		}

		id, err := strconv.Atoi(idStr)
		if err != nil {
			log.Printf("%s: unable to parse response ID: %v\n", c.url, err)
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

func (c *Connection) isConnected() bool {
	c.connMutex.RLock()
	defer c.connMutex.RUnlock()
	return c.conn != nil
}

func (c *Connection) setConn(conn *websocket.Conn) {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()
	c.conn = conn
}

func (c *Connection) clearConn(conn *websocket.Conn) {
	c.connMutex.Lock()
	defer c.connMutex.Unlock()

	if c.conn == conn {
		_ = c.conn.Close()
		c.conn = nil

		c.requestMutex.Lock()
		defer c.requestMutex.Unlock()

		for id, ch := range c.pendingResponses {
			close(ch)
			delete(c.pendingResponses, id)
		}
	}
}
