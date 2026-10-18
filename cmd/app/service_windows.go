package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/svc"
)

// SCM starts this same executable; no wrapper or interactive session is needed.
func runAsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("detect Windows service: %v", err)
	}
	if !isService {
		return false
	}
	if dir := os.Getenv("CONFIG_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0700); err != nil {
			log.Fatal(err)
		}
		f, err := os.OpenFile(filepath.Join(dir, "service.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		os.Stderr = f
	}
	if err := svc.Run("Mediarium", windowsService{}); err != nil {
		log.Fatal(err)
	}
	return true
}

type windowsService struct {
	// Test hook for the real SCM cancellation and failure paths.
	run func(context.Context) error
}

func (service windowsService) Execute(_ []string, requests <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	runApp := service.run
	if runApp == nil {
		runApp = run
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	status <- svc.Status{State: svc.StartPending, WaitHint: 30000}
	done := make(chan error, 1)
	go func() {
		for {
			stopBudget = shutdownClock{}
			restartWanted = nil
			if err := runApp(ctx); err != nil {
				done <- err
				return
			}
			if ctx.Err() != nil || restartWanted == nil || !restartWanted() {
				done <- nil
				return
			}
		}
	}()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	status <- current
	for {
		select {
		case err := <-done:
			if err != nil {
				log.Printf("Mediarium service failed: %v", err)
				return true, 1
			}
			return false, 0
		case req := <-requests:
			switch req.Cmd {
			case svc.Interrogate:
				status <- current
			case svc.Stop, svc.Shutdown:
				current = svc.Status{State: svc.StopPending, WaitHint: 15000}
				status <- current
				cancel()
			}
		}
	}
}
