package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rdborg/mediarium/internal/auth"
)

func TestMCPToolPanicBecomesAnError(t *testing.T) {
	c := &mcpCtx{ctx: context.Background(), user: &auth.User{}}
	boom := &mcpTool{name: "boom", run: func(*mcpCtx, json.RawMessage) (any, error) {
		panic("boom")
	}}
	out, err := c.runTool(boom, nil)
	if err == nil || out != nil {
		t.Fatalf("a panic must come back as an error, got %v, %v", out, err)
	}
}

func TestCanonicalKind(t *testing.T) {
	tests := map[string]string{
		"movies": "movies", "movie": "movies", " Films ": "movies", "TV": "tv", "series": "tv", "shows": "tv", "TV Shows": "tv",
		"music": "music", "albums": "music", "books": "books", "book": "books", "ebooks": "books", "games": "", "": "",
	}
	for in, want := range tests {
		if got := canonicalKind(in); got != want {
			t.Errorf("canonicalKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCoerceArgs(t *testing.T) {
	schema := schemaOf(props{"n": pInt(""), "f": pNum(""), "b": pBool(""), "s": pStr("")})
	tests := []struct{ name, in, want string }{
		{"text to numbers", `{"n":"5","f":"2.5"}`, `{"f":2.5,"n":5}`},
		{"fraction to whole", `{"n":5.9}`, `{"n":5}`},
		{"text to bool", `{"b":"Yes"}`, `{"b":true}`},
		{"nulls dropped", `{"n":null,"s":null}`, `{}`},
		{"empty text dropped", `{"n":"","b":""}`, `{}`},
		{"number to text", `{"s":12}`, `{"s":"12"}`},
		{"unknown text kept for the argument check", `{"n":"many"}`, `{"n":"many"}`},
		{"not an object", `[1]`, `[1]`},
		{"nothing", ``, ``},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(coerceArgs(json.RawMessage(tc.in), schema))
			if got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestGenreMatches(t *testing.T) {
	tests := []struct {
		genres []string
		want   string
		ok     bool
	}{
		{[]string{"Action", "Thriller"}, "action", true},
		{[]string{"Action & Adventure"}, "Action", true},
		{[]string{"Science Fiction"}, "sci-fi", true},
		{[]string{"Sci-Fi & Fantasy"}, "scifi", true},
		{[]string{"Drama"}, "action", false},
		{nil, "action", false},
		{nil, "", true},
	}
	for _, tc := range tests {
		if got := genreMatches(tc.genres, tc.want); got != tc.ok {
			t.Errorf("genreMatches(%v, %q) = %v", tc.genres, tc.want, got)
		}
	}
}
