package api

import "testing"

func TestValidAutoSignIn(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		oidcWorks  bool
		wantOK     bool
		wantProble bool
	}{
		{"off is always fine", "", false, true, false},
		{"off while sso works", "", true, true, false},
		{"sso while it works", "oidc", true, true, false},
		{"sso while it is off or unfinished", "oidc", false, false, true},
		{"a method that is not a redirect", "ldap", true, false, true},
		{"a passkey is not a redirect either", "passkey", true, false, true},
		{"odd casing is not accepted", "OIDC", true, false, true},
		{"junk", "https://evil.example", true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ok, problem := validAutoSignIn(tc.value, tc.oidcWorks)
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if (problem != "") != tc.wantProble {
				t.Errorf("problem = %q, want a problem: %v", problem, tc.wantProble)
			}
		})
	}
}
