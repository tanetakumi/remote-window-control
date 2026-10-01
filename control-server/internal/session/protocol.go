package session

import pion "github.com/pion/webrtc/v4"

// Message types exchanged with the browser over the WebSocket, which carries
// WebRTC signaling and host notices only. Input commands travel on the control
// data channel; one arriving here is rejected with typeInputError.
const (
	typeOffer  = "webrtc.offer"
	typeAnswer = "webrtc.answer"
	typeICE    = "webrtc.ice"
	// typeError reports a fatal problem; the connection is closed afterwards.
	typeError = "error"
	// typeInputError reports a rejected input command; the connection stays up.
	typeInputError = "input.error"
)

// message is the JSON envelope for signaling traffic in both directions.
type message struct {
	Type      string                 `json:"type"`
	SDP       string                 `json:"sdp,omitempty"`
	Candidate *pion.ICECandidateInit `json:"candidate,omitempty"`
	Message   string                 `json:"message,omitempty"`
}

func errorMessage(text string) message { return message{Type: typeError, Message: text} }
