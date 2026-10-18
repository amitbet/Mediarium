package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestWindowsServiceStopCancelsApplication(t *testing.T) {
	for _, cmd := range []svc.Cmd{svc.Stop, svc.Shutdown} {
		requests := make(chan svc.ChangeRequest, 1)
		status := make(chan svc.Status, 10)
		stopped := make(chan bool, 1)
		service := windowsService{run: func(ctx context.Context) error { <-ctx.Done(); return nil }}
		go func() { failed, code := service.Execute(nil, requests, status); stopped <- !failed && code == 0 }()
		if s := <-status; s.State != svc.StartPending {
			t.Fatalf("start state: %v", s)
		}
		if s := <-status; s.State != svc.Running || s.Accepts&svc.AcceptStop == 0 {
			t.Fatalf("running state: %v", s)
		}
		requests <- svc.ChangeRequest{Cmd: cmd}
		if s := <-status; s.State != svc.StopPending {
			t.Fatalf("stop state: %v", s)
		}
		select {
		case ok := <-stopped:
			if !ok {
				t.Fatal("stop reported failure")
			}
		case <-time.After(3 * time.Second):
			t.Fatal("service did not cancel application")
		}
	}
}

func TestWindowsServiceFailureReportedToSCM(t *testing.T) {
	status := make(chan svc.Status, 10)
	service := windowsService{run: func(context.Context) error { return errors.New("startup failed") }}
	failed, code := service.Execute(nil, make(chan svc.ChangeRequest), status)
	if !failed || code == 0 {
		t.Fatalf("failure lost: %v, %d", failed, code)
	}
}

func TestWindowsServiceRestartsApplicationOnRequest(t *testing.T) {
	requests := make(chan svc.ChangeRequest, 1)
	status := make(chan svc.Status, 10)
	done := make(chan bool, 1)
	started := make(chan struct{})
	calls := 0
	service := windowsService{run: func(ctx context.Context) error {
		calls++
		if calls == 1 {
			restartWanted = func() bool { return true }
			return nil
		}
		close(started)
		<-ctx.Done()
		return nil
	}}
	go func() { failed, code := service.Execute(nil, requests, status); done <- !failed && code == 0 }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("application was not restarted")
	}
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	select {
	case ok := <-done:
		if !ok {
			t.Fatal("restart reported failure")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("restarted application did not stop")
	}
	if calls != 2 {
		t.Fatalf("expected two runs, got %d", calls)
	}
}
