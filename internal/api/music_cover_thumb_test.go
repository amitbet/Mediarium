package api_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rdborg/mediarium/internal/books"
)

// artJPEG makes a picture that looks a little like sleeve art (smooth colour,
// some shapes, some grain) as a JPEG of the given size and quality.
func artJPEG(t testing.TB, w, h, seed, quality int) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(int64(seed)))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	r0, g0, b0 := rng.Intn(200), rng.Intn(200), rng.Intn(200)
	cx, cy, rad := rng.Intn(w), rng.Intn(h), w/6+rng.Intn(w/4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := r0 + x*55/w
			g := g0 + y*55/h
			b := b0 + (x+y)*30/(w+h)
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) < rad*rad {
				r, g, b = 255-r, 255-g, 255-b
			}
			n := rng.Intn(17) - 8
			img.Set(x, y, color.RGBA{uint8(clamp(r + n)), uint8(clamp(g + n)), uint8(clamp(b + n)), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func clamp(v int) int { return max(0, min(255, v)) }

// slowArchive is a fake Cover Art Archive that is slow, counts its requests
// and how many were open at the same moment.
type slowArchive struct {
	*httptest.Server
	delay    time.Duration
	hits     atomic.Int32
	open     atomic.Int32
	peak     atomic.Int32
	mu       sync.Mutex
	missing  map[string]bool // release groups without a cover
	art      []byte
	perMBID  map[string]int
	failWith int // when set, answer this status for everything
}

func newSlowArchive(t *testing.T, delay time.Duration, art []byte) *slowArchive {
	t.Helper()
	a := &slowArchive{delay: delay, art: art, missing: map[string]bool{}, perMBID: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/release-group/{mbid}/front-500", func(w http.ResponseWriter, r *http.Request) {
		a.hits.Add(1)
		n := a.open.Add(1)
		defer a.open.Add(-1)
		for {
			p := a.peak.Load()
			if n <= p || a.peak.CompareAndSwap(p, n) {
				break
			}
		}
		a.mu.Lock()
		a.perMBID[r.PathValue("mbid")]++
		missing, fail := a.missing[r.PathValue("mbid")], a.failWith
		a.mu.Unlock()
		time.Sleep(a.delay)
		switch {
		case fail != 0:
			w.WriteHeader(fail)
		case missing:
			http.NotFound(w, r)
		default:
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(a.art)
		}
	})
	a.Server = httptest.NewServer(mux)
	t.Cleanup(a.Close)
	return a
}

func (a *slowArchive) countFor(mbid string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.perMBID[mbid]
}

// getMany asks for the same address from n goroutines at once.
func getMany(t *testing.T, e *musicEnv, url string, n int) (statuses []int, bodies [][]byte) {
	t.Helper()
	statuses, bodies = make([]int, n), make([][]byte, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, url, nil)
			resp, err := e.client.Do(req)
			if err != nil {
				return
			}
			defer resp.Body.Close()
			var buf bytes.Buffer
			buf.ReadFrom(resp.Body)
			statuses[i], bodies[i] = resp.StatusCode, buf.Bytes()
		}()
	}
	wg.Wait()
	return
}

