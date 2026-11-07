package api

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func bigJSONBody() string {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < 200; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"title":"Episode number %d","status":"downloaded","overview":"Some words about it."}`, i, i)
	}
	b.WriteString("]}\n")
	return b.String()
}

func getVia(t *testing.T, h http.Handler, method string, hdr map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/api/x", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	gzipJSON(h).ServeHTTP(rec, req)
	return rec
}

func jsonHandler(body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSONString(w, body)
	})
}

func writeJSONString(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

func unzip(t *testing.T, b []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("not gzip: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip: %v", err)
	}
	return string(out)
}

func TestGzipJSONCompressesBigAnswers(t *testing.T) {
	body := bigJSONBody()
	rec := getVia(t, jsonHandler(body), http.MethodGet, map[string]string{"Accept-Encoding": "gzip, deflate"})
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("not compressed: %v", rec.Header())
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q", rec.Header().Get("Vary"))
	}
	if rec.Header().Get("Content-Length") != "" {
		t.Errorf("a stale Content-Length: %q", rec.Header().Get("Content-Length"))
	}
	if got := unzip(t, rec.Body.Bytes()); got != body {
		t.Fatal("the decompressed body is not the original")
	}
	if rec.Body.Len() >= len(body)/3 {
		t.Errorf("compressed %d bytes to %d: hardly any gain", len(body), rec.Body.Len())
	}
}

func TestGzipJSONLeavesTheseAlone(t *testing.T) {
	body := bigJSONBody()
	gz := map[string]string{"Accept-Encoding": "gzip"}
	for _, c := range []struct {
		name   string
		h      http.Handler
		method string
		hdr    map[string]string
		vary   bool // JSON, so caches are told it depends on Accept-Encoding
	}{
		{"client does not accept gzip", jsonHandler(body), http.MethodGet, nil, false},
		{"gzip refused with q=0", jsonHandler(body), http.MethodGet, map[string]string{"Accept-Encoding": "gzip;q=0"}, false},
		{"a small answer", jsonHandler(`{"ok":true}` + "\n"), http.MethodGet, gz, true},
		{"a POST", jsonHandler(body), http.MethodPost, gz, false},
		{"a range request", jsonHandler(body), http.MethodGet, map[string]string{"Accept-Encoding": "gzip", "Range": "bytes=0-99"}, false},
		{"an event stream", jsonHandler(body), http.MethodGet, map[string]string{"Accept-Encoding": "gzip", "Accept": "text/event-stream"}, false},
		{"an image", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = io.WriteString(w, body)
		}), http.MethodGet, gz, false},
		{"a file download", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = io.WriteString(w, body)
		}), http.MethodGet, gz, false},
		{"already compressed", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var b bytes.Buffer
			zw := gzip.NewWriter(&b)
			_, _ = zw.Write([]byte(body))
			_ = zw.Close()
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "gzip")
			_, _ = w.Write(b.Bytes())
		}), http.MethodGet, gz, true},
		{"an error answer", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, body)
		}), http.MethodGet, gz, false},
		{"a stream that flushes", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"first":1}`+"\n")
			w.(http.Flusher).Flush()
			_, _ = io.WriteString(w, body)
		}), http.MethodGet, gz, true},
	} {
		rec := getVia(t, c.h, c.method, c.hdr)
		if enc := rec.Header().Get("Content-Encoding"); c.name != "already compressed" && enc != "" {
			t.Errorf("%s: got Content-Encoding %q, want none", c.name, enc)
		}
		if c.name == "already compressed" {
			if got := unzip(t, rec.Body.Bytes()); got != body { // one layer only
				t.Errorf("%s: the body is not exactly one gzip layer of the original", c.name)
			}
		}
		if hasVary := strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding"); hasVary != c.vary {
			t.Errorf("%s: Vary = %q, want Accept-Encoding present = %v", c.name, rec.Header().Get("Vary"), c.vary)
		}
	}
	// The plain ones come out unchanged.
	if rec := getVia(t, jsonHandler(body), http.MethodGet, nil); rec.Body.String() != body {
		t.Error("an uncompressed answer was changed")
	}
	if rec := getVia(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, body)
	}), http.MethodGet, gz); rec.Code != http.StatusNotFound || rec.Body.String() != body {
		t.Errorf("an error answer changed: %d", rec.Code)
	}
}

