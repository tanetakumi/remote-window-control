package media_test

import (
	"fmt"
	"strings"
	"testing"

	pion "github.com/pion/webrtc/v4"

	"share-app-host/internal/media"
	"share-app-host/test/testutil"
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

func TestPeerAnswersABrowserOfferWithAVP9VideoTrack(t *testing.T) {
	peer := newPeer(t, media.PeerOptions{})

	answer, err := peer.AcceptOffer(testutil.BrowserOffer(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"m=video", "VP9/90000", "profile-id=0", "m=application"} {
		if !strings.Contains(answer, want) {
			t.Errorf("answer lacks %q:\n%s", want, answer)
		}
	}
	for _, unwanted := range []string{"VP8", "H264", "AV1", "profile-id=2"} {
		if strings.Contains(answer, unwanted) {
			t.Errorf("answer advertises %q, which the host cannot encode", unwanted)
		}
	}
}

func TestPeerRejectsOffersWithoutVP9Profile0(t *testing.T) {
	for _, codec := range []pion.RTPCodecParameters{
		{RTPCodecCapability: pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000}, PayloadType: 96},
		{RTPCodecCapability: pion.RTPCodecCapability{MimeType: pion.MimeTypeVP9, ClockRate: 90000, SDPFmtpLine: "profile-id=2"}, PayloadType: 100},
	} {
		t.Run(codec.MimeType+codec.SDPFmtpLine, func(t *testing.T) {
			peer := newPeer(t, media.PeerOptions{})
			answer, err := peer.AcceptOffer(testutil.BrowserOffer(t, codec))
			if err == nil || !strings.Contains(err.Error(), "does not support VP9 profile 0") || answer != "" {
				t.Fatalf("unsupported offer: error=%v answer=%s", err, answer)
			}
		})
	}
}

func TestPeerAcceptsOnlyOneOffer(t *testing.T) {
	peer := newPeer(t, media.PeerOptions{})
	offer := testutil.BrowserOffer(t)
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
	if _, err := peer.AcceptOffer(testutil.BrowserOffer(t)); err != nil {
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

func TestPeerUsesTheOfferedVP9PayloadType(t *testing.T) {
	for _, profile := range []string{"profile-id=0", ""} {
		t.Run(profile, func(t *testing.T) {
			peer := newPeer(t, media.PeerOptions{})
			codec := pion.RTPCodecParameters{
				RTPCodecCapability: pion.RTPCodecCapability{MimeType: pion.MimeTypeVP9, ClockRate: 90000, SDPFmtpLine: profile},
				PayloadType:        120,
			}
			answer, err := peer.AcceptOffer(testutil.BrowserOffer(t, codec))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(answer, "a=rtpmap:120 VP9/90000") {
				t.Fatalf("answer does not use the offered payload type: %s", answer)
			}
		})
	}
}
