package session

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"share-app-host/internal/input"
)

const (
	// readLimit caps a single incoming message.
	readLimit = input.MaxMessageBytes
	// idleTimeout is how long the peer may stay silent. The browser answers
	// our pings, so a healthy connection renews it every pingInterval.
	idleTimeout  = 45 * time.Second
	pingInterval = 15 * time.Second
	// writeTimeout bounds one write, so a stuck peer cannot block the session.
	writeTimeout = 5 * time.Second
)

// conn wraps a WebSocket for one session: it serialises writes, enforces
// deadlines and runs a keep-alive heartbeat.
type conn struct {
	ws *websocket.Conn

	writeMu sync.Mutex

	stopOnce  sync.Once
	stop      chan struct{}
	heartbeat sync.WaitGroup
}

// newConn configures ws and starts its heartbeat. Call Close when done.
func newConn(ws *websocket.Conn) *conn {
	c := &conn{ws: ws, stop: make(chan struct{})}
	ws.SetReadLimit(readLimit)
	_ = ws.SetReadDeadline(time.Now().Add(idleTimeout))
	ws.SetPongHandler(func(string) error {
		return ws.SetReadDeadline(time.Now().Add(idleTimeout))
	})
	c.heartbeat.Add(1)
	go c.runHeartbeat()
	return c
}

func (c *conn) runHeartbeat() {
	defer c.heartbeat.Done()
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			if err := c.ws.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeTimeout)); err != nil {
				c.abort()
				return
			}
		}
	}
}

// read returns the next text or binary message.
func (c *conn) read() ([]byte, error) {
	_, data, err := c.ws.ReadMessage()
	return data, err
}

// send writes m as JSON. Writes are serialised and bounded by writeTimeout;
// a failed write closes the connection, which ends the read loop.
func (c *conn) send(m message) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = c.ws.SetWriteDeadline(time.Now().Add(writeTimeout))
	if err := c.ws.WriteJSON(m); err != nil {
		c.abort()
	}
}

// abort closes the socket, which unblocks a pending read. It is safe to call
// from any goroutine and any number of times.
func (c *conn) abort() {
	_ = c.ws.Close()
}

// Close stops the heartbeat, waits for it and closes the socket.
func (c *conn) Close() {
	c.stopOnce.Do(func() { close(c.stop) })
	c.abort()
	c.heartbeat.Wait()
}
