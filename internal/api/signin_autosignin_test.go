package api_test

import (
	"net/http"
	"testing"
)

func TestAutoSignInSetting(t *testing.T) {
	e := newOIDCEnv(t)
	methods := func() map[string]any {
		return e.must(e.client(), "GET", "/api/auth/methods", nil, http.StatusOK)
	}

	// Off by default: the sign-in page does not redirect.
	if m := methods(); m["autoSignIn"] != nil {
		t.Errorf("on by default: %v", m)
	}
	st := e.must(e.admin, "GET", "/api/auth/sign-in", nil, http.StatusOK)
	if st["autoSignIn"] != "" {
		t.Errorf("settings say %v by default", st["autoSignIn"])
	}
	choices, _ := st["autoSignInChoices"].([]any)
	if len(choices) != 1 || choices[0].(map[string]any)["id"] != "oidc" || choices[0].(map[string]any)["name"] != "Authentik" {
		t.Errorf("choices %v", st["autoSignInChoices"])
	}

	// Anything but off or a working provider is refused.
	for _, bad := range []string{"ldap", "passkey", "OIDC", "https://evil.example"} {
		if got, _ := e.call(e.admin, "PUT", "/api/auth/sign-in", map[string]any{"autoSignIn": bad}); got != http.StatusBadRequest {
			t.Errorf("%q saved with status %d", bad, got)
		}
	}
	if m := methods(); m["autoSignIn"] != nil {
		t.Errorf("a refused value took effect: %v", m)
	}

	// Switched on, the sign-in page learns of it; local sign-in stays.
	e.must(e.admin, "PUT", "/api/auth/sign-in", map[string]any{"autoSignIn": "oidc"}, http.StatusOK)
	m := methods()
	if m["autoSignIn"] != "oidc" || m["local"] != true || m["oidc"] == nil {
		t.Errorf("methods %v", m)
	}

	// Not for someone who is not signed in.
	if got, _ := e.call(e.client(), "PUT", "/api/auth/sign-in", map[string]any{"autoSignIn": ""}); got == http.StatusOK {
		t.Errorf("an anonymous visitor changed the setting")
	}

	// A provider that is switched off takes the redirect with it, but the
	// saved choice comes back with the provider.
	e.saveConfig(map[string]any{"enabled": false})
	if m := methods(); m["autoSignIn"] != nil {
		t.Errorf("redirect still offered with the provider off: %v", m)
	}
	if got, _ := e.call(e.admin, "PUT", "/api/auth/sign-in", map[string]any{"autoSignIn": "oidc"}); got != http.StatusBadRequest {
		t.Errorf("chose a provider that is off: %d", got)
	}
	e.saveConfig(nil)
	if m := methods(); m["autoSignIn"] != "oidc" {
		t.Errorf("choice lost: %v", m)
	}

	// Off again.
	e.must(e.admin, "PUT", "/api/auth/sign-in", map[string]any{"autoSignIn": ""}, http.StatusOK)
	if m := methods(); m["autoSignIn"] != nil {
		t.Errorf("still on: %v", m)
	}
}
