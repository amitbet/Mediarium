package indexers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// When FlareSolverr is too slow the message names FlareSolverr, says how long
// the wait was and what to try.
func TestCardigannFlareSolverrTooSlowSaysSo(t *testing.T) {
	site := cloudflareSite(t)
	release := make(chan struct{})
	fs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer fs.Close()
	defer close(release)
	m := fixtureManager(t, "fixturepublic")
	m.FlareSolverrURL = func() string { return fs.URL }
	inst := Instance{ID: 1, Name: "CF Site", Kind: KindCardigann, DefinitionID: "fixturepublic", BaseURL: site.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := m.Test(ctx, inst)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"FlareSolverr didn't finish the Cloudflare check after", "is running and has internet access", "try again"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message %q should contain %q", err.Error(), want)
		}
	}
}

// A FlareSolverr that takes a while but answers inside the budget works.
func TestCardigannFlareSolverrSlowButInTime(t *testing.T) {
	site := cloudflareSite(t)
	fs := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(700 * time.Millisecond)
		var req struct {
			URL string `json:"url"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		u, _ := url.Parse(req.URL)
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"solution": map[string]any{
				"url": req.URL, "status": 200, "userAgent": "FlareSolverr-Browser/1.0",
				"cookies":  []map[string]any{{"name": "cf_clearance", "value": "cleared", "domain": u.Hostname(), "path": "/"}},
				"response": publicResults,
			},
		})
	}))
	defer fs.Close()
	m := fixtureManager(t, "fixturepublic")
	m.FlareSolverrURL = func() string { return fs.URL }
	inst := Instance{ID: 1, Name: "CF Site", Kind: KindCardigann, DefinitionID: "fixturepublic", BaseURL: site.URL}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Test(ctx, inst); err != nil {
		t.Fatal(err)
	}
}

func TestCardigannSearchBudget(t *testing.T) {
	tests := []struct {
		name string
		url  func() string
		want time.Duration
	}{
		{"no FlareSolverr", func() string { return "" }, cardigannTimeout},
		{"blank address", func() string { return "  " }, cardigannTimeout},
		{"FlareSolverr set", func() string { return "http://flaresolverr:8191" }, cardigannFlareSolverrTimeout},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := &CardigannManager{FlareSolverrURL: tc.url}
			if got := m.SearchBudget(); got != tc.want {
				t.Errorf("SearchBudget() = %v, want %v", got, tc.want)
			}
		})
	}
	if (*CardigannManager)(nil).SearchBudget() != cardigannTimeout {
		t.Error("a missing manager should give the plain limit")
	}
	if cardigannFlareSolverrTimeout <= time.Duration(flareSolverrMaxTimeout)*time.Millisecond+15*time.Second {
		t.Error("the FlareSolverr search limit must outlast one FlareSolverr request")
	}
}