func TestCoverThumbnailsShareOneFetchAndAreSmall(t *testing.T) {
	e := newMusicEnv(t)
	artist := e.addArtist(t, "none")
	first := albumByTitle(t, artist, "First Album")
	full := artJPEG(t, 500, 500, 1, 90)
	archive := newSlowArchive(t, 250*time.Millisecond, full)
	e.server.TestSetMusicBrainz(archive.URL)

	url := e.base + first["coverUrl"].(string)
	statuses, bodies := getMany(t, e, url+"?size=thumb", 12)
	for i, st := range statuses {
		if st != 200 || len(bodies[i]) == 0 {
			t.Fatalf("request %d: status %d, %d bytes", i, st, len(bodies[i]))
		}
	}
	if n := archive.countFor(albumWithCover); n != 1 {
		t.Fatalf("12 requests for one cover must share one download, the archive saw %d", n)
	}
	thumb := bodies[0]
	cfg, format, err := image.DecodeConfig(bytes.NewReader(thumb))
	if err != nil || format != "jpeg" || max(cfg.Width, cfg.Height) > 256 {
		t.Fatalf("thumbnail: %v %q %dx%d", err, format, cfg.Width, cfg.Height)
	}
	if len(thumb) >= len(full)/2 {
		t.Fatalf("thumbnail is %d bytes, the cover %d: not small enough", len(thumb), len(full))
	}
	t.Logf("cover %d bytes -> thumbnail %d bytes (%dx%d)", len(full), len(thumb), cfg.Width, cfg.Height)

	// The full picture is still what ?size is not asked for.
	if status, _, body := getRaw(t, e.client, url); status != 200 || !bytes.Equal(body, full) {
		t.Fatalf("full size: %d, %d bytes", status, len(body))
	}
	// And the thumbnail is kept on disk, next to the covers.
	thumbs, _ := filepath.Glob(filepath.Join(e.server.TestConfigDir(), "music-covers", "thumb-*.img"))
	if len(thumbs) != 1 {
		t.Fatalf("thumbnail files: %v", thumbs)
	}
	if archive.hits.Load() != 1 {
		t.Fatalf("archive hits: %d", archive.hits.Load())
	}
}

func TestCoverThumbnailHeadersAndNotModified(t *testing.T) {
	e := newMusicEnv(t)
	artist := e.addArtist(t, "none")
	first := albumByTitle(t, artist, "First Album")
	archive := newSlowArchive(t, 0, artJPEG(t, 500, 500, 2, 90))
	e.server.TestSetMusicBrainz(archive.URL)
	url := e.base + first["coverUrl"].(string) + "?size=thumb"

	status, hdr, body := getRaw(t, e.client, url)
	if status != 200 || hdr.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("thumbnail: %d %q", status, hdr.Get("Content-Type"))
	}
	if cc := hdr.Get("Cache-Control"); cc != "private, max-age=259200" {
		t.Fatalf("Cache-Control: %q", cc)
	}
	etag := hdr.Get("ETag")
	if etag == "" || hdr.Get("Last-Modified") == "" || hdr.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers: %v", hdr)
	}
	status, _, again := getRaw(t, e.client, url, "If-None-Match", etag)
	if status != http.StatusNotModified || len(again) != 0 {
		t.Fatalf("If-None-Match: %d, %d bytes", status, len(again))
	}
	if status, _, _ := getRaw(t, e.client, url, "If-None-Match", `"other"`); status != 200 {
		t.Fatalf("a different ETag must get the picture: %d", status)
	}
	// A thumbnail of the same picture is the same file, byte for byte.
	if _, _, second := getRaw(t, e.client, url); !bytes.Equal(body, second) {
		t.Fatal("the thumbnail changed between requests")
	}
	// The full cover has an ETag too now.
	_, fullHdr, _ := getRaw(t, e.client, strings.TrimSuffix(url, "?size=thumb"))
	if fullHdr.Get("ETag") == "" || fullHdr.Get("ETag") == etag {
		t.Fatalf("full size ETag: %q (thumbnail %q)", fullHdr.Get("ETag"), etag)
	}
	// A picture the decoder cannot read still shows: the thumbnail request
	// falls back to the picture itself.
	odd := e.base + "/api/music/covers/release-group/aaaaaaaa-0000-4000-8000-0000000000ff?size=thumb"
	archive.mu.Lock()
	archive.art = fakeJPEG
	archive.mu.Unlock()
	if status, _, body := getRaw(t, e.client, odd); status != 200 || !bytes.Equal(body, fakeJPEG) {
		t.Fatalf("undecodable picture: %d, %d bytes", status, len(body))
	}
}

