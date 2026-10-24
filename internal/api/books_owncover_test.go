package api_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/api"
)

// ---- fixtures

func coverFixturePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func coverFixtureJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 180, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func coverFixtureGIF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 3, 3), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// coverBookServer is a server (admin "client", member "sam") with the ebook
// module on and one book in the library, whose Open Library entry lists three
// covers (the first is its automatic one) and a removed one.
func coverBookServer(t *testing.T) (server *api.Server, base string, admin, member *http.Client, bookID float64) {
	t.Helper()
	ol := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/works/OL1W.json":
			fmt.Fprint(w, `{"title":"The Hobbit","description":"A hobbit goes on a trip.","covers":[123,-1,456,789]}`)
		case strings.HasPrefix(r.URL.Path, "/b/id/"):
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(coverFixtureJPEG(t, 20, 30))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(ol.Close)
	server, base, admin, member, _ = familyServer(t)
	server.TestSetOpenLibrary(ol.URL)
	postJSONMethod[map[string]any](t, admin, http.MethodPut, base+"/api/modules", map[string]any{"ebooks": true}, http.StatusOK)
	book := postJSON[map[string]any](t, admin, base+"/api/books", map[string]any{
		"olKey": "OL1W", "title": "The Hobbit", "author": "J.R.R. Tolkien", "year": 1937, "coverId": 123, "ebook": true,
	}, http.StatusCreated)
	return server, base, admin, member, book["id"].(float64)
}

