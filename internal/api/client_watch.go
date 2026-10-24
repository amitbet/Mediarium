package api

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/rdborg/mediarium/internal/download"
	"github.com/rdborg/mediarium/internal/netguard"
	"github.com/rdborg/mediarium/internal/notify"
	"github.com/rdborg/mediarium/internal/plainerror"
	"github.com/rdborg/mediarium/internal/qbit"
	"github.com/rdborg/mediarium/internal/usenetclient"
)

// The download clients that are watched. The key tells them apart; the label
// is how a message names them.
const (
	watchTorrentClient = "torrent-client"
	watchUsenetClient  = "usenet-client"
	watchBuiltinUsenet = "builtin-usenet"
)

const builtinUsenetLabel = "The built-in Usenet downloader"

// clientUnreachable reports whether err means the client could not be reached at
// all, as opposed to a client that answered and did not like the request (a
// wrong password, a missing category). Only the first counts against it.
func clientUnreachable(err error) bool {
	var cat *usenetclient.CategoryError
	var refused *usenetclient.RefusedError
	switch {
	case err == nil:
		return false
	case errors.Is(err, context.Canceled),
		errors.Is(err, qbit.ErrAuth), errors.Is(err, usenetclient.ErrAuth), errors.Is(err, usenetclient.ErrNotThisClient),
		errors.Is(err, netguard.ErrBlocked),
		errors.As(err, &cat), errors.As(err, &refused):
		return false
	}
	return true
}

// builtinUsenetUnreachable reports whether a failed built-in download is about
// reaching the Usenet servers (not the release, not the disk).
func builtinUsenetUnreachable(err error) bool {
	var rel *download.ReleaseError
	var nerr net.Error
	return err != nil && !errors.As(err, &rel) && errors.As(err, &nerr)
}

// noteClientCheck records the outcome of one status request to a download
// client and sends a "down" or "back" message when that changes the picture
// (see notify.ClientWatch for when). A client that answered with a refusal is
// not counted either way. Called from the places that ask the client anyway.
func (s *Server) noteClientCheck(key, label string, err error) {
	if err != nil && !clientUnreachable(err) {
		return
	}
	switch v := s.clientWatch.Note(key, err == nil); v.Change {
	case notify.ClientWentDown:
		s.sendNotification(notify.ComposeClientDown(label, plainerror.Message(netguard.CleanError(err)), s.publicLinks(), time.Now()))
	case notify.ClientCameBack:
		s.sendNotification(notify.ComposeClientUp(label, v.Outage, s.publicLinks(), time.Now()))
	}
}
