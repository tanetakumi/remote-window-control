package webrtc

import (
	"context"
	"fmt"
	"sync"

	pion "github.com/pion/webrtc/v4"
	"share-app-host/internal/nativecapture"
	"share-app-host/internal/targetwindow"
)

type Peer struct {
	pc         *pion.PeerConnection
	mu         sync.Mutex
	cancel     context.CancelFunc
	workers    sync.WaitGroup
	closeOnce  sync.Once
	closeErr   error
	pendingICE []pion.ICECandidateInit
	offered    bool
}

func NewPeer(bridge *nativecapture.Bridge, targets *targetwindow.Manager, onICE func(pion.ICECandidateInit), onControl func([]byte), onFailure func(error)) (*Peer, error) {
	pc, err := pion.NewAPI().NewPeerConnection(pion.Configuration{})
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	peer := &Peer{pc: pc, cancel: cancel}
	pc.OnICECandidate(func(candidate *pion.ICECandidate) {
		if candidate != nil && ctx.Err() == nil {
			onICE(candidate.ToJSON())
		}
	})
	pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		if ctx.Err() == nil && (state == pion.PeerConnectionStateFailed || state == pion.PeerConnectionStateDisconnected) {
			onFailure(fmt.Errorf("media connection lost"))
		}
	})
	pc.OnDataChannel(func(channel *pion.DataChannel) {
		if channel.Label() != "control" {
			_ = channel.Close()
			return
		}
		channel.OnMessage(func(msg pion.DataChannelMessage) {
			if ctx.Err() == nil && len(msg.Data) <= 64*1024 {
				onControl(msg.Data)
			}
		})
	})
	if err := attachWindowVideoTrack(ctx, pc, bridge, targets, &peer.workers, onFailure); err != nil {
		cancel()
		_ = pc.Close()
		return nil, err
	}
	return peer, nil
}
func (p *Peer) AcceptOffer(sdp string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.offered {
		return "", fmt.Errorf("offer already accepted")
	}
	if err := p.pc.SetRemoteDescription(pion.SessionDescription{Type: pion.SDPTypeOffer, SDP: sdp}); err != nil {
		return "", err
	}
	p.offered = true
	for _, c := range p.pendingICE {
		if err := p.pc.AddICECandidate(c); err != nil {
			return "", err
		}
	}
	p.pendingICE = nil
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	if err = p.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}
	return answer.SDP, nil
}
func (p *Peer) AddICECandidate(candidate pion.ICECandidateInit) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.offered {
		if len(p.pendingICE) >= 128 {
			return fmt.Errorf("too many pending ICE candidates")
		}
		p.pendingICE = append(p.pendingICE, candidate)
		return nil
	}
	return p.pc.AddICECandidate(candidate)
}
func (p *Peer) Close() error {
	p.closeOnce.Do(func() { p.cancel(); p.closeErr = p.pc.Close(); p.workers.Wait() })
	return p.closeErr
}
