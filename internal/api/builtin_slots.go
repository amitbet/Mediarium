package api

import (
	"context"
	"sync"
)

// "Downloads at the same time" can be set up to 500 so a download client such
// as qBittorrent is not held back. The built-in engines are not meant for that:
// each Usenet download opens a worker per connection and holds its parsed NZB,
// and each built-in torrent opens peer connections and keeps pieces in memory.
// So the built-in engines keep their own cap, whatever the shared number says.
// At 5 or fewer nothing changes, because these caps never go below 5.
const (
	builtinMinSlots     = 5  // never held below the old maximum
	builtinTorrentSlots = 20 // built-in torrents downloading side by side
	builtinUsenetMax    = 50 // however many connections a provider allows
)

// slotGate lets at most limit() holders in at once and lets the rest wait in
// the order they came. The limit is asked again at every change, so it can
// follow a setting. The zero value is ready to use.
type slotGate struct {
	mu      sync.Mutex
	held    int
	waiting []chan struct{}
}

// acquire waits for a place and returns the function that gives it back (safe
// to call more than once). It returns ctx's error if the download is stopped
// while it waits.
func (g *slotGate) acquire(ctx context.Context, limit func() int) (func(), error) {
	g.mu.Lock()
	if len(g.waiting) == 0 && g.held < max(limit(), 1) {
		g.held++
		g.mu.Unlock()
		return g.releaser(limit), nil
	}
	w := make(chan struct{})
	g.waiting = append(g.waiting, w)
	g.mu.Unlock()

	select {
	case <-w:
		return g.releaser(limit), nil
	case <-ctx.Done():
		g.mu.Lock()
		defer g.mu.Unlock()
		for i, x := range g.waiting {
			if x == w {
				g.waiting = append(g.waiting[:i], g.waiting[i+1:]...)
				return nil, ctx.Err()
			}
		}
		// A place was handed over in the same moment: give it on.
		g.held--
		g.grantLocked(limit)
		return nil, ctx.Err()
	}
}

func (g *slotGate) releaser(limit func() int) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			g.held--
			g.grantLocked(limit)
		})
	}
}

func (g *slotGate) grantLocked(limit func() int) {
	for len(g.waiting) > 0 && g.held < max(limit(), 1) {
		w := g.waiting[0]
		g.waiting = g.waiting[1:]
		g.held++
		close(w)
	}
}

// builtinTorrentLimit is how many built-in torrents may download at once.
func builtinTorrentLimit() int { return builtinTorrentSlots }

// builtinUsenetLimit is how many built-in Usenet downloads may run at once:
// more than the provider's connections would only wait for them. Never below
// builtinMinSlots, so the usual small settings behave as before.
func (s *Server) builtinUsenetLimit() int {
	most := 0
	if servers, err := s.ClientRepo.List(); err == nil {
		for _, sc := range servers {
			most = max(most, sc.Config.Connections)
		}
	}
	return min(max(most, builtinMinSlots), builtinUsenetMax)
}
