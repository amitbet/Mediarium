package api

import (
	"context"
	"testing"
	"time"
)

func TestSlotGateHoldsBackExtraDownloadsInOrder(t *testing.T) {
	var g slotGate
	limit := func() int { return 2 }
	ctx := context.Background()

	r1, _ := g.acquire(ctx, limit)
	r2, _ := g.acquire(ctx, limit)

	order := make(chan int, 2)
	release := make(chan func(), 2)
	for i := 3; i <= 4; i++ {
		go func() {
			r, err := g.acquire(ctx, limit)
			if err != nil {
				t.Error(err)
				return
			}
			order <- i
			release <- r
		}()
		time.Sleep(20 * time.Millisecond) // so they line up 3 then 4
	}
	select {
	case n := <-order:
		t.Fatalf("download %d started while both places were taken", n)
	case <-time.After(50 * time.Millisecond):
	}
	r1()
	r1() // giving back twice must not free a second place
	if got := <-order; got != 3 {
		t.Fatalf("first to start after a release was %d, want 3", got)
	}
	select {
	case n := <-order:
		t.Fatalf("download %d started on one freed place", n)
	case <-time.After(50 * time.Millisecond):
	}
	r2()
	if got := <-order; got != 4 {
		t.Fatalf("next to start was %d, want 4", got)
	}
	(<-release)()
	(<-release)()
}

func TestSlotGateWaiterThatIsStoppedGivesUpItsTurn(t *testing.T) {
	var g slotGate
	limit := func() int { return 1 }
	r1, _ := g.acquire(context.Background(), limit)

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { _, err := g.acquire(ctx, limit); errc <- err }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	if err := <-errc; err == nil {
		t.Fatal("a stopped download should not get a place")
	}
	r1()
	// The place is free again for the next one.
	done := make(chan struct{})
	go func() {
		r, _ := g.acquire(context.Background(), limit)
		r()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the place was lost when a waiting download was stopped")
	}
}

func TestBuiltinCapsNeverGoBelowTheOldMaximum(t *testing.T) {
	if builtinTorrentLimit() < builtinMinSlots || builtinUsenetMax < builtinMinSlots {
		t.Fatalf("the built-in caps (%d torrents, up to %d Usenet) must stay at %d or more", builtinTorrentLimit(), builtinUsenetMax, builtinMinSlots)
	}
}
