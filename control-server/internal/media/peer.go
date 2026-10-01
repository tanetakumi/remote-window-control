package media

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	pion "github.com/pion/webrtc/v4"
)

const (
	// controlChannelLabel is the only data channel the browser may open.
	controlChannelLabel = "control"
	// maxPendingICE bounds candidates buffered before the offer arrives.
	maxPendingICE = 128
)

var (
	errConnectionLost    = errors.New("media connection lost")
	errOfferAccepted     = errors.New("offer already accepted")
	errTooManyCandidates = errors.New("too many pending ICE candidates")
)

// PeerOptions configures a Peer.
type PeerOptions struct {
	Source  Source
	Target  Target
	Encoder EncoderConfig
	// StatsInterval, when positive, logs streaming statistics at this interval.
	StatsInterval time.Duration
	// Logf records lifecycle events; nil uses the host logger.
	Logf func(string, ...any)

	// OnICE receives each local ICE candidate to send to the browser.
	OnICE func(pion.ICECandidateInit)
	// OnControl receives each message from the browser's control channel.
	OnControl func([]byte)
	// OnFailure is called when the media connection or the video stream fails.
	OnFailure func(error)
}

// Peer is one WebRTC connection to the browser: a video track fed by a
// Pipeline, and a data channel for control messages.
type Peer struct {
	pc      *pion.PeerConnection
	cancel  context.CancelFunc
	workers sync.WaitGroup

	closeOnce sync.Once
	closeErr  error

	mu         sync.Mutex // guards offered and pendingICE
	offered    bool
	pendingICE []pion.ICECandidateInit
}

// NewPeer creates a peer connection with its video track. Streaming starts
// once the connection is established.
func NewPeer(opts PeerOptions) (*Peer, error) {
	if opts.Logf == nil {
		opts.Logf = log.Printf
	}
	if opts.OnICE == nil {
		opts.OnICE = func(pion.ICECandidateInit) {}
	}
	if opts.OnControl == nil {
		opts.OnControl = func([]byte) {}
	}
	if opts.OnFailure == nil {
		opts.OnFailure = func(error) {}
	}
	pc, err := pion.NewAPI().NewPeerConnection(pion.Configuration{})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p := &Peer{pc: pc, cancel: cancel}

	connected := make(chan struct{})
	var connectedOnce sync.Once
	pc.OnICECandidate(func(candidate *pion.ICECandidate) {
		if candidate != nil && ctx.Err() == nil {
			opts.OnICE(candidate.ToJSON())
		}
	})
	pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		opts.Logf("WebRTC state=%s", state)
		switch state {
		case pion.PeerConnectionStateConnected:
			connectedOnce.Do(func() { close(connected) })
		case pion.PeerConnectionStateFailed, pion.PeerConnectionStateDisconnected:
			if ctx.Err() == nil {
				opts.OnFailure(errConnectionLost)
			}
		}
	})
	pc.OnDataChannel(func(channel *pion.DataChannel) {
		if channel.Label() != controlChannelLabel {
			_ = channel.Close()
			return
		}
		channel.OnOpen(func() { opts.Logf("control channel open") })
		channel.OnClose(func() { opts.Logf("control channel closed") })
		channel.OnMessage(func(msg pion.DataChannelMessage) {
			if ctx.Err() == nil {
				opts.OnControl(msg.Data)
			}
		})
	})

	track, sender, err := addVideoTrack(pc)
	if err != nil {
		cancel()
		_ = pc.Close()
		return nil, err
	}
	pipeline := NewPipeline(opts.Source, opts.Target, track, opts.Encoder)
	pipeline.logf = opts.Logf
	pipeline.StatsInterval = opts.StatsInterval

	p.workers.Add(2)
	go func() {
		defer p.workers.Done()
		readRTCP(sender, pipeline.RequestKeyframe)
	}()
	go func() {
		defer p.workers.Done()
		select {
		case <-ctx.Done():
			return
		case <-connected:
		}
		if err := pipeline.Run(ctx); err != nil && ctx.Err() == nil {
			opts.OnFailure(err)
		}
	}()
	return p, nil
}

// AcceptOffer applies the browser's SDP offer and returns the SDP answer. Only
// one offer is accepted; candidates received earlier are applied first.
func (p *Peer) AcceptOffer(sdp string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.offered {
		return "", errOfferAccepted
	}
	if err := p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: sdp}); err != nil {
		return "", err
	}
	p.offered = true
	for _, candidate := range p.pendingICE {
		if err := p.pc.AddICECandidate(candidate); err != nil {
			return "", err
		}
	}
	p.pendingICE = nil

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}
	return answer.SDP, nil
}

// AddICECandidate applies a remote candidate, holding it until the offer has
// been accepted if it arrives early.
func (p *Peer) AddICECandidate(candidate pion.ICECandidateInit) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.offered {
		return p.pc.AddICECandidate(candidate)
	}
	if len(p.pendingICE) >= maxPendingICE {
		return errTooManyCandidates
	}
	p.pendingICE = append(p.pendingICE, candidate)
	return nil
}

// Close stops streaming, closes the connection and waits for the workers. It
// is idempotent.
func (p *Peer) Close() error {
	p.closeOnce.Do(func() {
		p.cancel()
		p.closeErr = p.pc.Close()
		p.workers.Wait()
	})
	return p.closeErr
}
