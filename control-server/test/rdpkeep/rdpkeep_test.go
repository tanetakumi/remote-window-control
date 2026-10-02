package rdpkeep_test

import (
	"testing"

	"share-app-host/internal/rdpkeep"
)

func TestStartRequiresCredentials(t *testing.T) {
	for name, cfg := range map[string]rdpkeep.Config{
		"missing username": {Password: "pw"},
		"missing password": {Username: "user"},
	} {
		m := rdpkeep.NewManager(cfg)
		status, err := m.SetEnabled(true)
		if err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		if status.State != rdpkeep.StateOff || status.Enabled {
			t.Fatalf("%s: status = %+v, want off and disabled", name, status)
		}
	}
}

func TestStopWithoutStartIsANoop(t *testing.T) {
	m := rdpkeep.NewManager(rdpkeep.Config{Username: "user", Password: "pw"})
	status, err := m.SetEnabled(false)
	if err != nil {
		t.Fatal(err)
	}
	if status.State != rdpkeep.StateOff || status.Enabled {
		t.Fatalf("status = %+v, want off and disabled", status)
	}
}