// sendCover posts a multipart form with one file part and returns the status and body.
func sendCover(t *testing.T, client *http.Client, url, field, name string, data []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile(field, name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = mw.Close()
	req, err := http.NewRequest(http.MethodPost, url, &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer resp.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func bookCoverURL(t *testing.T, client *http.Client, base string, id float64) (url string, chosen bool) {
	t.Helper()
	b := getJSON[map[string]any](t, client, fmt.Sprintf("%s/api/books/%.0f", base, id))
	url, _ = b["coverUrl"].(string)
	chosen, _ = b["coverChosen"].(bool)
	return url, chosen
}

func fetch(t *testing.T, client *http.Client, url string) (*http.Response, []byte) {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, body
}

// ---- upload

// 16x24, lossy WebP, solid red (made with libwebp; Go can only read WebP).
const coverFixtureWebPB64 = "UklGRkIAAABXRUJQVlA4IDYAAAAQAwCdASoQABgAPrVInkmnJCKhMAgA4BaJQBOl0AAQNgAA/upl/xC72H3/84l/JH6BdrxuMAA="

// A WebP picture is accepted, kept as a JPEG and served as one.
func TestChangeBookCoverByWebPUpload(t *testing.T) {
	_, base, admin, _, id := coverBookServer(t)
	webp, err := base64.StdEncoding.DecodeString(coverFixtureWebPB64)
	if err != nil {
		t.Fatal(err)
	}
	status, body := sendCover(t, admin, fmt.Sprintf("%s/api/books/%.0f/cover", base, id), "file", "cover.webp", webp)
	if status != http.StatusOK {
		t.Fatalf("upload: %d %v", status, body)
	}
	url, chosen := bookCoverURL(t, admin, base, id)
	if !chosen {
		t.Fatalf("the cover was not kept: %q", url)
	}
	resp, img := fetch(t, admin, base+url)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("serve: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(img)); err != nil || format != "jpeg" || cfg.Width != 16 || cfg.Height != 24 {
		t.Errorf("served picture: %v %v %+v", err, format, cfg)
	}
}

func TestChangeBookCoverByUpload(t *testing.T) {
	_, base, admin, member, id := coverBookServer(t)
	upload := fmt.Sprintf("%s/api/books/%.0f/cover", base, id)
	if url, chosen := bookCoverURL(t, admin, base, id); url != "/api/book-covers/123-M.jpg" || chosen {
		t.Fatalf("before: %q, chosen %v", url, chosen)
	}

	cases := []struct {
		name   string
		field  string
		data   []byte
		status int
		text   string // part of the sentence the person reads
	}{
		{"a GIF is refused", "file", coverFixtureGIF(t), http.StatusUnsupportedMediaType, "JPEG, PNG or WebP"},
		{"a web page is refused", "file", []byte("<html>not a picture</html>"), http.StatusUnsupportedMediaType, "JPEG, PNG or WebP"},
		{"a broken WebP picture is refused", "file", append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 40)...), http.StatusUnprocessableEntity, "couldn't be read"},
		{"a broken PNG is refused", "file", coverFixturePNG(t, 50, 80)[:100], http.StatusUnprocessableEntity, "couldn't be read"},
		{"a picture over 8 MB is refused", "file", append(coverFixturePNG(t, 10, 10), make([]byte, 8<<20)...), http.StatusRequestEntityTooLarge, "8 MB"},
		{"an empty file is refused", "file", nil, http.StatusBadRequest, "Choose a picture"},
		{"the wrong form field is refused", "picture", coverFixturePNG(t, 50, 80), http.StatusBadRequest, "Choose a picture"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, body := sendCover(t, admin, upload, tc.field, "cover.png", tc.data)
			if status != tc.status {
				t.Fatalf("status %d (%v), want %d", status, body, tc.status)
			}
			if msg, _ := body["error"].(string); !strings.Contains(msg, tc.text) {
				t.Errorf("message %q should mention %q", msg, tc.text)
			}
			if url, chosen := bookCoverURL(t, admin, base, id); url != "/api/book-covers/123-M.jpg" || chosen {
				t.Errorf("a refused picture changed the cover: %q, chosen %v", url, chosen)
			}
		})
	}

	// A member who may manage books uploads a PNG: kept as a JPEG, served to everyone.
	status, body := sendCover(t, member, upload, "file", "cover.png", coverFixturePNG(t, 400, 600))
	if status != http.StatusOK {
		t.Fatalf("upload: %d %v", status, body)
	}
	url, chosen := bookCoverURL(t, admin, base, id)
	if !strings.HasPrefix(url, fmt.Sprintf("/api/books/%.0f/own-cover?v=", id)) || !chosen {
		t.Fatalf("after upload: %q, chosen %v", url, chosen)
	}
	if body["coverUrl"] != url {
		t.Errorf("the upload answer shows %v, the book shows %q", body["coverUrl"], url)
	}
	resp, img := fetch(t, member, base+url)
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("serve: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control %q: the address changes with the picture, so it can be kept", cc)
	}
	if cfg, format, err := image.DecodeConfig(bytes.NewReader(img)); err != nil || format != "jpeg" || cfg.Width != 400 || cfg.Height != 600 {
		t.Errorf("served picture: %v %v %+v", err, format, cfg)
	}

	// The e-reader and file-cover route use it too.
	if resp, _ := fetch(t, admin, fmt.Sprintf("%s/api/books/%.0f/file-cover", base, id)); resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("file-cover: %d %q", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	// A second picture gets a new address, so no browser keeps showing the first.
	time.Sleep(2 * time.Millisecond)
	if status, body := sendCover(t, admin, upload, "file", "cover.jpg", coverFixtureJPEG(t, 100, 150)); status != http.StatusOK {
		t.Fatalf("second upload: %d %v", status, body)
	}
	if again, _ := bookCoverURL(t, admin, base, id); again == url {
		t.Errorf("the address %q did not change", again)
	}

	// The book list shows it too.
	list := getJSON[[]map[string]any](t, admin, base+"/api/books?brief=1")
	if len(list) != 1 || !strings.Contains(list[0]["coverUrl"].(string), "/own-cover?v=") {
		t.Errorf("list: %v", list)
	}

	// Remove my cover: the automatic one is back, the picture is gone.
	removed := postJSONMethod[map[string]any](t, admin, http.MethodDelete, fmt.Sprintf("%s/api/books/%.0f/cover", base, id), nil, http.StatusOK)
	if removed["coverUrl"] != "/api/book-covers/123-M.jpg" || removed["coverChosen"] != nil {
		t.Errorf("after removing: %v", removed)
	}
	if resp, _ := fetch(t, admin, base+url); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the removed picture still answers: %d", resp.StatusCode)
	}
}

