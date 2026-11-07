package api

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipPool keeps compressors for reuse: a new gzip writer allocates a few
// hundred KB, and the API answers hundreds of requests a minute.
var gzipPool = sync.Pool{New: func() any {
	zw, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
	return zw
}}

// gzipJSON compresses the JSON answers of GET requests for a client that
// accepts gzip. JSON shrinks to a tenth or less, which matters on a phone, a
// slow Wi-Fi or a tunnel: a show with a thousand episodes is a few hundred KB
// plain. It leaves alone:
//
//   - anything that is not JSON (images, files, downloads, event streams);
//   - answers that are already encoded (the list pages compress themselves in
//     revalidated), partial answers (Range) and answers under 1 KB;
//   - an answer whose handler flushes before it is done, since that is a
//     stream and the caller wants each part as it is written.
//
// A strong ETag is made weak when the body is compressed, because the bytes
// differ from the plain answer, and the answer says Vary: Accept-Encoding.
func gzipJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.Header.Get("Range") != "" || r.Header.Get("Upgrade") != "" ||
			strings.Contains(r.Header.Get("Accept"), "text/event-stream") ||
			!acceptsGzip(r.Header.Get("Accept-Encoding")) {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipJSONWriter{ResponseWriter: w}
		next.ServeHTTP(gw, r)
		gw.finish()
	})
}

type gzipState int

const (
	gzipUndecided gzipState = iota // header not written yet
	gzipPending                    // JSON 200: holding the first bytes to see whether it is big enough
	gzipPlain                      // passes straight through
	gzipPacking                    // compressing
)

type gzipJSONWriter struct {
	http.ResponseWriter
	state  gzipState
	status int
	held   []byte
	zw     *gzip.Writer
}

func isJSONType(contentType string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mt == "application/json" || strings.HasSuffix(mt, "+json")
}

func (g *gzipJSONWriter) WriteHeader(code int) {
	if g.state != gzipUndecided {
		return
	}
	g.status = code
	h := g.Header()
	if code != http.StatusOK || !isJSONType(h.Get("Content-Type")) {
		g.state = gzipPlain
		g.ResponseWriter.WriteHeader(code)
		return
	}
	// A JSON answer: caches must know it depends on Accept-Encoding, whether
	// or not this one is big enough to compress.
	h.Add("Vary", "Accept-Encoding")
	if h.Get("Content-Encoding") != "" || h.Get("Content-Range") != "" {
		g.state = gzipPlain
		g.ResponseWriter.WriteHeader(code)
		return
	}
	if n, err := strconv.Atoi(h.Get("Content-Length")); err == nil && n < minGzipBytes {
		g.state = gzipPlain
		g.ResponseWriter.WriteHeader(code)
		return
	}
	g.state = gzipPending
}

func (g *gzipJSONWriter) Write(p []byte) (int, error) {
	if g.state == gzipUndecided {
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(p))
		}
		g.WriteHeader(http.StatusOK)
	}
	switch g.state {
	case gzipPlain:
		return g.ResponseWriter.Write(p)
	case gzipPacking:
		return g.zw.Write(p)
	}
	g.held = append(g.held, p...)
	if len(g.held) >= minGzipBytes {
		g.startPacking()
		if _, err := g.zw.Write(g.held); err != nil {
			return 0, err
		}
		g.held = nil
	}
	return len(p), nil
}

// startPacking turns compression on for the answer.
func (g *gzipJSONWriter) startPacking() {
	h := g.Header()
	h.Del("Content-Length")
	h.Set("Content-Encoding", "gzip")
	if tag := h.Get("ETag"); tag != "" && !strings.HasPrefix(tag, "W/") {
		h.Set("ETag", "W/"+tag)
	}
	g.ResponseWriter.WriteHeader(g.status)
	g.zw = gzipPool.Get().(*gzip.Writer)
	g.zw.Reset(g.ResponseWriter)
	g.state = gzipPacking
}

// Flush sends what there is. A handler that flushes mid-answer is streaming,
// so the answer then goes out as it is, uncompressed.
func (g *gzipJSONWriter) Flush() {
	switch g.state {
	case gzipUndecided:
		g.WriteHeader(http.StatusOK)
		g.Flush()
		return
	case gzipPending:
		g.state = gzipPlain
		g.ResponseWriter.WriteHeader(g.status)
		if len(g.held) > 0 {
			_, _ = g.ResponseWriter.Write(g.held)
			g.held = nil
		}
	case gzipPacking:
		_ = g.zw.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the real writer.
func (g *gzipJSONWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

// finish ends the answer: small ones go out plain, compressed ones are closed.
func (g *gzipJSONWriter) finish() {
	switch g.state {
	case gzipPending:
		g.state = gzipPlain
		g.ResponseWriter.WriteHeader(g.status)
		if len(g.held) > 0 {
			_, _ = g.ResponseWriter.Write(g.held)
		}
		g.held = nil
	case gzipPacking:
		_ = g.zw.Close()
		g.zw.Reset(nil)
		gzipPool.Put(g.zw)
		g.zw = nil
	}
}
