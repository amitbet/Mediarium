package vpn

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseBytes(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		rx, tx int64
	}{
		{"empty", "", 0, 0},
		{"one peer", "rx_bytes=1500\ntx_bytes=300\n", 1500, 300},
		{"two peers add up", "rx_bytes=10\ntx_bytes=1\nrx_bytes=5\ntx_bytes=2\n", 15, 3},
		{"garbage", "rx_bytes=abc\ntx_bytes=\n", 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rx, tx := parseBytes(tc.raw)
			if rx != tc.rx || tx != tc.tx {
				t.Fatalf("parseBytes = %d, %d, want %d, %d", rx, tx, tc.rx, tc.tx)
			}
		})
	}
}

func TestExplainTest(t *testing.T) {
	now := time.Now()
	tests := []struct {
		name      string
		upFor     time.Duration
		handshake time.Time
		err       error
		provider  string
		want      string
		wantPIA   bool
	}{
		{"just started", 5 * time.Second, time.Time{}, errors.New("x"), "", "hasn't answered yet", false},
		{"never answered", time.Minute, time.Time{}, errors.New("x"), "mullvad", "did not answer", false},
		{"never answered on PIA", time.Minute, time.Time{}, errors.New("x"), "pia", "did not answer", true},
		{"went quiet on PIA", time.Hour, now.Add(-10 * time.Minute), errors.New("x"), "pia", "gone quiet", true},
		{"timeout with a fresh handshake", time.Hour, now.Add(-time.Minute), context.DeadlineExceeded, "pia", "no answer in time", false},
		{"other failure with a fresh handshake", time.Hour, now.Add(-time.Minute), errors.New("status 500"), "", "request failed", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := explainTest(now, now.Add(-tc.upFor), tc.handshake, tc.err, tc.provider)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("explainTest = %q, want it to contain %q", got, tc.want)
			}
			if has := strings.Contains(got, "For PIA, run their script again and load the new file."); has != tc.wantPIA {
				t.Fatalf("PIA hint present = %v, want %v in %q", has, tc.wantPIA, got)
			}
		})
	}
}

func TestManagerTestWithoutATunnel(t *testing.T) {
	m := NewManager()
	defer m.Close()
	if res := m.Test(context.Background(), ""); res.OK || res.Reason != "No VPN is switched on." {
		t.Fatalf("off: %+v", res)
	}
	m.Broken("Saved", "Couldn't read the saved connection. Add it again.")
	if res := m.Test(context.Background(), ""); res.OK || res.Reason != "Couldn't read the saved connection. Add it again." {
		t.Fatalf("broken: %+v", res)
	}
}

// A tunnel whose server never answers fails the test with the plain reason, and
// the status shows "no handshake" once the grace period is over.
func TestManagerTestQuietServer(t *testing.T) {
	m := NewManager()
	defer m.Close()
	base := time.Now()
	m.setClock(base)
	m.checkIP = func(ctx context.Context, _ *Tunnel) (string, error) { return "", context.DeadlineExceeded }
	if err := m.Activate("Quiet", quietConfig(t)); err != nil {
		t.Fatalf("activate: %v", err)
	}
	if st := m.Status(); !st.TunnelUp || st.NoHandshake {
		t.Fatalf("inside the grace time: %+v", st)
	}
	m.setClock(base.Add(handshakeGrace + time.Second))
	st := m.Status()
	if !st.NoHandshake || !st.LastHandshake.IsZero() {
		t.Fatalf("after the grace time: %+v, want no handshake", st)
	}
	res := m.Test(context.Background(), "pia")
	if res.OK || !strings.Contains(res.Reason, "did not answer") || !strings.Contains(res.Reason, "For PIA") {
		t.Fatalf("test result: %+v", res)
	}
}

func TestManagerTestPasses(t *testing.T) {
	m := NewManager()
	defer m.Close()
	m.checkIP = func(ctx context.Context, _ *Tunnel) (string, error) { return "203.0.113.7", nil }
	if err := m.Activate("Quiet", quietConfig(t)); err != nil {
		t.Fatalf("activate: %v", err)
	}
	res := m.Test(context.Background(), "")
	if !res.OK || res.IP != "203.0.113.7" || res.Reason != "" {
		t.Fatalf("test result: %+v", res)
	}
}