func TestArtistWithoutCoverIsRemembered(t *testing.T) {
	e := newMusicEnv(t)
	artist := e.addArtist(t, "none")
	archive := newSlowArchive(t, 20*time.Millisecond, nil)
	archive.failWith = 0
	for _, a := range artist["albums"].([]any) {
		archive.missing[a.(map[string]any)["mbid"].(string)] = true
	}
	e.server.TestSetMusicBrainz(archive.URL)
	url := e.base + artist["coverUrl"].(string) + "?size=thumb"

	status, hdr, _ := getRaw(t, e.client, url)
	if status != http.StatusNotFound {
		t.Fatalf("artist without covers: %d", status)
	}
	if cc := hdr.Get("Cache-Control"); cc != "private, max-age=1800" {
		t.Fatalf("a missing cover must be remembered by the browser, got Cache-Control %q", cc)
	}
	asked := archive.hits.Load()
	if asked == 0 {
		t.Fatal("the archive was never asked")
	}
	// Remembered on the server, not only in the files: with the marks gone and
	// the page shown again, nothing goes to the archive.
	marks, _ := filepath.Glob(filepath.Join(e.server.TestConfigDir(), "music-covers", "*.none"))
	if len(marks) == 0 {
		t.Fatal("no 'no cover' marks were written")
	}
	for _, m := range marks {
		os.Remove(m)
	}
	for range 3 {
		if status, _, _ := getRaw(t, e.client, url); status != http.StatusNotFound {
			t.Fatalf("status %d", status)
		}
	}
	if archive.hits.Load() != asked {
		t.Fatalf("a page view asked the archive again: %d -> %d", asked, archive.hits.Load())
	}
}

func TestArchiveDownIsNotRetriedAtOnce(t *testing.T) {
	e := newMusicEnv(t)
	artist := e.addArtist(t, "none")
	first := albumByTitle(t, artist, "First Album")
	archive := newSlowArchive(t, 0, nil)
	archive.failWith = http.StatusServiceUnavailable
	e.server.TestSetMusicBrainz(archive.URL)
	url := e.base + first["coverUrl"].(string)
	for range 4 {
		status, hdr, _ := getRaw(t, e.client, url)
		if status != http.StatusNotFound || hdr.Get("Cache-Control") != "private, max-age=60" {
			t.Fatalf("archive down: %d %q", status, hdr.Get("Cache-Control"))
		}
	}
	if n := archive.countFor(albumWithCover); n > 3 { // the client retries a few times inside one download
		t.Fatalf("the archive was asked %d times for 4 page views", n)
	}
	first4 := archive.hits.Load()
	getRaw(t, e.client, url)
	if archive.hits.Load() != first4 {
		t.Fatal("asked again within the minute")
	}
}

func TestDownloadsFromTheArchiveAreLimited(t *testing.T) {
	e := newMusicEnv(t)
	archive := newSlowArchive(t, 120*time.Millisecond, artJPEG(t, 300, 300, 3, 85))
	e.server.TestSetMusicBrainz(archive.URL)

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			getRaw(t, e.client, fmt.Sprintf("%s/api/music/covers/release-group/bbbbbbbb-0000-4000-8000-%012d", e.base, i))
		}()
	}
	wg.Wait()
	if archive.hits.Load() != 16 {
		t.Fatalf("archive hits: %d", archive.hits.Load())
	}
	if peak := archive.peak.Load(); peak > 4 {
		t.Fatalf("%d downloads were open at once, the limit is 4", peak)
	}
}

