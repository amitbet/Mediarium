package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestUserApplicationSingleInstanceAndGracefulStop(t *testing.T) {
	name := fmt.Sprintf(`Local\MediariumTest-%d-%d`, os.Getpid(), time.Now().UnixNano())
	dir := t.TempDir()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- runUserApplication(name, dir, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		})
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("user app did not start")
	}
	if err := runUserApplication(name, dir, func(context.Context) error { t.Error("second instance started"); return nil }); err != nil {
		t.Fatal(err)
	}
	ptr, _ := windows.UTF16PtrFromString(name)
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, ptr)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(event)
	if err := windows.SetEvent(event); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("user app did not stop gracefully")
	}
	if _, err := os.Stat(dir + `\user.pid`); !os.IsNotExist(err) {
		t.Fatalf("PID file not removed: %v", err)
	}
}

func TestUserApplicationRestartsOnRequest(t *testing.T) {
	calls := 0
	err := runUserApplication(fmt.Sprintf(`Local\MediariumRestartTest-%d`, time.Now().UnixNano()), t.TempDir(), func(context.Context) error {
		calls++
		if calls == 1 {
			restartWanted = func() bool { return true }
		}
		return nil
	})
	if err != nil || calls != 2 {
		t.Fatalf("runs=%d, error=%v", calls, err)
	}
}
