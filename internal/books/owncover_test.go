package books_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/books"
)

func fixturePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fixtureJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fixtureGIF(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewPaletted(image.Rect(0, 0, 4, 4), color.Palette{color.Black, color.White}), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPrepareOwnCover(t *testing.T) {
	brokenWebP := append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 40)...)
	pngHalf := fixturePNG(t, 60, 90)
	pngHalf = pngHalf[:len(pngHalf)/2] // starts like a picture, ends nowhere
	tooBig := append(fixturePNG(t, 10, 10), make([]byte, books.MaxOwnCoverBytes)...)

	cases := []struct {
		name    string
		data    []byte
		wantErr error
		maxW    int // the result is at most this wide (0 = not checked)
		maxH    int
	}{
		{name: "a PNG", data: fixturePNG(t, 60, 90), maxW: 60, maxH: 90},
		{name: "a JPEG", data: fixtureJPEG(t, 90, 60), maxW: 90, maxH: 60},
		{name: "a big picture is cut down", data: fixturePNG(t, 1600, 2400), maxW: 800, maxH: 1200},
		{name: "a GIF", data: fixtureGIF(t), wantErr: books.ErrCoverFormat},
		{name: "a WebP picture", data: webpCover(t), maxW: 16, maxH: 24},
		{name: "a WebP picture that is cut off", data: brokenWebP, wantErr: books.ErrCoverUnreadable},
		{name: "a web page", data: []byte("<html><body>not a picture</body></html>"), wantErr: books.ErrCoverFormat},
		{name: "nothing at all", data: nil, wantErr: books.ErrCoverFormat},
		{name: "a PNG that is cut off", data: pngHalf, wantErr: books.ErrCoverUnreadable},
		{name: "a picture over the limit", data: tooBig, wantErr: books.ErrCoverTooBig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := books.PrepareOwnCover(tc.data)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			cfg, format, err := image.DecodeConfig(bytes.NewReader(out))
			if err != nil || format != "jpeg" {
				t.Fatalf("the result is not a JPEG: %q, %v", format, err)
			}
			if cfg.Width > tc.maxW || cfg.Height > tc.maxH || cfg.Width == 0 {
				t.Errorf("result is %dx%d, want at most %dx%d", cfg.Width, cfg.Height, tc.maxW, tc.maxH)
			}
		})
	}
}

func TestOwnCoverIsKeptApartFromTheAutomaticOne(t *testing.T) {
	r := newRepo(t)
	b, err := r.Add(books.Book{OLKey: "OL1W", Title: "One", CoverID: 111})
	if err != nil {
		t.Fatal(err)
	}
	if got := b.CoverPath("M"); got != "/api/book-covers/111-M.jpg" {
		t.Fatalf("automatic cover = %q", got)
	}

	// A picture of its own wins, and the address changes with every new picture.
	t0 := time.UnixMilli(1_700_000_000_000)
	if err := r.SetOwnCover(b.ID, []byte("one"), t0); err != nil {
		t.Fatal(err)
	}
	b, _ = r.Get(b.ID)
	if got, want := b.CoverPath("L"), fmt.Sprintf("/api/books/%d/own-cover?v=1700000000000", b.ID); got != want {
		t.Fatalf("own cover = %q, want %q", got, want)
	}
	if img, err := r.OwnCoverImage(b.ID); err != nil || string(img) != "one" {
		t.Fatalf("picture = %q, %v", img, err)
	}
	if err := r.SetOwnCover(b.ID, []byte("two"), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	b2, _ := r.Get(b.ID)
	if b2.CoverPath("M") == b.CoverPath("M") {
		t.Error("a new picture must get a new address, or browsers keep showing the old one")
	}
	if img, _ := r.OwnCoverImage(b.ID); string(img) != "two" {
		t.Errorf("picture = %q, want the newer one", img)
	}

	// Picking another edition's cover drops the picture; the automatic id stays.
	if err := r.PickCover(b.ID, 222); err != nil {
		t.Fatal(err)
	}
	b, _ = r.Get(b.ID)
	if b.CoverPath("M") != "/api/book-covers/222-M.jpg" || b.CoverID != 111 || b.OwnCover != 0 {
		t.Fatalf("after picking: path %q, automatic %d, own %d", b.CoverPath("M"), b.CoverID, b.OwnCover)
	}
	if _, err := r.OwnCoverImage(b.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("the picture should be gone, got %v", err)
	}

	// A picture of its own replaces a picked cover.
	if err := r.SetOwnCover(b.ID, []byte("three"), t0.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	b, _ = r.Get(b.ID)
	if b.PickedCoverID != 0 || b.OwnCover == 0 {
		t.Fatalf("after uploading: picked %d, own %d", b.PickedCoverID, b.OwnCover)
	}

	// Removing it all brings the automatic cover back.
	if err := r.ClearOwnCover(b.ID); err != nil {
		t.Fatal(err)
	}
	b, _ = r.Get(b.ID)
	if b.CoverPath("M") != "/api/book-covers/111-M.jpg" || b.OwnCover != 0 || b.PickedCoverID != 0 {
		t.Fatalf("after removing: %q, own %d, picked %d", b.CoverPath("M"), b.OwnCover, b.PickedCoverID)
	}
	if _, err := r.OwnCoverImage(b.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("the picture should be gone, got %v", err)
	}
}

func TestOwnCoverOfAMissingBookAndDeletedBook(t *testing.T) {
	r := newRepo(t)
	for name, err := range map[string]error{
		"save":   r.SetOwnCover(999, []byte("x"), time.Now()),
		"pick":   r.PickCover(999, 5),
		"remove": r.ClearOwnCover(999),
	} {
		if !errors.Is(err, sql.ErrNoRows) {
			t.Errorf("%s on a book that isn't there: %v", name, err)
		}
	}
	b, err := r.Add(books.Book{OLKey: "OL2W", Title: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.SetOwnCover(b.ID, []byte("x"), time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.OwnCoverImage(b.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("a deleted book's picture should be gone, got %v", err)
	}
}

func TestWorkCovers(t *testing.T) {
	cases := []struct {
		name string
		body string
		key  string
		want []int
		err  error
	}{
		{name: "several editions", body: `{"covers":[5,-1,7,5,9]}`, key: "OL1W", want: []int{5, 7, 9}},
		{name: "none", body: `{"title":"x"}`, key: "OL1W", want: []int{}},
		{name: "only removed ones", body: `{"covers":[-1]}`, key: "OL1W", want: []int{}},
		{name: "a work key with a path in it", body: `{}`, key: "OL1W/../x", err: books.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/works/OL1W.json" {
					http.NotFound(w, r)
					return
				}
				fmt.Fprint(w, tc.body)
			}))
			defer srv.Close()
			c := &books.Client{Base: srv.URL, HTTP: srv.Client()}
			got, err := c.WorkCovers(context.Background(), tc.key)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error = %v, want %v", err, tc.err)
			}
			if err == nil && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("covers = %v, want %v", got, tc.want)
			}
		})
	}

	// More than a dozen are cut off.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"covers":[1,2,3,4,5,6,7,8,9,10,11,12,13,14,15]}`)
	}))
	defer srv.Close()
	c := &books.Client{Base: srv.URL, HTTP: srv.Client()}
	if got, _ := c.WorkCovers(context.Background(), "OL1W"); len(got) != 12 {
		t.Errorf("got %d covers, want 12", len(got))
	}
}