func TestLocalCoverThumbnailIsSmallAndNeverReplacesTheFile(t *testing.T) {
	e := newMusicEnv(t)
	artist := e.addArtist(t, "none")
	first := albumByTitle(t, artist, "First Album")
	id := int64(first["id"].(float64))
	archive := newSlowArchive(t, 0, artJPEG(t, 500, 500, 4, 90))
	e.server.TestSetMusicBrainz(archive.URL)

	folder := filepath.Join(e.musicRoot, "Fixture Band", "First Album (1999)")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	big := artJPEG(t, 1600, 1600, 5, 92)
	if err := os.WriteFile(filepath.Join(folder, "cover.jpg"), big, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := e.server.MusicRepo.SetAlbumImported(id, "FLAC", folder); err != nil {
		t.Fatal(err)
	}
	status, _, small := getRaw(t, e.client, e.base+first["coverUrl"].(string)+"?size=thumb")
	if status != 200 || len(small) >= len(big)/10 {
		t.Fatalf("thumbnail of a local cover: %d, %d bytes (original %d)", status, len(small), len(big))
	}
	if _, _, same := getRaw(t, e.client, e.base+first["coverUrl"].(string)); !bytes.Equal(same, big) {
		t.Fatal("the full size must still be the file itself")
	}
	if onDisk, _ := os.ReadFile(filepath.Join(folder, "cover.jpg")); !bytes.Equal(onDisk, big) {
		t.Fatal("the cover in the album folder was changed")
	}
	if archive.hits.Load() != 0 {
		t.Fatal("a local cover must not ask the archive")
	}
	t.Logf("local cover %d bytes -> thumbnail %d bytes", len(big), len(small))

	// The artist picture is served from the same file, and a link in the
	// folder that leads out of the music folder is ignored.
	if status, _, body := getRaw(t, e.client, e.base+artist["coverUrl"].(string)+"?size=thumb"); status != 200 || !bytes.Equal(body, small) {
		t.Fatalf("artist thumbnail: %d, %d bytes", status, len(body))
	}
	if err := os.Remove(filepath.Join(folder, "cover.jpg")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.jpg")
	if err := os.WriteFile(outside, big, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(folder, "cover.jpg")); err != nil {
		t.Skipf("no symbolic links here: %v", err)
	}
	_, _, body := getRaw(t, e.client, e.base+first["coverUrl"].(string))
	if bytes.Equal(body, big) {
		t.Fatal("a link leading out of the music folder was followed")
	}
}

// TestMeasureCoverBytes prints what a page of 100 artists costs as it is
// sent now, as thumbnails behind their own address, and as thumbnails inlined
// into the list as base64. It only runs when asked for: the numbers are for a
// decision, not for the build.
func TestMeasureCoverBytes(t *testing.T) {
	if os.Getenv("MEDIARIUM_MEASURE_COVERS") == "" {
		t.Skip("set MEDIARIUM_MEASURE_COVERS=1 to print the numbers")
	}
	const n = 100
	var fullTotal, thumbTotal, b64Total int
	var resize time.Duration
	for i := range n {
		full := artJPEG(t, 500, 500, 100+i, 90)
		start := time.Now()
		small, err := books.ResizeCoverQuality(full, 256, 256, 70)
		resize += time.Since(start)
		if err != nil {
			t.Fatal(err)
		}
		fullTotal += len(full)
		thumbTotal += len(small)
		b64Total += base64.StdEncoding.EncodedLen(len(small)) + len("data:image/jpeg;base64,")
	}
	listJSON := n * 330 // an artist row without any picture: about 330 bytes
	urlOnly := n * (len(`"coverUrl":"/api/music/artists/1234/cover?size=thumb",`) + 4)
	t.Logf("100 artists, covers 500x500 q90: full %d KB, thumbnails %d KB (avg %d bytes), thumbnails as base64 %d KB",
		fullTotal/1024, thumbTotal/1024, thumbTotal/n, b64Total/1024)
	t.Logf("list JSON without pictures ~%d KB; with the thumbnail address ~%d KB; with inlined base64 ~%d KB (and it can never be cached by the browser on its own)",
		listJSON/1024, (listJSON+urlOnly)/1024, (listJSON+b64Total)/1024)
	t.Logf("making a thumbnail: %v each", resize/n)
}