// A big answer written in several pieces still comes out whole.
func TestGzipJSONPiecewise(t *testing.T) {
	body := bigJSONBody()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		for i := 0; i < len(body); i += 100 {
			_, _ = io.WriteString(w, body[i:min(i+100, len(body))])
		}
	})
	rec := getVia(t, h, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if rec.Header().Get("Content-Encoding") != "gzip" || unzip(t, rec.Body.Bytes()) != body {
		t.Fatalf("pieces were lost: %v", rec.Header())
	}
	// And a body that never reaches 1 KB across pieces goes out plain.
	small := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"a":`)
		_, _ = io.WriteString(w, `1}`)
	})
	if rec := getVia(t, small, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"}); rec.Body.String() != `{"a":1}` || rec.Header().Get("Content-Encoding") != "" {
		t.Errorf("small pieces: %q %v", rec.Body.String(), rec.Header())
	}
}

// A strong ETag is made weak when the body is compressed.
func TestGzipJSONWeakensTheETag(t *testing.T) {
	body := bigJSONBody()
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"abc"`)
		writeJSONString(w, body)
	})
	rec := getVia(t, h, http.MethodGet, map[string]string{"Accept-Encoding": "gzip"})
	if got := rec.Header().Get("ETag"); got != `W/"abc"` {
		t.Errorf("ETag = %q, want W/\"abc\"", got)
	}
	rec = getVia(t, h, http.MethodGet, nil)
	if got := rec.Header().Get("ETag"); got != `"abc"` {
		t.Errorf("an uncompressed answer's ETag = %q, want it unchanged", got)
	}
}

// The list routes that compress themselves (revalidated) are not compressed a
// second time, keep their ETag, and still answer 304 to the tag.
func TestGzipJSONWithRevalidated(t *testing.T) {
	body := bigJSONBody()
	h := revalidated(func(w http.ResponseWriter, r *http.Request) { writeJSONString(w, body) })
	gz := map[string]string{"Accept-Encoding": "gzip"}
	rec := getVia(t, h, http.MethodGet, gz)
	if rec.Header().Get("Content-Encoding") != "gzip" || unzip(t, rec.Body.Bytes()) != body {
		t.Fatalf("not exactly one gzip layer: %v", rec.Header())
	}
	tag := rec.Header().Get("ETag")
	if !strings.HasPrefix(tag, "W/") {
		t.Fatalf("ETag %q", tag)
	}
	rec = getVia(t, h, http.MethodGet, map[string]string{"Accept-Encoding": "gzip", "If-None-Match": tag})
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Errorf("a repeat with the tag got %d with %d bytes, want 304 with none", rec.Code, rec.Body.Len())
	}
	// The same tag whether or not the client takes gzip.
	if plain := getVia(t, h, http.MethodGet, nil); plain.Header().Get("ETag") != tag || plain.Body.String() != body {
		t.Errorf("the plain answer differs: %q", plain.Header().Get("ETag"))
	}
}

// Through a real server: the client sees the compressed bytes on the wire and
// the right JSON after decoding, and the connection stays usable.
func TestGzipJSONOnTheWire(t *testing.T) {
	body := bigJSONBody()
	srv := httptest.NewServer(gzipJSON(jsonHandler(body)))
	defer srv.Close()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true}}
	for i := 0; i < 3; i++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.Header.Get("Content-Encoding") != "gzip" || unzip(t, raw) != body {
			t.Fatalf("round %d: %v", i, resp.Header)
		}
	}
}

// TestGzipJSONSavings logs what the middleware does to a show page with a
// thousand episodes: the size on the wire and the time to compress it.
func TestGzipJSONSavings(t *testing.T) {
	var b strings.Builder
	b.WriteString(`{"id":1,"title":"A Long Show","episodes":[`)
	for i := 0; i < 1000; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"id":%d,"season":%d,"episode":%d,"title":"Episode %d","airDate":"2011-04-17","status":"downloaded","monitored":true,"filePath":"Shows/A Long Show/Season %d/A Long Show - S%02dE%02d.mkv","sizeBytes":1523456789,"fileInfo":{"vcodec":"h264","res":"1080p","acodecs":["eac3"],"alangs":["en"],"slangs":["en","fr"]}}`,
			i, i/20+1, i%20+1, i, i/20+1, i/20+1, i%20+1)
	}
	b.WriteString("]}\n")
	body := b.String()
	h := gzipJSON(jsonHandler(body))
	req := httptest.NewRequest(http.MethodGet, "/api/series/1", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	const runs = 50
	start := time.Now()
	for i := 0; i < runs; i++ {
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, req)
	}
	per := time.Since(start) / runs
	t.Logf("1000-episode show: %d bytes plain, %d bytes gzipped (%.1f%%), %v to compress", len(body), rec.Body.Len(), 100*float64(rec.Body.Len())/float64(len(body)), per)
	if rec.Body.Len() >= len(body)/4 {
		t.Errorf("only down to %d of %d bytes", rec.Body.Len(), len(body))
	}
}
