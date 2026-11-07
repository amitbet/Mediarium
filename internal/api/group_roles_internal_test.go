package api

import (
	"testing"

	"github.com/rdborg/mediarium/internal/auth"
)

func TestGroupRolesForExistingAccounts(t *testing.T) {
	admin := &auth.User{IsAdmin: true}
	viewer := &auth.User{Viewer: true}
	member := &auth.User{}
	both := groupRoles{adminOn: true, viewerOn: true, def: auth.RoleMember}
	onlyAdmin := groupRoles{adminOn: true, def: auth.RoleMember}
	onlyViewer := groupRoles{viewerOn: true, def: auth.RoleMember}
	none := groupRoles{def: auth.RoleMember}
	with := func(g groupRoles, inAdmin, inViewer bool, def string) groupRoles {
		g.inAdmin, g.inViewer = inAdmin, inViewer
		if def != "" {
			g.def = def
		}
		return g
	}

	tests := []struct {
		name      string
		g         groupRoles
		u         *auth.User
		current   string
		wantRole  string
		wantGroup string
	}{
		{"in the admin group", with(both, true, false, ""), member, "", auth.RoleAdmin, ""},
		{"in the viewer group", with(both, false, true, ""), member, "", auth.RoleViewer, auth.RoleViewer},
		{"in both groups: admin wins", with(both, true, true, ""), member, "", auth.RoleAdmin, ""},
		{"in both groups, was a group viewer", with(both, true, true, ""), viewer, auth.RoleViewer, auth.RoleAdmin, ""},
		{"in neither: a member stays a member", with(both, false, false, ""), member, "", auth.RoleMember, ""},
		{"in neither: an admin falls back to the default", with(both, false, false, ""), admin, "", auth.RoleMember, ""},
		{"in neither: an admin falls back to a viewer default", with(both, false, false, auth.RoleViewer), admin, "", auth.RoleViewer, ""},
		{"left the viewer group: back to the default", with(both, false, false, ""), viewer, auth.RoleViewer, auth.RoleMember, ""},
		{"left the viewer group, default viewer", with(both, false, false, auth.RoleViewer), viewer, auth.RoleViewer, auth.RoleViewer, ""},
		{"viewer by hand is not touched", with(both, false, false, ""), viewer, "", auth.RoleViewer, ""},
		{"viewer by hand who is in the group becomes group-managed", with(both, false, true, ""), viewer, "", auth.RoleViewer, auth.RoleViewer},
		{"admin leaves the admin group into the viewer group", with(both, false, true, ""), admin, "", auth.RoleViewer, auth.RoleViewer},
		{"no groups set up: nothing changes (admin)", with(none, true, true, ""), admin, "", auth.RoleAdmin, ""},
		{"no groups set up: nothing changes (viewer)", with(none, true, true, ""), viewer, "", auth.RoleViewer, ""},
		{"no groups set up: nothing changes (member)", with(none, true, true, ""), member, "", auth.RoleMember, ""},
		{"only an admin group: a viewer by hand stays", with(onlyAdmin, false, true, ""), viewer, "", auth.RoleViewer, ""},
		{"only an admin group: a member stays", with(onlyAdmin, false, false, ""), member, "", auth.RoleMember, ""},
		{"only an admin group: an admin falls back", with(onlyAdmin, false, false, ""), admin, "", auth.RoleMember, ""},
		{"only a viewer group: a hand-made admin stays", with(onlyViewer, false, false, ""), admin, "", auth.RoleAdmin, ""},
		{"only a viewer group: a hand-made admin in the group stays", with(onlyViewer, false, true, ""), admin, "", auth.RoleAdmin, ""},
		{"only a viewer group: the admin group's name means nothing", with(onlyViewer, true, false, ""), member, "", auth.RoleMember, ""},
		{"only a viewer group: member joins", with(onlyViewer, false, true, ""), member, "", auth.RoleViewer, auth.RoleViewer},
		{"only a viewer group: member by hand stays out", with(onlyViewer, false, false, ""), member, "", auth.RoleMember, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			role, group := tc.g.existingAccountRole(tc.u, tc.current)
			if role != tc.wantRole || group != tc.wantGroup {
				t.Errorf("got role %q (from group %q), want %q (%q)", role, group, tc.wantRole, tc.wantGroup)
			}
		})
	}
}

func TestGroupRolesForNewAccounts(t *testing.T) {
	tests := []struct {
		name      string
		g         groupRoles
		wantRole  string
		wantGroup string
	}{
		{"admin group", groupRoles{adminOn: true, viewerOn: true, inAdmin: true, def: "member"}, auth.RoleAdmin, ""},
		{"viewer group", groupRoles{adminOn: true, viewerOn: true, inViewer: true, def: "member"}, auth.RoleViewer, auth.RoleViewer},
		{"both", groupRoles{adminOn: true, viewerOn: true, inAdmin: true, inViewer: true, def: "member"}, auth.RoleAdmin, ""},
		{"neither", groupRoles{adminOn: true, viewerOn: true, def: "member"}, auth.RoleMember, ""},
		{"neither, default viewer", groupRoles{adminOn: true, viewerOn: true, def: "viewer"}, auth.RoleViewer, ""},
		{"in a group that is not set up", groupRoles{inAdmin: true, inViewer: true, def: "member"}, auth.RoleMember, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			role, group := tc.g.newAccountRole()
			if role != tc.wantRole || group != tc.wantGroup {
				t.Errorf("got role %q (from group %q), want %q (%q)", role, group, tc.wantRole, tc.wantGroup)
			}
		})
	}
}
