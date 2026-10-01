package signaling

import (
	"github.com/gorilla/websocket"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"share-app-host/internal/capture"
	"share-app-host/internal/input"
	"share-app-host/internal/window"
	"strings"
	"testing"
	"time"
)

func TestWebSocketRejectsSecondTabAndReconnectsAfterCleanup(t *testing.T) {
	bridge := capture.NewProbe(filepath.Join(t.TempDir(), "CaptureProbe.exe"))
	h := NewHub(input.NewDispatcher(nil), bridge, window.NewSelection(bridge))
	server := httptest.NewServer(h)
	defer server.Close()
	defer h.Close()
	address := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"
	_, rejected, err := websocket.DefaultDialer.Dial(address, http.Header{"Origin": {"https://other-host"}})
	if rejected != nil {
		_ = rejected.Body.Close()
	}
	if err == nil || rejected == nil || rejected.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin connection: %v %#v", err, rejected)
	}
	headers := http.Header{"Origin": {"https://" + strings.TrimPrefix(server.URL, "http://")}}
	conn, _, err := websocket.DefaultDialer.Dial(address, headers)
	if err != nil {
		t.Fatal(err)
	}
	second, response, err := websocket.DefaultDialer.Dial(address, headers)
	if second != nil {
		_ = second.Close()
	}
	if response != nil {
		_ = response.Body.Close()
	}
	if err == nil || response == nil || response.StatusCode != 409 {
		t.Fatalf("second connection: %v %#v", err, response)
	}
	_ = conn.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		next, response, err := websocket.DefaultDialer.Dial(address, headers)
		if response != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			_ = next.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("slot not released after peer workers closed:", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
