package logbuf

import (
	"bytes"
	"log"
	"log/slog"
	"strings"
	"testing"
)

func TestDebugSwitchTakesEffectAtOnce(t *testing.T) {
	var out bytes.Buffer
	oldOut, oldFlags, oldDef := log.Writer(), log.Flags(), slog.Default()
	t.Cleanup(func() {
		slog.SetDefault(oldDef)
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
		SetDebug(false)
	})
	log.SetOutput(ScrubWriter{W: &out})
	InstallSlog()

	SetDebug(false)
	slog.Debug("quiet line")
	slog.Info("info line", "k", "v")
	if strings.Contains(out.String(), "quiet line") {
		t.Fatalf("debug lines must stay out while it is off: %q", out.String())
	}
	if !strings.Contains(out.String(), "info line") {
		t.Fatalf("info lines are always written: %q", out.String())
	}

	SetDebug(true)
	if !Debug() {
		t.Fatal("Debug() should report on")
	}
	slog.Debug("detailed line", "apikey", "0123456789abcdef0123456789abcdef", "password", "hunter2")
	got := out.String()
	if !strings.Contains(got, "detailed line") {
		t.Fatalf("debug lines are written once it is on: %q", got)
	}
	if strings.Contains(got, "0123456789abcdef") || strings.Contains(got, "hunter2") {
		t.Fatalf("secrets must never reach the log, even at debug level: %q", got)
	}

	SetDebug(false)
	out.Reset()
	slog.Debug("quiet again")
	if out.Len() != 0 {
		t.Fatalf("switching off applies at once: %q", out.String())
	}
	// The log package keeps writing in its own style.
	log.Print("plain line")
	if !strings.Contains(out.String(), "plain line") || strings.Contains(out.String(), "level=") {
		t.Fatalf("log package output changed: %q", out.String())
	}
}
