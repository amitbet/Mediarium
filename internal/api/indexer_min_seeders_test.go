package api_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// A torrent indexer can be given a minimum number of seeders; it shows on the
// indexer's card, a change that leaves it out keeps it, and a bad number is
// refused in plain words.
func TestIndexerMinSeeders(t *testing.T) {
	_, base, client := loginNewServer(t)
	idx := newTVIndexerWith(t, nil)

	created := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Tor", "definitionId": "fixture", "baseUrl": idx.URL, "apiKey": "k", "protocol": "torrent", "minSeeders": 5,
	}, http.StatusCreated)
	id := int64(created["id"].(float64))
	if created["minSeeders"] != float64(5) {
		t.Fatalf("the minimum should show on the card: %+v", created)
	}

	plain := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Plain", "definitionId": "fixture", "baseUrl": idx.URL, "apiKey": "k", "protocol": "torrent",
	}, http.StatusCreated)
	if plain["minSeeders"] != float64(0) {
		t.Fatalf("the minimum is off by default: %+v", plain)
	}

	url := fmt.Sprintf("%s/api/indexers/%d", base, id)
	for _, bad := range []any{-1, 1001, 99999} {
		res := putJSONStatus(t, client, url, map[string]any{"minSeeders": bad}, http.StatusBadRequest)
		if msg, _ := res["error"].(string); !strings.Contains(msg, "Minimum seeders") || !strings.Contains(msg, "0 to 1000") {
			t.Fatalf("%v: expected a plain message about minimum seeders, got %+v", bad, res)
		}
	}
	postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Bad", "definitionId": "fixture", "baseUrl": idx.URL, "apiKey": "k", "protocol": "torrent", "minSeeders": 5000,
	}, http.StatusBadRequest)

	kept := putJSONStatus(t, client, url, map[string]any{"name": "Tor 2"}, http.StatusOK)
	if kept["minSeeders"] != float64(5) {
		t.Fatalf("leaving the field out should keep the minimum: %+v", kept)
	}
	top := putJSONStatus(t, client, url, map[string]any{"minSeeders": 1000}, http.StatusOK)
	if top["minSeeders"] != float64(1000) {
		t.Fatalf("1000 is allowed: %+v", top)
	}
	off := putJSONStatus(t, client, url, map[string]any{"minSeeders": 0}, http.StatusOK)
	if off["minSeeders"] != float64(0) {
		t.Fatalf("0 turns it off: %+v", off)
	}
}
