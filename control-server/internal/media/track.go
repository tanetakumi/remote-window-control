package media

import (
	"fmt"
	"strings"

	"github.com/pion/rtcp"
	pion "github.com/pion/webrtc/v4"
)

// Identifiers the browser sees for the video track.
const (
	videoTrackID  = "video"
	videoStreamID = "share-app"
)

// addVideoTrack adds a send-only VP9 track to pc. Samples written to the
// returned track are packetised and sent to the browser.
func addVideoTrack(pc *pion.PeerConnection) (*pion.TrackLocalStaticSample, *pion.RTPSender, error) {
	track, err := pion.NewTrackLocalStaticSample(
		pion.RTPCodecCapability{MimeType: pion.MimeTypeVP9, ClockRate: 90000, SDPFmtpLine: "profile-id=0"}, videoTrackID, videoStreamID)
	if err != nil {
		return nil, nil, err
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		return nil, nil, err
	}
	return track, sender, nil
}

// setVideoCodec chooses the offered VP9 profile-0 format and its RTX format.
// Do this after SetRemoteDescription so Pion's payload types match the offer.
// Explicit profile checking avoids Pion's MIME-only fallback to profile 2.
func setVideoCodec(transceiver *pion.RTPTransceiver) error {
	available := transceiver.Sender().GetParameters().Codecs
	for _, codec := range available {
		if !strings.EqualFold(codec.MimeType, pion.MimeTypeVP9) {
			continue
		}
		profile := "0" // RFC 9628: absent profile-id means profile 0.
		for _, parameter := range strings.Split(codec.SDPFmtpLine, ";") {
			key, value, _ := strings.Cut(parameter, "=")
			if strings.EqualFold(strings.TrimSpace(key), "profile-id") {
				profile = strings.TrimSpace(value)
			}
		}
		if profile != "0" {
			continue
		}
		preferred := []pion.RTPCodecParameters{codec}
		apt := fmt.Sprintf("apt=%d", codec.PayloadType)
		for _, candidate := range available {
			if candidate.MimeType == pion.MimeTypeRTX && candidate.SDPFmtpLine == apt {
				preferred = append(preferred, candidate)
			}
		}
		return transceiver.SetCodecPreferences(preferred)
	}
	return fmt.Errorf("browser does not support VP9 profile 0")
}

// readRTCP reads incoming RTCP until the sender is closed; the interceptors
// (NACK retransmission, reports) only process feedback while it is being read.
// A Picture Loss Indication or Full Intra Request calls onKeyframeRequest.
func readRTCP(sender *pion.RTPSender, onKeyframeRequest func()) {
	for {
		packets, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		for _, packet := range packets {
			switch packet.(type) {
			case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
				onKeyframeRequest()
			}
		}
	}
}
