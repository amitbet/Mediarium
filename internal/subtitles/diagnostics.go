package subtitles

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Diagnostic says what one lookup at one subtitle source asked and what came
// back. A search that finds nothing shows these lines, so a report can tell
// "the site has none" from "the site answered but everything was thrown out".
// It never holds a key or a URL.
type Diagnostic struct {
	Source string `json:"source"` // "OpenSubtitles", "SubDL", an add-on's name
	Lookup string `json:"lookup"` // "tmdb", "imdb", "title", or "" for a whole-source line
	Value  string `json:"value,omitempty"`
	Bare   bool   `json:"bare,omitempty"` // the second try with fewer filters
	Count  int    `json:"count"`          // results kept
	Found  int    `json:"found"`          // what the site sent back, before anything was dropped
	// Note is empty when all is well, or one of: "no-match" (the site doesn't
	// know the title), "wrong-language", "wrong-episode", "bad-link", "error".
	Note   string `json:"note,omitempty"`
	Detail string `json:"detail,omitempty"` // e.g. the languages that were found, or the site's own message
}

// Diagnostics collects the lines of one search.
type Diagnostics struct {
	mu    sync.Mutex
	items []Diagnostic
}

type diagKey struct{}

// WithDiagnostics returns a context whose searches report into the result.
func WithDiagnostics(ctx context.Context) (context.Context, *Diagnostics) {
	d := &Diagnostics{}
	return context.WithValue(ctx, diagKey{}, d), d
}

// Add records one line.
func (d *Diagnostics) Add(line Diagnostic) {
	if d == nil {
		return
	}
	line.Detail = redactSecrets(line.Detail)
	d.mu.Lock()
	d.items = append(d.items, line)
	d.mu.Unlock()
}

// List returns the lines in the order they were recorded.
func (d *Diagnostics) List() []Diagnostic {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]Diagnostic(nil), d.items...)
}

// Report records a line into the context's collector, if it has one.
func Report(ctx context.Context, line Diagnostic) {
	if d, ok := ctx.Value(diagKey{}).(*Diagnostics); ok {
		d.Add(line)
	}
}

var secretParam = regexp.MustCompile(`(?i)(api_?key|token|password)=[^&\s"']+`)

// redactSecrets blanks key-looking query values in a message.
func redactSecrets(s string) string {
	return secretParam.ReplaceAllString(s, "${1}=…")
}

// joinLanguages lists language spellings, sorted, at most eight.
func joinLanguages(m map[string]bool) string {
	out := make([]string, 0, len(m))
	for l := range m {
		out = append(out, l)
	}
	sort.Strings(out)
	if len(out) > 8 {
		out = append(out[:8], "…")
	}
	return strings.Join(out, ", ")
}
