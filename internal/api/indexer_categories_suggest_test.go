package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The suggestions for extra categories come from the indexer's own capabilities.
func TestIndexerCategoriesSuggestions(t *testing.T) {
	_, base, client := loginNewServer(t)
	caps := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<?xml version="1.0"?><caps><categories><category id="7000" name="Books"><subcat id="7120" name="German"/></category></categories></caps>`)
	}))
	defer caps.Close()
	created := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"name": "Caps", "definitionId": "fixture", "baseUrl": caps.URL, "apiKey": "k", "protocol": "usenet",
	}, http.StatusCreated)
	got := getJSON[map[string]any](t, client, fmt.Sprintf("%s/api/indexers/%d/categories", base, int64(created["id"].(float64))))
	list, _ := got["categories"].([]any)
	if len(list) != 2 {
		t.Fatalf("expected the two categories the indexer lists, got %+v", got)
	}
	if second, _ := list[1].(map[string]any); second["id"] != 7120.0 || second["name"] != "Books/German" {
		t.Errorf("unexpected category: %+v", second)
	}
}
