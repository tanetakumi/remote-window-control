// Package session runs the single remote-control connection: a WebSocket that
// carries WebRTC signaling between the browser and the host, and the control
// data channel that the browser's input commands arrive on.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"

	"share-app-host/internal/input"
	"share-app-host/internal/media"
	"share-app-host/internal/origin"
)

const handshakeTimeout = 5 * time.Second

// Options are the collaborators of a Hub.
type Options struct {
	// Dispatcher applies input commands; it is activated while a browser is
	// connected and releases held input when the connection ends.
	Dispatcher *input.Dispatcher
	// Sources, Target and Encoder configure the video each connection streams.
	// Encoder is called once per connection, so a changed setting applies to
	// the next one.
	Source media.Source
	// PCSource includes secondary windows in the capture for PC control.
	PCSource media.Source
	// PreparePC minimizes other windows before starting PC capture.
	PreparePC func(context.Context) error
	Target    media.Target
	Encoder   func() media.EncoderConfig
	// StatsInterval, when positive, logs streaming statistics at this
	// interval and asks the browser to report its own.
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

	c.send(message{Type: typeConfig, StatsIntervalMs: h.opts.StatsInterval.Milliseconds()})

	fail := func(err error) {
		c.logf("session failed: %v", err)
		c.send(errorMessage(err.Error()))
		c.abort()
	}
	control := func(payload []byte) {
		if err := h.opts.Dispatcher.Dispatch(payload); err != nil {
			if errors.Is(err, input.ErrModeSwitch) {
				fail(err)
			} else {
				c.send(message{Type: typeInputError, Message: err.Error()})
			}
		}
	}
	peer, err := media.NewPeer(media.PeerOptions{
		Logf:          c.logf,
		Source:        h.opts.Source,
		Target:        h.opts.Target,
		Encoder:       h.opts.Encoder(),
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
	// Mode switches prepare the desktop and restart capture, so they run off
	// the Dispatcher lock in a worker that ends with the connection.
	ctx, cancel := context.WithCancel(context.Background())
	requests := make(chan modeRequest, 1)
	h.opts.Dispatcher.SetModeHandler(func(mode string, changed bool) {
		select {
		case requests <- modeRequest{mode: mode, changed: changed}:
		default:
			// A conforming client has only one mode request in flight.
			c.abort()
		}
	})
	var worker sync.WaitGroup
	worker.Go(func() { h.switchModes(ctx, c, peer, requests, fail) })
	defer func() {
		cancel()
		h.opts.Dispatcher.SetModeHandler(nil)
		worker.Wait()
		_ = peer.Close() // runs before ReleaseAll: stop the workers, then release input
	}()

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
			// Only codec lines are needed to diagnose browser VP9 support.
			for _, line := range strings.Split(msg.SDP, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "a=rtpmap:") || strings.HasPrefix(line, "a=fmtp:") {
					c.logf("offer codec %s", line)
				}
			}
			answer, err := peer.AcceptOffer(msg.SDP)
			if err != nil {
				c.logf("offer failed: %v", err)
				c.send(errorMessage(err.Error()))
				return
			}
			c.send(message{Type: typeAnswer, SDP: answer})
			c.logf("answer sent")
		case typeClientStats:
			c.logf("client stats %s", formatReport(msg.Report))
		case typeICE:
			if msg.Candidate == nil {
				continue
			}
			if err := peer.AddICECandidate(*msg.Candidate); err != nil {
				c.logf("ICE candidate failed: %v", err)
				return
			}
		default:
			c.logf("unsupported signaling message type=%q", msg.Type)
			c.send(errorMessage("unsupported signaling message"))
			return
		}
	}
}

// maxReportFields bounds the counters of one logged client report.
const maxReportFields = 64

// formatReport renders a client report as sorted key=value pairs.
func formatReport(report map[string]float64) string {
	keys := slices.Sorted(maps.Keys(report))
	if len(keys) > maxReportFields {
		return fmt.Sprintf("fields=%d (too many to log)", len(keys))
	}
	fields := make([]string, len(keys))
	for i, key := range keys {
		fields[i] = key + "=" + strconv.FormatFloat(report[key], 'f', -1, 64)
	}
	return strings.Join(fields, " ")
}

type modeRequest struct {
	mode    string
	changed bool
}

// switchModes applies mode requests until ctx ends. A changed mode restarts
// capture, and the browser is told only once the new capture's first sample
// is sent; input stays paused until then.
func (h *Hub) switchModes(ctx context.Context, c *conn, peer *media.Peer, requests <-chan modeRequest, fail func(error)) {
	for {
		var request modeRequest
		select {
		case <-ctx.Done():
			return
		case request = <-requests:
		}
		if !request.changed {
			c.send(message{Type: typeInputMode, Mode: request.mode})
			continue
		}
		pc := request.mode == input.ModePC
		if pc && h.opts.PreparePC != nil {
			if err := h.opts.PreparePC(ctx); err != nil {
				if ctx.Err() == nil {
					fail(err)
				}
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		source := h.opts.Source
		if pc {
			source = h.opts.PCSource
		}
		peer.Restart(source, func() {
			if ctx.Err() != nil {
				return
			}
			h.opts.Dispatcher.CompleteMode()
			c.send(message{Type: typeInputMode, Mode: request.mode})
		})
	}
}
