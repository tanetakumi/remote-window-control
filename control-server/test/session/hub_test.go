package session_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	pion "github.com/pion/webrtc/v4"

	"share-app-host/internal/input"
	"share-app-host/internal/media"
	"share-app-host/internal/session"
	"share-app-host/test/testutil"
)

const wait = 3 * time.Second

// noTarget never has a window selected, so no capture is ever started.
type noTarget struct{}

func (noTarget) State() (uint64, bool, <-chan struct{}) { return 0, false, make(chan struct{}) }

type fixture struct {
	hub      *session.Hub
	injector *testutil.RecordingInjector
	address  string
	headers  http.Header
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	return newFixtureWith(t, session.Options{})
}

// newFixtureWith serves a Hub with opts, filling in the collaborators tests
// share: a recording injector, a capture that cannot open and no target.
func newFixtureWith(t *testing.T, opts session.Options) *fixture {
	t.Helper()
	injector := &testutil.RecordingInjector{}
	opts.Dispatcher = input.NewDispatcher(injector)
	opts.Source = func(context.Context, uint64) (media.FrameStream, error) {
		return nil, errors.New("no capture in tests")
	}
	opts.Target = noTarget{}
	opts.Encoder = media.DefaultEncoderConfig("ffmpeg")
	hub := session.NewHub(opts)
	server := httptest.NewServer(hub)
	t.Cleanup(server.Close) // runs after hub.Close
	t.Cleanup(hub.Close)

	host := strings.TrimPrefix(server.URL, "http://")
	return &fixture{
		hub:      hub,
		injector: injector,
		address:  "ws://" + host + "/ws",
		headers:  http.Header{"Origin": {"https://" + host}},
	}
}

// dial opens a WebSocket and returns it with the HTTP response, which is the
// rejection when the dial fails.
func (f *fixture) dial(t *testing.T, headers http.Header) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	conn, resp, err := websocket.DefaultDialer.Dial(f.address, headers)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if conn != nil {
		t.Cleanup(func() { _ = conn.Close() })
	}
	return conn, resp, err
}

// connect opens a WebSocket and reads the host's first message, the session
// configuration.
func (f *fixture) connect(t *testing.T) *websocket.Conn {
	t.Helper()
	conn, _, err := f.dial(t, f.headers)
	if err != nil {
		t.Fatal(err)
	}
	readUntil(t, conn, "session.config")
	return conn
}

func send(t *testing.T, conn *websocket.Conn, v any) {
	t.Helper()
	if err := conn.WriteJSON(v); err != nil {
		t.Fatal(err)
	}
}

type reply struct {
	Type            string `json:"type"`
	SDP             string `json:"sdp"`
	Message         string `json:"message"`
	StatsIntervalMs int64  `json:"statsIntervalMs"`
}

// readUntil reads replies until one has the wanted type, skipping ICE
// candidates, which can arrive at any time.
func readUntil(t *testing.T, conn *websocket.Conn, want string) reply {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	for {
		var r reply
		if err := conn.ReadJSON(&r); err != nil {
			t.Fatalf("waiting for %q: %v", want, err)
		}
		if r.Type == want {
			return r
		}
		if r.Type != "webrtc.ice" {
			t.Fatalf("waiting for %q, got %+v", want, r)
		}
	}
}

// expectClosed fails unless the server closes the connection soon.
func expectClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(wait))
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if ne, ok := err.(interface{ Timeout() bool }); ok && ne.Timeout() {
				t.Fatal("connection was not closed")
			}
			return
		}
	}
}

func TestCrossOriginConnectionsAreRejected(t *testing.T) {
	f := newFixture(t)
	_, resp, err := f.dial(t, http.Header{"Origin": {"https://other-host"}})
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin dial: err=%v response=%v", err, resp)
	}
	// The refusal must not have used up the connection slot.
	f.connect(t)
}

