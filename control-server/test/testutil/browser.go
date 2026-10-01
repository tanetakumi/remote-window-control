package testutil

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"
)

// Reply is a message the host sends over the signaling WebSocket.
type Reply struct {
	Type    string `json:"type"`
	SDP     string `json:"sdp"`
	Message string `json:"message"`
}

// Browser plays the web client against a session Hub: it signals over the
// WebSocket, negotiates WebRTC and opens the "control" data channel, which is
// where input is sent. Everything is closed when the test ends.
type Browser struct {
	WS      *websocket.Conn
	PC      *pion.PeerConnection
	Control *pion.DataChannel

	// Replies receives every WebSocket message that is not WebRTC negotiation,
	// such as "error" and "input.error".
	Replies chan Reply

	writeMu sync.Mutex // gorilla allows one concurrent writer
	mu      sync.Mutex // guards remote and pending
	remote  bool
	pending []pion.ICECandidateInit
}

// ConnectBrowser connects to the WebSocket at address and waits until the
// control data channel is open.
func ConnectBrowser(t *testing.T, address string, headers http.Header) *Browser {
	t.Helper()
	ws, resp, err := websocket.DefaultDialer.Dial(address, headers)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	pc, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		_ = ws.Close()
		t.Fatal(err)
	}
	b := &Browser{WS: ws, PC: pc, Replies: make(chan Reply, 32)}
	t.Cleanup(func() { _ = ws.Close(); _ = pc.Close() })

	open := make(chan struct{})
	if b.Control, err = pc.CreateDataChannel("control", nil); err != nil {
		t.Fatal(err)
	}
	b.Control.OnOpen(func() { close(open) })
	if _, err = pc.AddTransceiverFromKind(pion.RTPCodecTypeVideo,
		pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}
	pc.OnICECandidate(func(c *pion.ICECandidate) {
		if c != nil {
			init := c.ToJSON()
			_ = b.SendSignaling(map[string]any{"type": "webrtc.ice", "candidate": init})
		}
	})

	go b.readSignaling()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = pc.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	if err = b.SendSignaling(map[string]string{"type": "webrtc.offer", "sdp": offer.SDP}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-open:
	case <-time.After(10 * time.Second):
		t.Fatal("the control data channel did not open")
	}
	return b
}

// SendSignaling writes a JSON message on the WebSocket.
func (b *Browser) SendSignaling(v any) error {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	return b.WS.WriteJSON(v)
}

// SendInput sends an input command on the control data channel.
func (b *Browser) SendInput(t *testing.T, v any) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := b.Control.SendText(string(data)); err != nil {
		t.Fatal(err)
	}
}

// NextReply waits for the next non-negotiation WebSocket message.
func (b *Browser) NextReply(t *testing.T) Reply {
	t.Helper()
	select {
	case r := <-b.Replies:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no message from the host")
		return Reply{}
	}
}

func (b *Browser) readSignaling() {
	for {
		_, data, err := b.WS.ReadMessage()
		if err != nil {
			return
		}
		var m struct {
			Reply
			Candidate *pion.ICECandidateInit `json:"candidate"`
		}
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "webrtc.answer":
			if b.PC.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeAnswer, SDP: m.SDP}) != nil {
				return
			}
			b.mu.Lock()
			b.remote = true
			pending := b.pending
			b.pending = nil
			b.mu.Unlock()
			for _, c := range pending {
				_ = b.PC.AddICECandidate(c)
			}
		case "webrtc.ice":
			if m.Candidate == nil {
				continue
			}
			b.mu.Lock()
			if !b.remote {
				b.pending = append(b.pending, *m.Candidate)
				b.mu.Unlock()
				continue
			}
			b.mu.Unlock()
			_ = b.PC.AddICECandidate(*m.Candidate)
		default:
			select {
			case b.Replies <- m.Reply:
			default:
			}
		}
	}
}
