package notify

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// A download client counts as down after this many failed checks in a row, over at
// least ClientDownMinSpan (the lists on the Activity page ask every few
// seconds, and a client that restarts is gone for a moment).
const ClientDownAfter = 3

// ClientDownMinSpan is how long the failed checks must stretch over.
const ClientDownMinSpan = time.Minute

// A client's "down" message is held back until this long after its last
// message, so one that keeps going up and down does not fill the channel.
const ClientMessageGap = 10 * time.Minute

// ClientChange is what a check of a download client changed.
type ClientChange int

const (
	ClientSame     ClientChange = iota // nothing to tell
	ClientWentDown                     // it is now down and a message should go out
	ClientCameBack                     // it is back after a "down" message and a message should go out
)

// ClientVerdict is the answer to one check.
type ClientVerdict struct {
	Change ClientChange
	Outage time.Duration // for ClientCameBack: how long it was down
}

// ClientWatch follows the checks of download clients (the status requests
// Mediarium makes anyway) and says when a client went down and when it came
// back. It tells about an outage once: one "down" message when the third
// check in a row fails, one "back" message when a check works again, and a
// new "down" message never sooner than ClientMessageGap after the last one.
// A "back" message only follows a "down" message that was sent.
type ClientWatch struct {
	// Now is the clock (default time.Now); tests replace it.
	Now func() time.Time

	mu      sync.Mutex
	clients map[string]*clientState
}

type clientState struct {
	fails     int
	firstFail time.Time
	down      bool      // three checks failed in a row and none worked since
	announced bool      // the "down" message for this outage was sent
	lastMsg   time.Time // when the last message about this client was sent
}

func (w *ClientWatch) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// Note records one check of the client named key: ok is true when the client
// answered.
func (w *ClientWatch) Note(key string, ok bool) ClientVerdict {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.clients == nil {
		w.clients = map[string]*clientState{}
	}
	st := w.clients[key]
	if st == nil {
		st = &clientState{}
		w.clients[key] = st
	}
	now := w.now()
	if ok {
		out := ClientVerdict{}
		if st.down && st.announced {
			out = ClientVerdict{Change: ClientCameBack, Outage: now.Sub(st.firstFail)}
			st.lastMsg = now
		}
		*st = clientState{lastMsg: st.lastMsg}
		return out
	}
	if st.fails == 0 {
		st.firstFail = now
	}
	st.fails++
	if st.fails < ClientDownAfter || now.Sub(st.firstFail) < ClientDownMinSpan {
		return ClientVerdict{}
	}
	st.down = true
	if !st.announced && (st.lastMsg.IsZero() || now.Sub(st.lastMsg) >= ClientMessageGap) {
		st.announced = true
		st.lastMsg = now
		return ClientVerdict{Change: ClientWentDown}
	}
	return ClientVerdict{}
}

// ComposeClientDown is the message for a client that can't be reached. reason
// is what went wrong, in plain words ("connection refused").
func ComposeClientDown(label, reason string, links Links, at time.Time) Event {
	msg := label + " can't be reached"
	if r := plainReason(reason); r != "" {
		msg += ": " + r
	}
	ev := ComposeText(EventClientDown, label+" can't be reached", msg+".", links, at)
	ev.LinkURL = links.to("/activity")
	return ev
}

// ComposeClientUp is the message for a client that can be reached again.
func ComposeClientUp(label string, outage time.Duration, links Links, at time.Time) Event {
	ev := ComposeText(EventClientUp, label+" is reachable again", fmt.Sprintf("%s is reachable again after %s.", label, outageText(outage)), links, at)
	ev.LinkURL = links.to("/activity")
	return ev
}

// plainReason tidies a reason so it reads after a colon: one line, no capital
// at the start and no full stop at the end.
func plainReason(s string) string {
	s = strings.TrimSpace(strings.Join(strings.Fields(s), " "))
	s = strings.TrimRight(s, ". ")
	if r := []rune(s); len(r) > 1 && r[0] >= 'A' && r[0] <= 'Z' && r[1] >= 'a' && r[1] <= 'z' {
		s = strings.ToLower(string(r[0])) + string(r[1:])
	}
	return clip(s, 200)
}

// outageText says how long an outage lasted: "less than a minute", "12
// minutes", "1 hour 5 minutes".
func outageText(d time.Duration) string {
	mins := int((d + 30*time.Second) / time.Minute)
	switch {
	case mins < 1:
		return "less than a minute"
	case mins == 1:
		return "1 minute"
	case mins < 60:
		return fmt.Sprintf("%d minutes", mins)
	}
	h, m := mins/60, mins%60
	hours := fmt.Sprintf("%d hours", h)
	if h == 1 {
		hours = "1 hour"
	}
	switch m {
	case 0:
		return hours
	case 1:
		return hours + " 1 minute"
	}
	return fmt.Sprintf("%s %d minutes", hours, m)
}