// ---- link

func TestChangeBookCoverByLink(t *testing.T) {
	server, base, admin, _, id := coverBookServer(t)
	var hits atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.png", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(coverFixturePNG(t, 120, 180))
	})
	mux.HandleFunc("/ok-wrong-type.jpg", func(w http.ResponseWriter, r *http.Request) { // the bytes decide, not the header
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(coverFixtureJPEG(t, 120, 180))
	})
	mux.HandleFunc("/moved", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/ok.png", http.StatusFound) })
	mux.HandleFunc("/page", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png") // claims to be a picture
		_, _ = w.Write([]byte("<html>sign in first</html>"))
	})
	mux.HandleFunc("/gif", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(coverFixtureGIF(t)) })
	mux.HandleFunc("/big", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(append(coverFixturePNG(t, 10, 10), make([]byte, 9<<20)...))
	})
	mux.HandleFunc("/forbidden", func(w http.ResponseWriter, r *http.Request) { http.Error(w, "no", http.StatusForbidden) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	// The test server is on this machine, which the real client refuses (below).
	server.TestSetCoverLinkClient(srv.Client())

	link := fmt.Sprintf("%s/api/books/%.0f/cover/link", base, id)
	cases := []struct {
		name   string
		url    string
		status int
		text   string
	}{
		{"a PNG", srv.URL + "/ok.png", http.StatusOK, ""},
		{"a JPEG with the wrong type header", srv.URL + "/ok-wrong-type.jpg", http.StatusOK, ""},
		{"a redirect to a picture", srv.URL + "/moved", http.StatusOK, ""},
		{"a page that says it is a picture", srv.URL + "/page", http.StatusUnsupportedMediaType, "JPEG, PNG or WebP"},
		{"a GIF", srv.URL + "/gif", http.StatusUnsupportedMediaType, "JPEG, PNG or WebP"},
		{"a picture over 8 MB", srv.URL + "/big", http.StatusRequestEntityTooLarge, "8 MB"},
		{"a site that refuses", srv.URL + "/forbidden", http.StatusBadGateway, "status 403"},
		{"a link that goes nowhere", "http://127.0.0.1:1/x.png", http.StatusBadGateway, "Couldn't download"},
		{"an empty link", "", http.StatusBadRequest, "start with http or https"},
		{"words, not a link", "the hobbit cover", http.StatusBadRequest, "start with http or https"},
		{"a file link", "file:///etc/passwd", http.StatusBadRequest, "start with http or https"},
		{"an ftp link", "ftp://example.com/cover.png", http.StatusBadRequest, "start with http or https"},
		{"a very long link", "https://example.com/" + strings.Repeat("a", 3000), http.StatusBadRequest, "start with http or https"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Start every case from the automatic cover.
			postJSONMethod[map[string]any](t, admin, http.MethodDelete, fmt.Sprintf("%s/api/books/%.0f/cover", base, id), nil, http.StatusOK)
			got := postJSON[map[string]any](t, admin, link, map[string]any{"url": tc.url}, tc.status)
			url, chosen := bookCoverURL(t, admin, base, id)
			if tc.status != http.StatusOK {
				if msg, _ := got["error"].(string); !strings.Contains(msg, tc.text) {
					t.Errorf("message %q should mention %q", msg, tc.text)
				}
				if url != "/api/book-covers/123-M.jpg" || chosen {
					t.Errorf("a refused link changed the cover: %q", url)
				}
				return
			}
			if !strings.Contains(url, "/own-cover?v=") || !chosen {
				t.Fatalf("cover after the link: %q, chosen %v", url, chosen)
			}
			if resp, img := fetch(t, admin, base+url); resp.StatusCode != http.StatusOK || len(img) == 0 {
				t.Errorf("serve: %d", resp.StatusCode)
			} else if _, format, err := image.DecodeConfig(bytes.NewReader(img)); err != nil || format != "jpeg" {
				t.Errorf("kept as %q (%v), want a JPEG", format, err)
			}
		})
	}
	if hits.Load() == 0 {
		t.Error("the link was never fetched")
	}
}

