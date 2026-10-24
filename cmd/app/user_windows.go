package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/windows"
)

// User mode has its own configuration and media, never the service's database.
// The named event permits graceful stop without a console or an API credential.
func handleUserMode(args []string) (bool, error) {
	if len(args) != 1 || (args[0] != "--user-mode" && args[0] != "--stop-user-mode") {
		return false, nil
	}
	root := filepath.Join(os.Getenv("LOCALAPPDATA"), "Mediarium")
	if !filepath.IsAbs(root) {
		return true, errors.New("LOCALAPPDATA must name an absolute directory")
	}
	defaults := map[string]string{
		"CONFIG_DIR": filepath.Join(root, "config"), "APP_PORT": "8265",
		"MEDIARIUM_SUPERVISED": "1",
	}
	for key, folder := range map[string]string{"DOWNLOADS_DIR": "downloads", "MOVIES_DIR": "movies", "TV_DIR": "series", "MUSIC_DIR": "music", "EBOOKS_DIR": "ebooks", "AUDIOBOOKS_DIR": "audiobooks"} {
		defaults[key] = filepath.Join(root, "data", folder)
	}
	for key, value := range defaults {
		if os.Getenv(key) == "" {
			if err := os.Setenv(key, value); err != nil {
				return true, err
			}
		}
	}
	configDir := os.Getenv("CONFIG_DIR")
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return true, err
	}
	name := fmt.Sprintf(`Local\MediariumUser-%x`, sha256.Sum256([]byte(user.User.Sid.String()+strings.ToLower(filepath.Clean(configDir)))))
	if args[0] == "--stop-user-mode" {
		return true, stopUserApplication(name, configDir)
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return true, err
	}
	f, err := os.OpenFile(filepath.Join(configDir, "user.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return true, err
	}
	defer f.Close()
	os.Stderr = f
	log.SetOutput(f)
	exe, err := os.Executable()
	if err != nil {
		return true, err
	}
	if err := os.Setenv("PATH", filepath.Join(filepath.Dir(exe), "tools")+";"+os.Getenv("PATH")); err != nil {
		return true, err
	}
	err = runUserApplication(name, configDir, run)
	if err != nil {
		log.Printf("Mediarium user mode failed: %v", err)
	}
	return true, err
}

func runUserApplication(name, configDir string, runApp func(context.Context) error) error {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	event, err := windows.CreateEvent(nil, 1, 0, namePtr)
	if event != 0 {
		defer windows.CloseHandle(event)
	}
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return nil // A second shortcut click must not open the database again.
	}
	if err != nil {
		return err
	}
	pidFile := filepath.Join(configDir, "user.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		return err
	}
	defer os.Remove(pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waitDone := make(chan struct{})
	defer func() { <-waitDone }()
	defer windows.SetEvent(event)
	go func() {
		defer close(waitDone)
		_, _ = windows.WaitForSingleObject(event, windows.INFINITE)
		cancel()
	}()
	for {
		stopBudget = shutdownClock{}
		restartWanted = nil
		if err := runApp(ctx); err != nil {
			return err
		}
		if ctx.Err() != nil || restartWanted == nil || !restartWanted() {
			return nil
		}
	}
}

func stopUserApplication(name, configDir string) error {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, namePtr)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	b, err := os.ReadFile(filepath.Join(configDir, "user.pid"))
	if err != nil {
		return err
	}
	pid, err := strconv.ParseUint(strings.TrimSpace(string(b)), 10, 32)
	if err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	buf := make([]uint16, 32768)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(process, 0, &buf[0], &size); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Clean(windows.UTF16ToString(buf[:size])), filepath.Clean(exe)) {
		return errors.New("user process belongs to another executable; refusing to stop it")
	}
	if err := windows.SetEvent(event); err != nil {
		return err
	}
	state, err := windows.WaitForSingleObject(process, uint32((45*time.Second)/time.Millisecond))
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return errors.New("user process did not finish its graceful shutdown within 45 seconds")
	}
	return nil
}
