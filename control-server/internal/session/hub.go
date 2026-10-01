// Package session runs the single remote-control connection: a WebSocket that
// carries WebRTC signaling between the browser and the host, and the control
// data channel that the browser's input commands arrive on.
package session

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"

	"share-app-host/internal/input"
	"share-app-host/internal/media"
	"share-app-host/internal/origin"
)

const handshakeTimeout = 5 * time.Second

var errInputOnWebSocket = errors.New("input must be sent on the control data channel, not the WebSocket")

// Options are the collaborators of a Hub.
type Options struct {
	// Dispatcher applies input commands; it is activated while a browser is
	// connected and releases held input when the connection ends.
	Dispatcher *input.Dispatcher
	// Source, Target and Encoder configure the video each connection streams.
	Source  media.Source
	Target  media.Target
	Encoder media.EncoderConfig
	// StatsInterval, when positive, logs streaming statistics at this interval.
	StatsInterval time.Duration
}

// Hub serves the control WebSocket. Only one connection is active at a time;
// further attempts are refused with 409 Conflict until it ends.
type Hub struct {
	opts     Options
	slot     Slot
	upgrader websocket.Upgrader
}

// NewHub returns a Hub for the given collaborators.
func NewHub(opts Options) *Hub {
	return &Hub{
		opts: opts,
		upgrader: websocket.Upgrader{
			CheckOrigin:      origin.Same,
			HandshakeTimeout: handshakeTimeout,
		},
	}
}

// Close refuses new connections, closes the active one and waits until its
// resources (media workers, held input) have been released.
func (h *Hub) Close() {
	h.slot.Close()
}

// ServeHTTP upgrades the request to a WebSocket and serves the session.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !origin.Same(r) {
		log.Print("session rejected: origin forbidden")
		http.Error(w, "origin forbidden", http.StatusForbidden)
		return
	}
	if !h.slot.TryAcquire() {
		log.Print("session rejected: connection slot unavailable")
		http.Error(w, "a control connection is already active", http.StatusConflict)
		return
	}
	// Registered first, so it runs last: release only after every worker and
	// held input has been cleaned up.
	defer h.slot.Release()

	ws, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("signaling upgrade failed: %v", err)
		return
	}
	c := newConn(ws)
	c.logf("signaling connected")
	defer c.Close()
	h.slot.OnClose(c.abort)

	h.serve(c)
	c.logf("session ended")
}

// serve handles one connection until it ends.
func (h *Hub) serve(c *conn) {
	h.opts.Dispatcher.Activate()
	defer h.opts.Dispatcher.ReleaseAll()

	control := func(payload []byte) {
		if err := h.opts.Dispatcher.Dispatch(payload); err != nil {
			c.send(message{Type: typeInputError, Message: err.Error()})
		}
	}
	peer, err := media.NewPeer(media.PeerOptions{
		Logf:          c.logf,
		Source:        h.opts.Source,
		Target:        h.opts.Target,
		Encoder:       h.opts.Encoder,
		StatsInterval: h.opts.StatsInterval,
		OnICE: func(candidate pion.ICECandidateInit) {
			c.send(message{Type: typeICE, Candidate: &candidate})
		},
		OnControl: control,
		OnFailure: func(err error) {
			c.logf("media failed: %v", err)
			c.send(errorMessage(err.Error()))
			c.abort()
		},
	})
	if err != nil {
		c.logf("peer creation failed: %v", err)
		c.send(errorMessage(err.Error()))
		return
	}
	defer peer.Close() // runs before ReleaseAll: stop the workers, then release input

	for {
		data, err := c.read()
		if err != nil {
			c.logf("signaling read ended: %v", err)
			return
		}
		var msg message
		if err := json.Unmarshal(data, &msg); err != nil {
			c.logf("invalid signaling message: %v", err)
			c.send(errorMessage("invalid signaling message"))
			return
		}
		switch msg.Type {
		case typeOffer:
			c.logf("offer received")
			answer, err := peer.AcceptOffer(msg.SDP)
			if err != nil {
				c.logf("offer failed: %v", err)
				c.send(errorMessage(err.Error()))
				return
			}
			c.send(message{Type: typeAnswer, SDP: answer})
			c.logf("answer sent")
		case typeICE:
			if msg.Candidate == nil {
				continue
			}
			if err := peer.AddICECandidate(*msg.Candidate); err != nil {
				c.logf("ICE candidate failed: %v", err)
				return
			}
		default:
			// Input travels only on the control data channel; the WebSocket is
			// for signaling. Say so, rather than silently dropping it.
			c.send(message{Type: typeInputError, Message: errInputOnWebSocket.Error()})
		}
	}
}
