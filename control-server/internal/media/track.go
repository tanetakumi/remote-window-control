package media

import (
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

// drainRTCP reads and discards incoming RTCP until the sender is closed. The
// interceptors only process feedback while it is being read.
func drainRTCP(sender *pion.RTPSender) {
	buffer := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buffer); err != nil {
			return
		}
	}
}
