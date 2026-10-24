package torrentclient_test

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/anacrolix/torrent"

	"github.com/rdborg/mediarium/internal/speed"
	mediarium "github.com/rdborg/mediarium/internal/torrentclient"
)

// With uploading switched off in Settings (upload limit 0) a seeding torrent
// sends nothing, and starts again when the switch goes back.
func TestUploadSwitchedOffSendsNothing(t *testing.T) {
	t.Cleanup(func() { speed.SetUploadOff(false) })
	files := map[string][]byte{"movie.mkv": pattern(9, 1_000_000)}
	sw := newSwarm(t, files, 0)
	seedDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(seedDir, "Release"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seedDir, "Release", "movie.mkv"), files["movie.mkv"], 0o644); err != nil {
		t.Fatal(err)
	}
	engine, err := mediarium.New(mediarium.Config{DataDir: t.TempDir(), ListenPort: freePort(t)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close(); time.Sleep(100 * time.Millisecond) })
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	seeding, err := engine.AddTorrentFileIn(ctx, sw.torrent, seedDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := seeding.VerifyDataContext(ctx); err != nil {
		t.Fatal(err)
	}
	speed.SetUploadOff(true)
	ctl := mediarium.NewControl(seeding)
	go ctl.Run(ctx)
	time.Sleep(600 * time.Millisecond) // the control sees the switch

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = t.TempDir()
	cfg.NoDHT = true
	cfg.DisableTrackers = true
	cfg.NoDefaultPortForwarding = true
	cfg.ListenPort = 0
	leecher, err := torrent.NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer leecher.Close()
	lt, err := leecher.AddTorrent(sw.mi)
	if err != nil {
		t.Fatal(err)
	}
	lt.AddPeers([]torrent.PeerInfo{{Addr: stringAddr(net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.ListenPort())))}})
	lt.DownloadAll()

	time.Sleep(2500 * time.Millisecond)
	if got := lt.BytesCompleted(); got != 0 {
		t.Fatalf("with uploading off the seeder still gave away %d bytes", got)
	}

	speed.SetUploadOff(false)
	time.Sleep(600 * time.Millisecond) // the control sees it
	lt.AddPeers([]torrent.PeerInfo{{Addr: stringAddr(net.JoinHostPort("127.0.0.1", strconv.Itoa(engine.ListenPort())))}})
	waitUntil(t, "the download to finish once uploading is back", func() bool { return lt.BytesMissing() == 0 })
}
