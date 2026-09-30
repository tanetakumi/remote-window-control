package signaling

import (
	"github.com/gorilla/websocket"
	"net/http/httptest"
	"share-app-host/internal/auth"
	"share-app-host/internal/input"
	"share-app-host/internal/nativecapture"
	"share-app-host/internal/targetwindow"
	"strings"
	"testing"
	"time"
)

func TestWebSocketRejectsSecondTabAndReconnectsAfterCleanup(t *testing.T) {
	store := auth.NewStore("test")
	session, err := store.Exchange("test")
	if err != nil {
		t.Fatal(err)
	}
	bridge := nativecapture.NewBridge(t.TempDir())
	h := NewHub(store, input.NewDispatcher(nil), bridge, targetwindow.NewManager(bridge))
	server := httptest.NewServer(h)
	defer server.Close()
	defer h.Close()
	address := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + session.Token
	conn, _, err := websocket.DefaultDialer.Dial(address, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, response, err := websocket.DefaultDialer.Dial(address, nil)
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
		next, response, err := websocket.DefaultDialer.Dial(address, nil)
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
