package session

import pion "github.com/pion/webrtc/v4"

// Message types exchanged with the browser over the WebSocket, which carries
// WebRTC signaling and host notices only. Input commands travel on the control
// data channel.
const (
	// typeConfig is the host's first message: whether the browser should
	// report its receive statistics, and how often.
	typeConfig = "session.config"
	// typeClientStats carries the browser's receive statistics for the log.
	typeClientStats = "client.stats"

	typeOffer  = "webrtc.offer"
	typeAnswer = "webrtc.answer"
	typeICE    = "webrtc.ice"
	// typeError reports a fatal problem; the connection is closed afterwards.
	typeError = "error"
	// typeInputError reports a rejected input command; the connection stays up.
	typeInputError = "input.error"
	// typeInputMode confirms the input mode once its capture is streaming.
	typeInputMode = "input.mode"
)

// message is the JSON envelope for signaling traffic in both directions.
type message struct {
	Type      string                 `json:"type"`
	SDP       string                 `json:"sdp,omitempty"`
	Candidate *pion.ICECandidateInit `json:"candidate,omitempty"`
	Message   string                 `json:"message,omitempty"`
	// StatsIntervalMs belongs to typeConfig.
	StatsIntervalMs int64 `json:"statsIntervalMs,omitempty"`
	// Report belongs to typeClientStats: numeric counters by name.
	Report map[string]float64 `json:"report,omitempty"`
	// Mode belongs to typeInputMode: "window" or "pc".
	Mode string `json:"mode,omitempty"`
}

func errorMessage(text string) message { return message{Type: typeError, Message: text} }
