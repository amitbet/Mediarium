package api

import (
	"errors"
	"log/slog"

	"github.com/rdborg/mediarium/internal/auth"
)

// groupRoles is what the groups of a sign-in say about one person: which
// groups the setup has at all, which of them the person is in, and the role
// new people and people who fall out of a group get.
type groupRoles struct {
	adminOn, viewerOn bool // the setup has an administrator / viewer group
	inAdmin, inViewer bool
	def               string // the default role: auth.RoleMember or auth.RoleViewer
}

// newAccountRole is the role of a person who signs in for the first time, and
// whether a group (rather than the default) gave it.
func (g groupRoles) newAccountRole() (role, groupRole string) {
	switch {
	case g.adminOn && g.inAdmin:
		return auth.RoleAdmin, ""
	case g.viewerOn && g.inViewer:
		return auth.RoleViewer, auth.RoleViewer
	}
	return g.def, ""
}

// existingAccountRole is the role an account that signs in again should have,
// and the group role to remember afterwards. current is the group role noted
// now (see auth.Service.GroupRole). In order:
//
//   - in the administrator group: administrator;
//   - else in the viewer group: viewer;
//   - else an administrator the administrator group no longer lists, and a
//     viewer the viewer group gave the role to and no longer lists, get the
//     default role;
//   - anyone else keeps the role they have, so a role set by hand is never
//     touched by a group that is not set up for it.
//
// An administrator is never turned into a viewer by a viewer group while the
// setup has no administrator group: nothing then says the role came from a group.
func (g groupRoles) existingAccountRole(u *auth.User, current string) (role, groupRole string) {
	if !g.adminOn && !g.viewerOn {
		return u.Role(), current
	}
	switch {
	case g.adminOn && g.inAdmin:
		return auth.RoleAdmin, ""
	case g.viewerOn && g.inViewer:
		if u.IsAdmin && !g.adminOn {
			return u.Role(), current
		}
		return auth.RoleViewer, auth.RoleViewer
	case g.adminOn && u.IsAdmin:
		return g.def, ""
	case g.viewerOn && current == auth.RoleViewer && u.Role() == auth.RoleViewer:
		return g.def, ""
	}
	return u.Role(), current
}

// applyGroupRoles brings the role of an account that signs in again in step
// with its groups.
func (s *Server) applyGroupRoles(u *auth.User, g groupRoles) {
	if !g.adminOn && !g.viewerOn {
		return
	}
	current, err := s.Auth.GroupRole(u.ID)
	if err != nil {
		slog.Error("auth: could not read where an account's role comes from", "user", u.Username, "err", err)
		return
	}
	want, mark := g.existingAccountRole(u, current)
	if !s.syncExternalRole(u, want) {
		return // the role could not change (for example the last administrator): leave everything as it is
	}
	if mark != current {
		if err := s.Auth.SetGroupRole(u.ID, mark); err != nil {
			slog.Error("auth: could not note where an account's role comes from", "user", u.Username, "err", err)
		}
	}
}

// syncExternalRole gives the account the role want. It reports false when the
// role could not be changed.
func (s *Server) syncExternalRole(u *auth.User, want string) bool {
	if want == u.Role() {
		return true
	}
	changed, err := s.Auth.SyncExternalRole(u.ID, want)
	switch {
	case errors.Is(err, auth.ErrLastAdmin):
		slog.Warn("auth: kept the last administrator, even though the sign-in provider no longer lists them as one", "user", u.Username)
		return false
	case err != nil:
		slog.Error("auth: could not change an account's role", "user", u.Username, "err", err)
		return false
	case changed:
		slog.Warn("auth: an account's role changed to follow the sign-in provider", "user", u.Username, "role", want)
	}
	return true
}
