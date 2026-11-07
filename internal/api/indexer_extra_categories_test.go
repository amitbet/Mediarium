package api_test

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

// An indexer can be given extra categories per kind of media; they show on the
// card, an edit that leaves the field out keeps them, an empty object clears
// them, and a bad number is refused in plain words.
func TestIndexerExtraCategories(t *testing.T) {
	_, base, client := loginNewServer(t)
	idx := newTVIndexerWith(t, nil)

	created := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Extra", "definitionId": "fixture", "baseUrl": idx.URL, "apiKey": "k", "protocol": "usenet",
		"extraCategories": map[string]any{"tv": []int{105010, 105000, 105000}, "books": []int{7120}},
	}, http.StatusCreated)
	id := int64(created["id"].(float64))
	want := map[string]any{"tv": []any{105000.0, 105010.0}, "books": []any{7120.0}}
	if !reflect.DeepEqual(created["extraCategories"], want) {
		t.Fatalf("extra categories on the card: %+v", created["extraCategories"])
	}

	plain := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Plain", "definitionId": "fixture", "baseUrl": idx.URL, "apiKey": "k", "protocol": "usenet",
	}, http.StatusCreated)
	if n, _ := plain["extraCategories"].(map[string]any); len(n) != 0 {
		t.Fatalf("none by default: %+v", plain["extraCategories"])
	}

	url := fmt.Sprintf("%s/api/indexers/%d", base, id)
	for name, bad := range map[string]any{
		"unknown kind": map[string]any{"games": []int{4000}},
		"zero":         map[string]any{"tv": []int{0}},
		"negative":     map[string]any{"tv": []int{-3}},
		"too big":      map[string]any{"tv": []int{5000000}},
	} {
		res := putJSONStatus(t, client, url, map[string]any{"extraCategories": bad}, http.StatusBadRequest)
		if msg, _ := res["error"].(string); msg == "" || strings.Contains(msg, "{") {
			t.Fatalf("%s: expected a plain message, got %+v", name, res)
		}
	}
	postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Bad", "definitionId": "fixture", "baseUrl": idx.URL, "apiKey": "k", "protocol": "usenet",
		"extraCategories": map[string]any{"tv": []int{0}},
	}, http.StatusBadRequest)

	kept := putJSONStatus(t, client, url, map[string]any{"name": "Extra 2"}, http.StatusOK)
	if !reflect.DeepEqual(kept["extraCategories"], want) {
		t.Fatalf("leaving the field out should keep them: %+v", kept["extraCategories"])
	}
	changed := putJSONStatus(t, client, url, map[string]any{"extraCategories": map[string]any{"audiobooks": []int{3130}}}, http.StatusOK)
	if !reflect.DeepEqual(changed["extraCategories"], map[string]any{"audiobooks": []any{3130.0}}) {
		t.Fatalf("a new set replaces the old: %+v", changed["extraCategories"])
	}
	cleared := putJSONStatus(t, client, url, map[string]any{"extraCategories": map[string]any{}}, http.StatusOK)
	if n, _ := cleared["extraCategories"].(map[string]any); len(n) != 0 {
		t.Fatalf("an empty object clears them: %+v", cleared["extraCategories"])
	}
}
