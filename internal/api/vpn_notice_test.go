package api_test

import (
	"net/http"
	"testing"
)

// The "Torrenting without a VPN" notice can be dismissed for good by someone
// whose download client is already behind a VPN, and it comes back when they
// ask. It is not given at all while the built-in torrent client is off.
func TestVPNNoticeCanBeAcknowledged(t *testing.T) {
	_, base, client := loginNewServer(t)
	postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Tracker", "definitionId": "fixture", "baseUrl": "http://127.0.0.1:1", "protocol": "torrent",
	}, http.StatusCreated)

	h := healthByID(t, client, base)
	if h["no-vpn"] == nil || h["no-vpn"]["dismiss"] != "My download client is already behind a VPN" {
		t.Fatalf("the notice should offer to be dismissed: %+v", h["no-vpn"])
	}
	if s := getJSON[map[string]any](t, client, base+"/api/settings"); s["vpnNoticeAcknowledged"] != false {
		t.Fatalf("not acknowledged yet: %v", s["vpnNoticeAcknowledged"])
	}

	got := putJSONStatus(t, client, base+"/api/settings", map[string]any{"vpnNoticeAcknowledged": true}, http.StatusOK)
	if got["vpnNoticeAcknowledged"] != true {
		t.Fatalf("acknowledged: %v", got["vpnNoticeAcknowledged"])
	}
	if h = healthByID(t, client, base); h["no-vpn"] != nil {
		t.Fatalf("an acknowledged notice stays away: %+v", h["no-vpn"])
	}

	// An error about a kill switch with no VPN is not what was acknowledged.
	putJSONStatus(t, client, base+"/api/settings", map[string]any{"requireVpnForTorrents": true}, http.StatusOK)
	if h = healthByID(t, client, base); h["vpn-required"] == nil {
		t.Fatalf("the kill switch error still shows: %v", keysOf(h))
	}
	putJSONStatus(t, client, base+"/api/settings", map[string]any{"requireVpnForTorrents": false, "vpnNoticeAcknowledged": false}, http.StatusOK)
	if h = healthByID(t, client, base); h["no-vpn"] == nil {
		t.Fatalf("Show it again brings the notice back: %v", keysOf(h))
	}
}

// With the built-in torrent client off and an external one set up, the
// torrents do not run in Mediarium, so there is no notice either.
func TestVPNNoticeSkippedForAnExternalClientOnly(t *testing.T) {
	e := newClientEnv(t)
	e.save(t, map[string]any{})
	postJSON[map[string]any](t, e.client, e.url+"/api/indexers", map[string]any{
		"name": "Tracker", "definitionId": "fixture", "baseUrl": "http://127.0.0.1:1", "protocol": "torrent",
	}, http.StatusCreated)
	h := healthByID(t, e.client, e.url)
	if h["no-vpn"] != nil {
		t.Fatalf("an external client only: no notice expected: %+v", h["no-vpn"])
	}
	putJSONStatus(t, e.client, e.url+"/api/settings", map[string]any{"torrentEnabled": true}, http.StatusOK)
	if h = healthByID(t, e.client, e.url); h["no-vpn"] == nil {
		t.Fatalf("with the built-in client on, the notice is back: %v", keysOf(h))
	}
}
