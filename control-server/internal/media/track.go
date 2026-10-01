package media

import (
	"github.com/pion/rtcp"
	pion "github.com/pion/webrtc/v4"
)

// Identifiers the browser sees for the video track.
const (
	videoTrackID  = "video"
	videoStreamID = "share-app"
)

// addVideoTrack adds a send-only VP8 track to pc. Samples written to the
// returned track are packetised and sent to the browser.
func addVideoTrack(pc *pion.PeerConnection) (*pion.TrackLocalStaticSample, *pion.RTPSender, error) {
	track, err := pion.NewTrackLocalStaticSample(
		pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8}, videoTrackID, videoStreamID)
	if err != nil {
		return nil, nil, err
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		return nil, nil, err
	}
	return track, sender, nil
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