// With the real client the server must not fetch from its own machine or the
// home network, so a pasted link can't be used to look around inside it.
func TestChangeBookCoverLinkCannotReachTheHomeNetwork(t *testing.T) {
	_, base, admin, _, id := coverBookServer(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write(coverFixturePNG(t, 20, 30))
	}))
	defer srv.Close()

	link := fmt.Sprintf("%s/api/books/%.0f/cover/link", base, id)
	for name, target := range map[string]string{
		"this machine":             srv.URL + "/cover.png",
		"a private address":        "http://192.168.1.1/cover.png",
		"a cloud metadata address": "http://169.254.169.254/latest/meta-data/",
	} {
		got := postJSON[map[string]any](t, admin, link, map[string]any{"url": target}, http.StatusBadRequest)
		if msg, _ := got["error"].(string); !strings.Contains(msg, "own network") {
			t.Errorf("%s: message %q", name, msg)
		}
	}
	if hits.Load() != 0 {
		t.Errorf("the server fetched from this machine %d times", hits.Load())
	}
	if url, chosen := bookCoverURL(t, admin, base, id); url != "/api/book-covers/123-M.jpg" || chosen {
		t.Errorf("the cover changed: %q", url)
	}
}

// ---- other editions

func TestPickAnotherEditionsCover(t *testing.T) {
	_, base, admin, _, id := coverBookServer(t)
	root := fmt.Sprintf("%s/api/books/%.0f/cover", base, id)

	list := getJSON[map[string]any](t, admin, root+"/editions")
	editions := list["editions"].([]any)
	if len(editions) != 3 || list["current"] != float64(123) {
		t.Fatalf("editions: %v", list)
	}
	first := editions[1].(map[string]any)
	if first["coverId"] != float64(456) || first["url"] != "/api/book-covers/456-S.jpg" {
		t.Errorf("second edition: %v", first)
	}
	if resp, _ := fetch(t, admin, base+first["url"].(string)); resp.StatusCode != http.StatusOK {
		t.Errorf("a thumbnail: %d", resp.StatusCode)
	}

	for _, bad := range []map[string]any{{"coverId": 999}, {"coverId": 0}, {"coverId": -1}, {}} {
		postJSONMethod[map[string]any](t, admin, http.MethodPut, root+"/edition", bad, http.StatusBadRequest)
	}
	if url, chosen := bookCoverURL(t, admin, base, id); url != "/api/book-covers/123-M.jpg" || chosen {
		t.Fatalf("a refused pick changed the cover: %q", url)
	}

	picked := postJSONMethod[map[string]any](t, admin, http.MethodPut, root+"/edition", map[string]any{"coverId": 456}, http.StatusOK)
	if picked["coverUrl"] != "/api/book-covers/456-M.jpg" || picked["coverChosen"] != true {
		t.Fatalf("picked: %v", picked)
	}
	if list := getJSON[map[string]any](t, admin, root+"/editions"); list["current"] != float64(456) {
		t.Errorf("current after picking: %v", list["current"])
	}

	// An uploaded picture replaces the pick and hides "current".
	if status, body := sendCover(t, admin, root, "file", "c.png", coverFixturePNG(t, 40, 60)); status != http.StatusOK {
		t.Fatalf("upload: %d %v", status, body)
	}
	if list := getJSON[map[string]any](t, admin, root+"/editions"); list["current"] != float64(0) {
		t.Errorf("current while a picture of its own is shown: %v", list["current"])
	}
	// Picking again drops the picture.
	url, _ := bookCoverURL(t, admin, base, id)
	postJSONMethod[map[string]any](t, admin, http.MethodPut, root+"/edition", map[string]any{"coverId": 789}, http.StatusOK)
	if resp, _ := fetch(t, admin, base+url); resp.StatusCode != http.StatusNotFound {
		t.Errorf("the picture is still served: %d", resp.StatusCode)
	}

	removed := postJSONMethod[map[string]any](t, admin, http.MethodDelete, root, nil, http.StatusOK)
	if removed["coverUrl"] != "/api/book-covers/123-M.jpg" || removed["coverChosen"] != nil {
		t.Errorf("after removing: %v", removed)
	}
}

