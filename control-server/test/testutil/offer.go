package testutil

import (
	"testing"

	pion "github.com/pion/webrtc/v4"
)

// BrowserOffer returns the SDP offer a browser would send to the host: a
// receive-only video section plus the "control" data channel. The offering
// connection is closed when the test ends.
func BrowserOffer(t *testing.T) string {
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
