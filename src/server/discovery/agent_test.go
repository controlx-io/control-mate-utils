package discovery

import (
	"testing"
)

func TestCreatePayload(t *testing.T) {
	localIP := "192.168.1.10"
	allIPs := []string{"192.168.1.10", "10.0.0.1"}

	payload := createPayload(localIP, allIPs)

	if payload.LocalIP != localIP {
		t.Errorf("Expected LocalIP %s, got %s", localIP, payload.LocalIP)
	}

	if len(payload.OtherIPs) != 1 {
		t.Errorf("Expected 1 OtherIP, got %d", len(payload.OtherIPs))
	}

	if payload.OtherIPs[0] != "10.0.0.1" {
		t.Errorf("Expected OtherIP 10.0.0.1, got %s", payload.OtherIPs[0])
	}

	if payload.Type == "" {
		t.Error("Expected Type to be set")
	}
}

