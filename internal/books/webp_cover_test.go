package books_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/books"
)

// 16x24, lossy WebP, solid red (made with libwebp; Go can only read WebP).
const webpCoverB64 = "UklGRkIAAABXRUJQVlA4IDYAAAAQAwCdASoQABgAPrVInkmnJCKhMAgA4BaJQBOl0AAQNgAA/upl/xC72H3/84l/JH6BdrxuMAA="

func webpCover(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(webpCoverB64)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestResizeCoverReadsWebP(t *testing.T) {
	out, err := books.ResizeCover(webpCover(t), 8, 12)
	if err != nil {
		t.Fatal(err)
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil || format != "jpeg" || cfg.Width != 8 || cfg.Height != 12 {
		t.Fatalf("got %q %+v %v", format, cfg, err)
	}
}

func TestPrepareOwnCoverWebP(t *testing.T) {
	out, err := books.PrepareOwnCover(webpCover(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "jpeg" {
		t.Fatalf("the kept picture is %q, %v", format, err)
	}
	cut := webpCover(t)
	_, err = books.PrepareOwnCover(cut[:len(cut)/2])
	if !errors.Is(err, books.ErrCoverUnreadable) {
		t.Fatalf("a cut-off WebP: %v, want ErrCoverUnreadable", err)
	}
}

// Open Library may answer with a WebP picture; it is kept as a JPEG.
func TestFetchCoverKeepsWebPAsJPEG(t *testing.T) {
	webp := webpCover(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/webp")
		w.Write(webp)
	}))
	defer srv.Close()
	c := &books.Client{Base: srv.URL, UserAgent: "test", HTTP: srv.Client()}
	data, err := c.FetchCover(context.Background(), 123, "M")
	if err != nil {
		t.Fatal(err)
	}
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || format != "jpeg" {
		t.Fatalf("got %q, %v", format, err)
	}
}

// An EPUB whose cover is a WebP picture still has a cover.
func TestEPUBCoverWebP(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "book.epub")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	add("META-INF/container.xml", `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`)
	add("OEBPS/content.opf", `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><manifest><item id="cover" href="cover.webp" media-type="image/webp" properties="cover-image"/></manifest></package>`)
	add("OEBPS/cover.webp", string(webpCover(t)))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	data, ct, err := books.EPUBCover(file)
	if err != nil || ct != "image/webp" {
		t.Fatalf("got %q, %v", ct, err)
	}
	out, err := books.ResizeCover(data, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(out)); err != nil || format != "jpeg" || cfg.Width != 16 || cfg.Height != 24 {
		t.Fatalf("got %q %+v %v", format, cfg, err)
	}
}