// ---- who may

func TestChangeBookCoverNeedsPermissionToManage(t *testing.T) {
	_, base, admin, _, id := coverBookServer(t)
	postJSON[map[string]any](t, admin, base+"/api/users", map[string]any{
		"username": "pat", "password": "pat-password-1", "role": "member",
		"permissions": map[string]any{"manage": false, "play": true, "books": true},
	}, http.StatusCreated)
	jar, _ := cookiejar.New(nil)
	pat := &http.Client{Jar: jar, Timeout: 20 * time.Second}
	postJSON[map[string]any](t, pat, base+"/api/auth/login", map[string]string{"username": "pat", "password": "pat-password-1"}, http.StatusOK)

	root := fmt.Sprintf("%s/api/books/%.0f/cover", base, id)
	if status, _ := sendCover(t, pat, root, "file", "c.png", coverFixturePNG(t, 40, 60)); status != http.StatusForbidden {
		t.Errorf("upload without permission: %d", status)
	}
	if status, _ := doStatus(t, pat, http.MethodPost, root+"/link"); status != http.StatusForbidden {
		t.Errorf("link without permission: %d", status)
	}
	if status, _ := doStatus(t, pat, http.MethodGet, root+"/editions"); status != http.StatusForbidden {
		t.Errorf("editions without permission: %d", status)
	}
	if status, _ := doStatus(t, pat, http.MethodPut, root+"/edition"); status != http.StatusForbidden {
		t.Errorf("pick without permission: %d", status)
	}
	if status, _ := doStatus(t, pat, http.MethodDelete, root); status != http.StatusForbidden {
		t.Errorf("remove without permission: %d", status)
	}
	// They still see a cover somebody else chose.
	if status, _ := sendCover(t, admin, root, "file", "c.png", coverFixturePNG(t, 40, 60)); status != http.StatusOK {
		t.Fatalf("admin upload: %d", status)
	}
	url, _ := bookCoverURL(t, pat, base, id)
	if resp, _ := fetch(t, pat, base+url); resp.StatusCode != http.StatusOK {
		t.Errorf("a member can't see the cover: %d", resp.StatusCode)
	}
	// Signed out: nothing.
	if resp, _ := fetch(t, &http.Client{Timeout: 10 * time.Second}, base+url); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("signed out: %d", resp.StatusCode)
	}
}

func TestChangeBookCoverOfABookThatIsNotThere(t *testing.T) {
	_, base, admin, _, _ := coverBookServer(t)
	if status, _ := sendCover(t, admin, base+"/api/books/9999/cover", "file", "c.png", coverFixturePNG(t, 40, 60)); status != http.StatusNotFound {
		t.Errorf("upload: %d", status)
	}
	if status, _ := doStatus(t, admin, http.MethodDelete, base+"/api/books/9999/cover"); status != http.StatusNotFound {
		t.Errorf("remove: %d", status)
	}
	if status, _ := doStatus(t, admin, http.MethodGet, base+"/api/books/9999/own-cover"); status != http.StatusNotFound {
		t.Errorf("serve: %d", status)
	}
}
