package api_test

import (
	"net/http"
	"testing"
)

func TestProfileQualityOrderRoundTrip(t *testing.T) {
	_, httpSrv, client := newHuntTestServer(t)
	postJSON[map[string]any](t, client, httpSrv.URL+"/api/onboarding/admin", map[string]string{
		"username": "ryan", "password": "correct-horse-battery-staple", "firstName": "Ryan", "lastName": "Tester",
	}, http.StatusCreated)
	base := httpSrv.URL + "/api/quality-profiles"

	// Presets have no order of their own.
	list := getJSON[map[string]any](t, client, base)
	for _, raw := range list["profiles"].([]any) {
		if order := raw.(map[string]any)["qualityOrder"].([]any); len(order) != 0 {
			t.Fatalf("a built-in profile has no order, got %+v", raw)
		}
	}

	order := []map[string]any{
		{"tiers": []string{"Bluray-1080p"}},
		{"name": "Web or TV", "tiers": []string{"HDTV-1080p", "WEBDL-1080p"}},
	}
	created := postJSON[map[string]any](t, client, base, map[string]any{
		"name": "Grouped", "cutoff": "HDTV-1080p", "upgradeAllowed": true, "qualityOrder": order,
	}, http.StatusCreated)
	id := int64(created["id"].(float64))
	got := created["qualityOrder"].([]any)
	if len(got) != 2 {
		t.Fatalf("order not kept: %+v", created)
	}
	group := got[1].(map[string]any)
	if group["name"] != "Web or TV" || len(group["tiers"].([]any)) != 2 || group["tiers"].([]any)[0] != "WEBDL-1080p" {
		t.Fatalf("the group comes back with its qualities best first, got %+v", group)
	}
	if allowed := created["allowed"].([]any); len(allowed) != 3 {
		t.Fatalf("allowed is worked out from the order, got %+v", allowed)
	}

	url := base + "/" + itoa(id)
	// An update that leaves the order out keeps it; sending [] clears it.
	updated := putJSONStatus(t, client, url, map[string]any{"name": "Grouped", "allowed": []string{"HDTV-1080p", "WEBDL-1080p", "Bluray-1080p"}, "cutoff": "HDTV-1080p", "upgradeAllowed": true}, http.StatusOK)
	if len(updated["qualityOrder"].([]any)) != 2 {
		t.Fatalf("leaving the order out should keep it, got %+v", updated)
	}
	cleared := putJSONStatus(t, client, url, map[string]any{"name": "Grouped", "allowed": []string{"HDTV-1080p", "WEBDL-1080p"}, "cutoff": "HDTV-1080p", "qualityOrder": []any{}}, http.StatusOK)
	if len(cleared["qualityOrder"].([]any)) != 0 || len(cleared["allowed"].([]any)) != 2 {
		t.Fatalf("[] goes back to the built-in order with the sent allowed list, got %+v", cleared)
	}

	// Bad orders are refused.
	for name, bad := range map[string][]map[string]any{
		"a tier twice":      {{"tiers": []string{"Bluray-1080p"}}, {"name": "G", "tiers": []string{"Bluray-1080p", "DVD"}}},
		"unnamed group":     {{"tiers": []string{"Bluray-1080p", "DVD"}}},
		"unknown quality":   {{"tiers": []string{"Bluray-8K"}}},
		"cutoff not listed": {{"tiers": []string{"DVD"}}},
	} {
		t.Run(name, func(t *testing.T) {
			postJSON[map[string]any](t, client, base, map[string]any{"name": "Bad " + name, "cutoff": "Bluray-1080p", "qualityOrder": bad}, http.StatusBadRequest)
		})
	}
}
