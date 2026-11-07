package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandshakeAge(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name string
		at   time.Time
		want *int64
	}{
		{"never", time.Time{}, nil},
		{"ten seconds ago", now.Add(-10 * time.Second), ptr(int64(10))},
		{"clock a little behind", now.Add(5 * time.Second), ptr(int64(0))},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := handshakeAge(tc.at, now)
			if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
				t.Fatalf("handshakeAge = %v, want %v", got, tc.want)
			}
		})
	}
}

// With no VPN switched on, the test answers 200 with a plain reason.
func TestVPNTestWithNothingSwitchedOn(t *testing.T) {
	s := newBareServer(t)
	rec := httptest.NewRecorder()
	s.handleVPNTest(rec, httptest.NewRequest(http.MethodPost, "/api/vpn/test", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	if out["ok"] != false || out["reason"] != "No VPN is switched on." {
		t.Fatalf("unexpected answer: %v", out)
	}
}

// A tunnel whose server has not answered shows as up, with no handshake time.
func TestVPNStatusShowsTunnelWithoutHandshake(t *testing.T) {
	s := newBareServer(t)
	saveQuietVPN(t, s, "Quiet VPN")
	s.restoreVPN()
	waitFor(t, "the tunnel to come up", func() bool { return s.VPNManager.Tunnel() != nil })
	got := vpnStatusNow(t, s)
	if got["tunnelUp"] != true {
		t.Fatalf("tunnelUp = %v, want true: %v", got["tunnelUp"], got)
	}
	if _, has := got["handshakeAgeSec"]; has {
		t.Fatalf("a tunnel that never shook hands must not report a handshake age: %v", got)
	}
}
