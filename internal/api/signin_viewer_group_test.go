package api_test

import (
	"net/http"
	"testing"
)

// groupStep is one sign-in of the same person: first an administrator may set
// the role by hand, then the person signs in with these groups.
type groupStep struct {
	hand   string   // a role an administrator gives the account before this sign-in ("" = none)
	groups []string // what the provider says
	want   string   // the role after the sign-in
}

func TestOIDCViewerGroup(t *testing.T) {
	both := map[string]any{"adminGroup": "media-admins", "viewerGroup": "media-viewers"}
	tests := []struct {
		name  string
		cfg   map[string]any
		steps []groupStep
	}{
		{"new person in the viewer group", both, []groupStep{{groups: []string{"media-viewers"}, want: "viewer"}}},
		{"new person in the admin group", both, []groupStep{{groups: []string{"media-admins"}, want: "admin"}}},
		{"new person in both groups", both, []groupStep{{groups: []string{"media-viewers", "media-admins"}, want: "admin"}}},
		{"new person in neither", both, []groupStep{{groups: []string{"family"}, want: "member"}}},
		{"new person in neither, default viewer", map[string]any{"adminGroup": "media-admins", "viewerGroup": "media-viewers", "defaultRole": "viewer"},
			[]groupStep{{groups: []string{"family"}, want: "viewer"}}},
		{"removed from the viewer group falls back to the default", both, []groupStep{
			{groups: []string{"media-viewers"}, want: "viewer"},
			{groups: []string{"family"}, want: "member"},
		}},
		{"viewer promoted to admin, then dropped to the default", both, []groupStep{
			{groups: []string{"media-viewers"}, want: "viewer"},
			{groups: []string{"media-viewers", "media-admins"}, want: "admin"},
			{groups: []string{"family"}, want: "member"},
		}},
		{"admin moved into the viewer group", both, []groupStep{
			{groups: []string{"media-admins"}, want: "admin"},
			{groups: []string{"media-viewers"}, want: "viewer"},
		}},
		{"a member in the viewer group and back", both, []groupStep{
			{groups: []string{"family"}, want: "member"},
			{groups: []string{"media-viewers"}, want: "viewer"},
			{groups: []string{"family"}, want: "member"},
		}},
		{"a viewer by hand stays one when the group is set up", both, []groupStep{
			{groups: []string{"family"}, want: "member"},
			{hand: "viewer", groups: []string{"family"}, want: "viewer"},
		}},
		{"a role set by hand after the group gave one is kept", both, []groupStep{
			{groups: []string{"media-viewers"}, want: "viewer"},
			{hand: "member", groups: []string{"family"}, want: "member"},
		}},
		{"a viewer by hand follows the group once it lists them, and is released again", both, []groupStep{
			{groups: []string{"family"}, want: "member"},
			{hand: "viewer", groups: []string{"media-viewers"}, want: "viewer"},
			{groups: []string{"family"}, want: "member"},
		}},
		{"no groups set up: the role by hand and the groups mean nothing", map[string]any{"adminGroup": "", "viewerGroup": ""}, []groupStep{
			{groups: []string{"media-viewers", "media-admins"}, want: "member"},
			{hand: "viewer", groups: []string{"family"}, want: "viewer"},
			{groups: []string{"media-admins"}, want: "viewer"},
			{hand: "member", groups: []string{"media-viewers"}, want: "member"},
		}},
		{"only a viewer group: a member by hand stays out, an admin by hand is never made a viewer", map[string]any{"viewerGroup": "media-viewers"}, []groupStep{
			{groups: []string{"family"}, want: "member"},
			{hand: "admin", groups: []string{"family"}, want: "admin"},
			{groups: []string{"media-viewers"}, want: "admin"},
		}},
		{"only an admin group: a viewer by hand is left alone", map[string]any{"adminGroup": "media-admins"}, []groupStep{
			{groups: []string{"family"}, want: "member"},
			{hand: "viewer", groups: []string{"media-viewers"}, want: "viewer"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newOIDCEnv(t)
			e.saveConfig(map[string]any{"adminGroup": "", "viewerGroup": ""})
			e.saveConfig(tc.cfg)
			for i, st := range tc.steps {
				if st.hand != "" {
					id := e.scalar(`SELECT id FROM users WHERE username = 'sam'`)
					e.must(e.admin, "PUT", "/api/users/"+id, map[string]string{"role": st.hand}, http.StatusOK)
				}
				groups := st.groups
				e.idp.Mutate = func(c map[string]any) { c["groups"] = groups }
				b := e.client()
				e.browse(b, "")
				if u, role := e.signedInAs(b); u != "sam" || role != st.want {
					t.Fatalf("step %d: signed in as %q with role %q, want %q", i+1, u, role, st.want)
				}
			}
		})
	}
}

func TestOIDCViewerGroupNeedsAnotherGroupThanTheAdminGroup(t *testing.T) {
	e := newOIDCEnv(t)
	e.must(e.admin, "PUT", "/api/auth/oidc", e.config(map[string]any{"adminGroup": "same", "viewerGroup": "same"}), http.StatusBadRequest)
	e.must(e.admin, "PUT", "/api/auth/oidc", e.config(map[string]any{"enabled": false, "adminGroup": "same", "viewerGroup": " same "}), http.StatusBadRequest)
	got := e.must(e.admin, "PUT", "/api/auth/oidc", e.config(map[string]any{"adminGroup": "a", "viewerGroup": "b"}), http.StatusOK)
	if got["viewerGroup"] != "b" {
		t.Errorf("saved viewerGroup = %v", got["viewerGroup"])
	}
	// Empty is off.
	e.must(e.admin, "PUT", "/api/auth/oidc", e.config(map[string]any{"adminGroup": "", "viewerGroup": ""}), http.StatusOK)
}

func TestOIDCViewerGroupNeverDemotesTheLastAdministrator(t *testing.T) {
	e := newOIDCEnv(t)
	e.saveConfig(map[string]any{"adminGroup": "media-admins", "viewerGroup": "media-viewers"})
	b := e.client()
	e.browse(b, "")
	ryan := e.scalar(`SELECT id FROM users WHERE username = 'ryan'`)
	e.must(b, "PUT", "/api/users/"+ryan, map[string]string{"role": "member"}, http.StatusOK)
	// sam is the last administrator; the provider now puts him in the viewer group only.
	e.idp.Mutate = func(c map[string]any) { c["groups"] = []string{"media-viewers"} }
	b2 := e.client()
	e.browse(b2, "")
	if _, role := e.signedInAs(b2); role != "admin" {
		t.Errorf("the last administrator became %s", role)
	}
	if got := e.scalar(`SELECT group_role FROM users WHERE username = 'sam'`); got != "" {
		t.Errorf("group_role = %q, the role did not come from the viewer group", got)
	}
}

func TestLDAPViewerGroupFilter(t *testing.T) {
	const adminF = "(&(objectClass=groupOfNames)(cn=mediarium-admins)(member={dn}))"
	const viewerF = "(&(objectClass=groupOfNames)(cn=mediarium-viewers)(member={dn}))"
	both := map[string]any{"adminGroupFilter": adminF, "viewerGroupFilter": viewerF}
	type ldapStep struct {
		hand          string
		admin, viewer bool
		want          string
	}
	tests := []struct {
		name  string
		cfg   map[string]any
		steps []ldapStep
	}{
		{"new person in the viewer group", both, []ldapStep{{viewer: true, want: "viewer"}}},
		{"new person in the admin group", both, []ldapStep{{admin: true, want: "admin"}}},
		{"new person in both", both, []ldapStep{{admin: true, viewer: true, want: "admin"}}},
		{"new person in neither", both, []ldapStep{{want: "member"}}},
		{"removed from the viewer group", both, []ldapStep{{viewer: true, want: "viewer"}, {want: "member"}}},
		{"admin moves to the viewer group", both, []ldapStep{{admin: true, want: "admin"}, {viewer: true, want: "viewer"}}},
		{"viewer by hand is left alone", both, []ldapStep{{want: "member"}, {hand: "viewer", want: "viewer"}}},
		{"role by hand after the group is kept", both, []ldapStep{{viewer: true, want: "viewer"}, {hand: "member", want: "member"}}},
		{"only a viewer filter: not in it stays a member", map[string]any{"viewerGroupFilter": viewerF}, []ldapStep{{want: "member"}, {viewer: true, want: "viewer"}, {want: "member"}}},
		{"no filters: groups mean nothing", map[string]any{"adminGroupFilter": "", "viewerGroupFilter": ""}, []ldapStep{{admin: true, viewer: true, want: "member"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newLDAPEnv(t)
			cfg := map[string]any{"groupBaseDn": e.dir.groupsDN}
			for k, v := range tc.cfg {
				cfg[k] = v
			}
			e.saveConfig(cfg)
			vic := e.dir.users[2]
			for i, st := range tc.steps {
				if st.hand != "" {
					id := e.scalar(`SELECT id FROM users WHERE username = 'vic'`)
					e.must(e.admin, "PUT", "/api/users/"+id, map[string]string{"role": st.hand}, http.StatusOK)
				}
				e.dir.mu.Lock()
				vic.admin, vic.viewer = st.admin, st.viewer
				e.dir.mu.Unlock()
				c, status, _ := e.login("vic", "vic-password")
				if u, role := e.signedInAs(c); status != http.StatusOK || u != "vic" || role != st.want {
					t.Fatalf("step %d: %d, %q with role %q, want %q", i+1, status, u, role, st.want)
				}
			}
		})
	}
}

func TestLDAPViewerGroupFilterIsChecked(t *testing.T) {
	e := newLDAPEnv(t)
	const f = "(&(objectClass=groupOfNames)(cn=g)(member={dn}))"
	e.must(e.admin, "PUT", "/api/auth/ldap", e.config(map[string]any{"adminGroupFilter": f, "viewerGroupFilter": f}), http.StatusBadRequest)
	e.must(e.admin, "PUT", "/api/auth/ldap", e.config(map[string]any{"viewerGroupFilter": "(cn=viewers)"}), http.StatusBadRequest) // says nothing about the person
	e.must(e.admin, "PUT", "/api/auth/ldap", e.config(map[string]any{"viewerGroupFilter": "(member={dn}"}), http.StatusBadRequest) // not a filter
	e.must(e.admin, "PUT", "/api/auth/ldap", e.config(map[string]any{"adminGroupFilter": f, "viewerGroupFilter": "(&(cn=v)(member={dn}))", "groupBaseDn": e.dir.groupsDN}), http.StatusOK)
}
