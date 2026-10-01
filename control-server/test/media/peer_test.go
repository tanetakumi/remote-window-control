package media_test

import (
	"fmt"
	"strings"
	"testing"

	pion "github.com/pion/webrtc/v4"

	"share-app-host/internal/media"
)

func newPeer(t *testing.T, opts media.PeerOptions) *media.Peer {
	t.Helper()
	if opts.Source == nil {
		opts.Source = newFakeSource(stalled()).source()
	}
	if opts.Target == nil {
		opts.Target = newFakeTarget()
	}
	opts.Encoder = defaultEncoder()
	peer, err := media.NewPeer(opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	return peer
}

// browserOffer builds the SDP offer a browser would send: a receive-only video
// section and the control data channel.
func browserOffer(t *testing.T) string {
	t.Helper()
	browser, err := pion.NewPeerConnection(pion.Configuration{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = browser.Close() })
	if _, err = browser.AddTransceiverFromKind(pion.RTPCodecTypeVideo,
		pion.RTPTransceiverInit{Direction: pion.RTPTransceiverDirectionRecvonly}); err != nil {
		t.Fatal(err)
	}
	if _, err = browser.CreateDataChannel("control", nil); err != nil {
		t.Fatal(err)
	}
	offer, err := browser.CreateOffer(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = browser.SetLocalDescription(offer); err != nil {
		t.Fatal(err)
	}
	return offer.SDP
}

func TestPeerAnswersABrowserOfferWithAVP8VideoTrack(t *testing.T) {
	peer := newPeer(t, media.PeerOptions{})

	answer, err := peer.AcceptOffer(browserOffer(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"m=video", "VP8", "m=application"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
}

func TestPeerAcceptsOnlyOneOffer(t *testing.T) {
	peer := newPeer(t, media.PeerOptions{})
	offer := browserOffer(t)
	if _, err := peer.AcceptOffer(offer); err != nil {
		t.Fatal(err)
	}
	_, err := peer.AcceptOffer(offer)
	if err == nil || !strings.Contains(err.Error(), "offer already accepted") {
		t.Fatalf("second offer: error = %v", err)
	}
}

func TestPeerRejectsAnInvalidOffer(t *testing.T) {
	peer := newPeer(t, media.PeerOptions{})
	if _, err := peer.AcceptOffer("not an sdp"); err == nil {
		t.Fatal("garbage offer accepted")
	}
	// A rejected offer does not use up the one allowed offer.
	if _, err := peer.AcceptOffer(browserOffer(t)); err != nil {
		t.Fatalf("valid offer after a rejected one: %v", err)
	}
}

func TestPeerBoundsCandidatesReceivedBeforeTheOffer(t *testing.T) {
	peer := newPeer(t, media.PeerOptions{})
	candidate := pion.ICECandidateInit{Candidate: "candidate:1 1 udp 2130706431 192.0.2.1 5000 typ host"}
	for i := 0; i < 128; i++ {
		if err := peer.AddICECandidate(candidate); err != nil {
			t.Fatalf("candidate %d rejected: %v", i+1, err)
		}
	}
	err := peer.AddICECandidate(candidate)
	if err == nil || !strings.Contains(err.Error(), "too many pending ICE candidates") {
		t.Fatalf("candidate 129: error = %v", err)
	}
}

func TestPeerCloseIsIdempotent(t *testing.T) {
	peer, err := media.NewPeer(media.PeerOptions{
		Source:  newFakeSource(stalled()).source(),
		Target:  newFakeTarget(),
		Encoder: defaultEncoder(),
	})
	if err != nil {
		t.Fatal(err)
	}
	first := peer.Close()
	second := peer.Close()
	if fmt.Sprint(first) != fmt.Sprint(second) {
		t.Fatalf("Close results differ: %v vs %v", first, second)
	}
}