func TestSecondConnectionIsRejectedUntilTheFirstEnds(t *testing.T) {
	f := newFixture(t)
	first := f.connect(t)

	_, resp, err := f.dial(t, f.headers)
	if err == nil || resp == nil || resp.StatusCode != http.StatusConflict {
		t.Fatalf("second connection: err=%v response=%v", err, resp)
	}

	_ = first.Close()
	deadline := time.Now().Add(wait)
	for {
		conn, _, err := f.dial(t, f.headers)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot not released after the first connection closed: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestCloseDisconnectsTheActiveConnectionAndRefusesNewOnes(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	closed := make(chan struct{})
	go func() {
		f.hub.Close()
		close(closed)
	}()
	expectClosed(t, conn)
	select {
	case <-closed:
	case <-time.After(wait):
		t.Fatal("Hub.Close did not return")
	}

	if _, _, err := f.dial(t, f.headers); err == nil {
		t.Fatal("connection accepted after Close")
	}
}

func TestSessionConfigAnnouncesTheStatsIntervalFirst(t *testing.T) {
	f := newFixtureWith(t, session.Options{StatsInterval: 5 * time.Second})
	conn, _, err := f.dial(t, f.headers)
	if err != nil {
		t.Fatal(err)
	}
	if r := readUntil(t, conn, "session.config"); r.StatsIntervalMs != 5000 {
		t.Fatalf("config = %+v", r)
	}

	// Without a stats interval the browser is not asked to report.
	f = newFixture(t)
	if conn, _, err = f.dial(t, f.headers); err != nil {
		t.Fatal(err)
	}
	if r := readUntil(t, conn, "session.config"); r.StatsIntervalMs != 0 {
		t.Fatalf("default config = %+v", r)
	}
}

func TestClientStatsAreAcceptedWithoutEndingTheSession(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	send(t, conn, map[string]any{"type": "client.stats", "report": map[string]float64{"kbps": 120, "packetsLost": 3}})
	send(t, conn, map[string]string{"type": "webrtc.offer", "sdp": testutil.BrowserOffer(t)})
	readUntil(t, conn, "webrtc.answer")
}

func TestOfferIsAnswered(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	send(t, conn, map[string]string{"type": "webrtc.offer", "sdp": testutil.BrowserOffer(t)})
	answer := readUntil(t, conn, "webrtc.answer")
	for _, want := range []string{"m=video", "VP9"} {
		if !strings.Contains(answer.SDP, want) {
			t.Errorf("answer lacks %q", want)
		}
	}
}

func TestInvalidOfferIsReportedAndEndsTheSession(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	send(t, conn, map[string]string{"type": "webrtc.offer", "sdp": "not an sdp"})
	if r := readUntil(t, conn, "error"); r.Message == "" {
		t.Fatal("error carried no message")
	}
	expectClosed(t, conn)
}

func TestMalformedSignalingEndsTheSession(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	if err := conn.WriteMessage(websocket.TextMessage, []byte("this is not json")); err != nil {
		t.Fatal(err)
	}
	r := readUntil(t, conn, "error")
	if r.Message != "invalid signaling message" {
		t.Fatalf("message = %q", r.Message)
	}
	expectClosed(t, conn)
}

func TestOversizedMessageEndsTheSession(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	huge := `{"type":"input.text","text":"` + strings.Repeat("a", input.MaxMessageBytes) + `"}`
	_ = conn.WriteMessage(websocket.TextMessage, []byte(huge))
	expectClosed(t, conn)
}

// waitForEvents waits until the injector has recorded at least n events.
func (f *fixture) waitForEvents(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(wait)
	for {
		if events := f.injector.Events(); len(events) >= n {
			return events
		}
		if time.Now().After(deadline) {
			t.Fatalf("wanted %d injector events, have %v", n, f.injector.Events())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func (f *fixture) browser(t *testing.T) *testutil.Browser {
	t.Helper()
	return testutil.ConnectBrowser(t, f.address, f.headers)
}

func TestInputOnTheDataChannelReachesTheInjector(t *testing.T) {
	f := newFixture(t)
	b := f.browser(t)

	b.SendInput(t, map[string]any{"type": "input.tap", "button": "left", "x": 0.25, "y": 0.75})
	b.SendInput(t, map[string]any{"type": "input.text", "text": "hi"})
	events := f.waitForEvents(t, 2)
	if events[0] != "tap:left:0.25,0.75" || events[1] != "text:hi" {
		t.Fatalf("events: %v", events)
	}
}

func TestRejectedInputIsReportedWithoutEndingTheSession(t *testing.T) {
	f := newFixture(t)
	b := f.browser(t)

	b.SendInput(t, map[string]any{"type": "input.explode"})
	r := b.NextReply(t)
	if r.Type != "input.error" || !strings.Contains(r.Message, "unsupported input command") {
		t.Fatalf("reply = %+v", r)
	}

	// The session is still usable.
	b.SendInput(t, map[string]any{"type": "input.text", "text": "still here"})
	f.waitForEvents(t, 1)
}

// The WebSocket carries signaling only. Anything else, including an input
// command, is a protocol error and never reaches the injector.
func TestUnsupportedSignalingEndsTheSession(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)

	send(t, conn, map[string]any{"type": "input.tap", "button": "left", "x": 0.5, "y": 0.5})
	r := readUntil(t, conn, "error")
	if r.Message != "unsupported signaling message" {
		t.Fatalf("message = %q", r.Message)
	}
	expectClosed(t, conn)
	if events := f.injector.Events(); len(events) != 0 {
		t.Fatalf("input from the WebSocket reached the injector: %v", events)
	}
}

func TestDisconnectReleasesHeldInput(t *testing.T) {
	f := newFixture(t)
	b := f.browser(t)

	b.SendInput(t, map[string]any{"type": "input.mouseDown", "button": "left", "x": 0.5, "y": 0.5})
	b.SendInput(t, map[string]any{"type": "input.keyDown", "key": "Enter"})
	f.waitForEvents(t, 2)

	_ = b.WS.Close()
	_ = b.PC.Close()
	events := f.waitForEvents(t, 4)
	if events[2] != "up:Enter" || events[3] != "up:left:0.5,0.5" {
		t.Fatalf("release order: %v", events)
	}
}

func TestVP9UnsupportedOfferIsReportedAndEndsTheSession(t *testing.T) {
	f := newFixture(t)
	conn := f.connect(t)
	codec := pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{MimeType: pion.MimeTypeVP8, ClockRate: 90000},
		PayloadType:        96,
	}
	send(t, conn, map[string]string{"type": "webrtc.offer", "sdp": testutil.BrowserOffer(t, codec)})
	if r := readUntil(t, conn, "error"); !strings.Contains(r.Message, "does not support VP9 profile 0") {
		t.Fatalf("unexpected unsupported-codec error: %+v", r)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("session remained open after the offer failed")
	}
}
