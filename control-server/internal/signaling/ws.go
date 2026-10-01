package signaling

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
	"share-app-host/internal/capture"
	"share-app-host/internal/input"
	"share-app-host/internal/origin"
	peerpkg "share-app-host/internal/webrtc"
	"share-app-host/internal/window"
)

type Hub struct {
	dispatcher *input.Dispatcher
	bridge     *capture.Probe
	targets    *window.Selection
	mu         sync.Mutex
	active     bool
	activeConn *websocket.Conn
	closed     bool
	workers    sync.WaitGroup
	upgrader   websocket.Upgrader
}
type message struct {
	Type      string                 `json:"type"`
	SDP       string                 `json:"sdp,omitempty"`
	Candidate *pion.ICECandidateInit `json:"candidate,omitempty"`
}

func NewHub(dispatcher *input.Dispatcher, bridge *capture.Probe, targets *window.Selection) *Hub {
	return &Hub{dispatcher: dispatcher, bridge: bridge, targets: targets, upgrader: websocket.Upgrader{CheckOrigin: origin.Same, HandshakeTimeout: 5 * time.Second}}
}
func (h *Hub) acquire() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || h.active {
		return false
	}
	h.active = true
	h.workers.Add(1)
	return true
}
func (h *Hub) release() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.active = false
	h.activeConn = nil
	h.workers.Done()
}
func (h *Hub) Close() {
	h.mu.Lock()
	h.closed = true
	if h.activeConn != nil {
		_ = h.activeConn.Close()
	}
	h.mu.Unlock()
	h.workers.Wait()
}
func (h *Hub) SelectTarget(ctx context.Context, handle uint64) (window.Info, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return window.Info{}, fmt.Errorf("control server is shutting down")
	}
	var selected window.Info
	err := h.dispatcher.ChangeTarget(func() error { var err error; selected, err = h.targets.Select(ctx, handle); return err })
	return selected, err
}
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !origin.Same(r) {
		http.Error(w, "origin forbidden", 403)
		return
	}
	if !h.acquire() {
		http.Error(w, "a control connection is already active", 409)
		return
	}
	defer h.release() // Registered first: release only after every worker and input has closed.
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	h.mu.Lock()
	h.activeConn = conn
	if h.closed {
		_ = conn.Close()
	}
	h.mu.Unlock()
	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(45 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(45 * time.Second)) })
	var writeMu sync.Mutex
	write := func(value any) {
		writeMu.Lock()
		defer writeMu.Unlock()
		_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteJSON(value); err != nil {
			_ = conn.Close()
		}
	}
	heartbeatDone := make(chan struct{})
	var heartbeat sync.WaitGroup
	heartbeat.Add(1)
	go func() {
		defer heartbeat.Done()
		timer := time.NewTicker(15 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-timer.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					_ = conn.Close()
					return
				}
			}
		}
	}()
	defer func() { close(heartbeatDone); heartbeat.Wait() }()
	h.dispatcher.Activate()
	defer h.dispatcher.ReleaseAll()
	control := func(payload []byte) {
		if err := h.dispatcher.Dispatch(payload); err != nil {
			write(map[string]string{"type": "input.error", "message": err.Error()})
		}
	}
	peer, err := peerpkg.NewPeer(h.bridge, h.targets, func(candidate pion.ICECandidateInit) {
		write(map[string]any{"type": "webrtc.ice", "candidate": candidate})
	}, control, func(err error) {
		write(map[string]string{"type": "error", "message": err.Error()})
		_ = conn.Close()
	})
	if err != nil {
		write(map[string]string{"type": "error", "message": err.Error()})
		return
	}
	defer func() { _ = peer.Close(); h.dispatcher.ReleaseAll() }()
	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var msg message
		if err = json.Unmarshal(data, &msg); err != nil {
			write(map[string]string{"type": "error", "message": "invalid signaling message"})
			return
		}
		switch msg.Type {
		case "webrtc.offer":
			answer, err := peer.AcceptOffer(msg.SDP)
			if err != nil {
				write(map[string]string{"type": "error", "message": err.Error()})
				return
			}
			write(map[string]string{"type": "webrtc.answer", "sdp": answer})
		case "webrtc.ice":
			if msg.Candidate != nil {
				if err := peer.AddICECandidate(*msg.Candidate); err != nil {
					log.Printf("ICE error: %v", err)
					return
				}
			}
		default:
			control(data)
		}
	}
}
