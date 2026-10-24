package debrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakePremiumize is a strict stand-in for the Premiumize API, written from its
// public documentation. It refuses every call without the right key, every
// unknown path and every wrong method, and it records the calls that matter.
type fakePremiumize struct {
	t   *testing.T
	srv *httptest.Server

	mu         sync.Mutex
	cached     map[string]bool // info hashes the service has
	polls      int             // transfer lists answered
	finishAt   int             // the poll at which the transfer is finished
	transferOK bool            // a transfer was created
	created    []string        // src of each transfer/create
	uploaded   int             // torrent files received
	deleted    []string        // "transfer:<id>", "folder:<id>", "item:<id>"
	limited    bool            // answer 429 to everything
}

const pmKey = "pm-key"

func newFakePremiumize(t *testing.T) *fakePremiumize {
	f := &fakePremiumize{t: t, cached: map[string]bool{}, finishAt: 3}
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, v any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(v)
	}
	fail := func(w http.ResponseWriter, msg string) {
		reply(w, map[string]any{"status": "error", "message": msg})
	}
	// guard wraps a handler: rate limit, then the key.
	guard := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			f.mu.Lock()
			limited := f.limited
			f.mu.Unlock()
			if limited {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			if r.Header.Get("Authorization") != "Bearer "+pmKey {
				fail(w, "customer_id and pin parameter missing or not logged in")
				return
			}
			if r.URL.Query().Get("apikey") != "" {
				t.Errorf("the key must not travel in the address: %s", r.URL)
			}
			h(w, r)
		}
	}
	mux.HandleFunc("GET /account/info", guard(func(w http.ResponseWriter, r *http.Request) {
		reply(w, map[string]any{"status": "success", "customer_id": 424242, "premium_until": time.Now().Add(48 * time.Hour).Unix(), "limit_used": 0.25, "space_used": 1.5})
	}))
	mux.HandleFunc("POST /cache/check", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		items := r.PostForm["items[]"]
		if len(items) == 0 {
			fail(w, "no items")
			return
		}
		resp := make([]bool, len(items))
		for i, h := range items {
			resp[i] = f.cached[h]
		}
		reply(w, map[string]any{"status": "success", "response": resp, "transcoded": make([]bool, len(items)), "filename": make([]string, len(items)), "filesize": make([]int, len(items))})
	}))
	mux.HandleFunc("POST /transfer/directdl", guard(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		hash := strings.TrimPrefix(r.PostForm.Get("src"), "magnet:?xt=urn:btih:")
		if !f.cached[hash] {
			fail(w, "Not cached")
			return
		}
		reply(w, map[string]any{"status": "success", "location": f.srv.URL + "/dl/one", "filename": "Show.S01", "filesize": 12, "content": []map[string]any{
			{"path": "Show.S01/ep1.mkv", "size": 10, "link": f.srv.URL + "/dl/ep1.mkv", "stream_link": f.srv.URL + "/s/ep1"},
			{"path": "Show.S01/Subs/ep1.srt", "size": 2, "link": f.srv.URL + "/dl/ep1.srt"},
		}})
	}))
	mux.HandleFunc("POST /transfer/create", guard(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			fail(w, "bad form")
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if src := r.FormValue("src"); src != "" {
			f.created = append(f.created, src)
		} else if _, _, err := r.FormFile("file"); err == nil {
			f.uploaded++
		} else {
			fail(w, "src or file missing")
			return
		}
		f.transferOK = true
		reply(w, map[string]any{"status": "success", "id": "tr1", "name": "Show.S01", "type": "torrent"})
	}))
	mux.HandleFunc("GET /transfer/list", guard(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		list := []any{map[string]any{"id": "tr2", "name": "Old", "status": "error", "message": "Not enough space", "progress": nil}}
		if f.transferOK {
			f.polls++
			tr := map[string]any{"id": "tr1", "name": "Show.S01", "message": "Downloading", "status": "running", "progress": 0.4, "src": "magnet:?", "folder_id": nil, "file_id": nil}
			if f.polls >= f.finishAt {
				tr["status"], tr["progress"], tr["message"], tr["folder_id"] = "finished", 1, "Done", "root1"
			}
			list = append(list, tr)
		}
		reply(w, map[string]any{"status": "success", "transfers": list})
	}))
	mux.HandleFunc("GET /folder/list", guard(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("id") {
		case "root1":
			reply(w, map[string]any{"status": "success", "name": "Show.S01", "content": []map[string]any{
				{"type": "file", "id": "f1", "name": "ep1.mkv", "size": 10, "link": f.srv.URL + "/dl/ep1.mkv"},
				{"type": "folder", "id": "sub1", "name": "Subs"},
			}})
		case "sub1":
			reply(w, map[string]any{"status": "success", "name": "Subs", "content": []map[string]any{
				{"type": "file", "id": "f2", "name": "ep1.srt", "size": 2, "link": f.srv.URL + "/dl/ep1.srt"},
				{"type": "file", "id": "f3", "name": "../../evil.sh", "size": 1, "link": f.srv.URL + "/dl/evil"},
			}})
		default:
			fail(w, "folder not found")
		}
	}))
	mux.HandleFunc("GET /item/details", guard(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") != "single1" {
			fail(w, "item not found")
			return
		}
		reply(w, map[string]any{"status": "success", "id": "single1", "name": "Movie.mkv", "size": 7, "link": f.srv.URL + "/dl/movie.mkv"})
	}))
	del := func(kind string) http.HandlerFunc {
		return guard(func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			f.mu.Lock()
			f.deleted = append(f.deleted, kind+":"+r.PostForm.Get("id"))
			f.mu.Unlock()
			reply(w, map[string]any{"status": "success"})
		})
	}
	mux.HandleFunc("POST /transfer/delete", del("transfer"))
	mux.HandleFunc("POST /folder/delete", del("folder"))
	mux.HandleFunc("POST /item/delete", del("item"))
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakePremiumize) client(key string) *premiumize {
	return &premiumize{key: key, http: f.srv.Client(), base: f.srv.URL}
}

func TestPremiumizeAccount(t *testing.T) {
	f := newFakePremiumize(t)
	acc, err := f.client(pmKey).Account(context.Background())
	if err != nil || acc.Name != "424242" || !acc.Premium || acc.Expires.IsZero() || acc.FairUse != 0.25 {
		t.Fatalf("account: %+v %v", acc, err)
	}
	if _, err := f.client("wrong").Account(context.Background()); !errors.Is(err, ErrAuth) {
		t.Fatalf("a wrong key must be ErrAuth, got %v", err)
	}
}

// The slow path: the torrent is not cached, so a transfer runs and the files
// are read from the folder it made.
func TestPremiumizeTransferFlow(t *testing.T) {
	f := newFakePremiumize(t)
	p := f.client(pmKey)
	ctx := context.Background()
	id, err := p.Add(ctx, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=x", nil)
	if err != nil || id != "tr1" {
		t.Fatalf("add: %q %v", id, err)
	}
	if len(f.created) != 1 || !strings.HasPrefix(f.created[0], "magnet:?xt=urn:btih:0123") {
		t.Fatalf("created: %v", f.created)
	}
	if st, err := p.Status(ctx, id); err != nil || st.State != Downloading || st.Progress != 0.4 {
		t.Fatalf("running: %+v %v", st, err)
	}
	if st, _ := p.Status(ctx, id); st.State != Downloading {
		t.Fatalf("still running: %+v", st)
	}
	if st, _ := p.Status(ctx, id); st.State != Ready || st.Progress != 1 {
		t.Fatalf("finished: %+v", st)
	}
	files, err := p.Files(ctx, id)
	if err != nil || len(files) != 3 {
		t.Fatalf("files: %+v %v", files, err)
	}
	want := map[string]int64{"Show.S01/ep1.mkv": 10, "Show.S01/Subs/ep1.srt": 2, "Show.S01/Subs/evil.sh": 1}
	for _, fl := range files {
		name := strings.ReplaceAll(fl.Name, `\`, "/")
		if want[name] != fl.Size || !strings.HasPrefix(fl.URL, f.srv.URL+"/dl/") {
			t.Errorf("file %+v", fl)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing files: %v", want)
	}
	if err := p.Remove(ctx, id); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.deleted, ","); got != "transfer:tr1,folder:root1" {
		t.Fatalf("deleted %q: the transfer and its folder must both go", got)
	}
}

// A torrent Premiumize already has is read at once and never becomes a transfer.
func TestPremiumizeInstantPath(t *testing.T) {
	f := newFakePremiumize(t)
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	f.cached[hash] = true
	p := f.client(pmKey)
	ctx := context.Background()
	id, err := p.Add(ctx, "magnet:?xt=urn:btih:"+strings.ToUpper(hash)+"&dn=Show&tr=udp://x", nil)
	if err != nil || id != "dd-"+hash {
		t.Fatalf("add: %q %v", id, err)
	}
	if f.transferOK {
		t.Fatal("a cached torrent must not create a transfer")
	}
	if st, err := p.Status(ctx, id); err != nil || st.State != Ready {
		t.Fatalf("status: %+v %v", st, err)
	}
	files, err := p.Files(ctx, id) // also after a restart: the id alone is enough
	if err != nil || len(files) != 2 || files[0].Name != "Show.S01/ep1.mkv" || files[0].Size != 10 || files[1].URL != f.srv.URL+"/dl/ep1.srt" {
		t.Fatalf("files: %+v %v", files, err)
	}
	if err := p.Remove(ctx, id); err != nil || len(f.deleted) != 0 {
		t.Fatalf("remove: %v %v", err, f.deleted)
	}
}

func TestPremiumizeTorrentFileAndSingleFile(t *testing.T) {
	f := newFakePremiumize(t)
	p := f.client(pmKey)
	id, err := p.Add(context.Background(), "", []byte("d8:announce0:e"))
	if err != nil || id != "tr1" || f.uploaded != 1 {
		t.Fatalf("add file: %q %v uploaded=%d", id, err, f.uploaded)
	}
	// A transfer that made one file, not a folder.
	files, err := p.single(context.Background(), "single1")
	if err != nil || len(files) != 1 || files[0].Name != "Movie.mkv" || files[0].Size != 7 {
		t.Fatalf("single: %+v %v", files, err)
	}
}

func TestPremiumizeErrors(t *testing.T) {
	f := newFakePremiumize(t)
	ctx := context.Background()
	f.transferOK = true
	t.Run("transfer that failed", func(t *testing.T) {
		st, err := f.client(pmKey).Status(ctx, "tr2")
		if err != nil || st.State != Failed || !strings.Contains(st.Message, "Not enough space") {
			t.Fatalf("%+v %v", st, err)
		}
	})
	t.Run("transfer that is gone", func(t *testing.T) {
		st, err := f.client(pmKey).Status(ctx, "nope")
		if err != nil || st.State != Failed {
			t.Fatalf("%+v %v", st, err)
		}
	})
	t.Run("wrong key on every call", func(t *testing.T) {
		p := f.client("bad")
		if _, err := p.Add(ctx, "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567", nil); !errors.Is(err, ErrAuth) {
			t.Errorf("add: %v", err)
		}
		if _, err := p.Cached(ctx, []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}); !errors.Is(err, ErrAuth) {
			t.Errorf("cached: %v", err)
		}
		if _, err := p.List(ctx); !errors.Is(err, ErrAuth) {
			t.Errorf("list: %v", err)
		}
	})
	t.Run("rate limit", func(t *testing.T) {
		f.mu.Lock()
		f.limited = true
		f.mu.Unlock()
		defer func() {
			f.mu.Lock()
			f.limited = false
			f.mu.Unlock()
		}()
		_, err := f.client(pmKey).Account(ctx)
		if err == nil || errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "too many requests") {
			t.Fatalf("429 should be a plain, retryable error, got %v", err)
		}
	})
	t.Run("http auth status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
		defer srv.Close()
		if _, err := (&premiumize{key: "k", http: srv.Client(), base: srv.URL}).Account(ctx); !errors.Is(err, ErrAuth) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("error with a message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"status":"error","message":"Account is not premium"}`)
		}))
		defer srv.Close()
		_, err := (&premiumize{key: "k", http: srv.Client(), base: srv.URL}).Account(ctx)
		if err == nil || errors.Is(err, ErrAuth) || !strings.Contains(err.Error(), "Account is not premium") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("answer that is not JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "<html>") }))
		defer srv.Close()
		if _, err := (&premiumize{key: "k", http: srv.Client(), base: srv.URL}).Account(ctx); err == nil {
			t.Fatal("want an error")
		}
	})
}

func TestPremiumizeCached(t *testing.T) {
	const (
		a = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		b = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	)
	f := newFakePremiumize(t)
	f.cached[a] = true
	got, err := f.client(pmKey).Cached(context.Background(), []string{strings.ToUpper(a), b, a, " "})
	if err != nil || len(got) != 2 || !got[a] || got[b] {
		t.Fatalf("got %v %v", got, err)
	}
	svc, err := New(Premiumize, "key", http.DefaultClient)
	if err != nil || !CanCheckCache(svc) || svc.Name() != Premiumize || Label(Premiumize) != "Premiumize" {
		t.Fatalf("premiumize should offer the cache check: %v", err)
	}
}

func TestPremiumizeCachedBatchesAndShortAnswer(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	short := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		items := r.PostForm["items[]"]
		mu.Lock()
		sizes = append(sizes, len(items))
		n := len(items)
		if short {
			n--
		}
		mu.Unlock()
		resp := make([]bool, n)
		for i := range resp {
			resp[i] = i == 0
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "success", "response": resp})
	}))
	defer srv.Close()
	var hashes []string
	for i := range 120 {
		hashes = append(hashes, fmt.Sprintf("%040x", i+1))
	}
	p := &premiumize{key: "k", http: srv.Client(), base: srv.URL}
	got, err := p.Cached(context.Background(), hashes)
	if err != nil || len(got) != 120 || len(sizes) != 3 || sizes[0] != 50 || sizes[2] != 20 {
		t.Fatalf("batches %v, %d answers, %v", sizes, len(got), err)
	}
	short = true
	if _, err := p.Cached(context.Background(), hashes[:3]); err == nil {
		t.Fatal("a list of the wrong length must be an error, not a guess")
	}
}

func TestPremiumizeListStates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"status":"success","transfers":[
			{"id":"1","name":"Run","status":"running","progress":0.25,"message":"Downloading 3 MB/s"},
			{"id":"2","name":"Done","status":"finished","progress":1},
			{"id":"3","name":"Bad","status":"error","message":"Torrent is dead","progress":null},
			{"id":"4","name":"Wait","status":"waiting"},
			{"id":"5","name":"Gone","status":"deleted"},
			{"id":"6","name":"Late","status":"timeout","message":"Timed out"}]}`)
	}))
	defer srv.Close()
	got, err := (&premiumize{key: "k", http: srv.Client(), base: srv.URL}).List(context.Background())
	if err != nil || len(got) != 6 {
		t.Fatalf("%+v %v", got, err)
	}
	want := []State{Downloading, Ready, Failed, Waiting, Failed, Failed}
	for i, w := range want {
		if got[i].State != w || got[i].Seeders != -1 {
			t.Errorf("item %d: %+v, want state %s", i, got[i], w)
		}
	}
	if got[0].Progress != 0.25 || got[1].Progress != 1 || got[2].Text != "Torrent is dead" {
		t.Errorf("progress or text: %+v", got)
	}
}

// Several downloads waiting at once ask Premiumize for the list once.
func TestPremiumizeSharesTheTransferList(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		_, _ = io.WriteString(w, `{"status":"success","transfers":[{"id":"1","status":"running","progress":0.1},{"id":"2","status":"waiting"}]}`)
	}))
	defer srv.Close()
	// Every download has a client of its own.
	a := &premiumize{key: "k", http: srv.Client(), base: srv.URL, listFor: time.Minute}
	b := &premiumize{key: "k", http: srv.Client(), base: srv.URL, listFor: time.Minute}
	for i, id := range []string{"1", "2", "1", "2"} {
		p := a
		if i%2 == 1 {
			p = b
		}
		if _, err := p.Status(context.Background(), id); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("%d transfer lists asked for, want 1", calls)
	}
}

func TestMagnetHash(t *testing.T) {
	cases := map[string]string{
		"magnet:?xt=urn:btih:0123456789ABCDEF0123456789abcdef01234567&dn=x": "0123456789abcdef0123456789abcdef01234567",
		"magnet:?dn=x&xt=urn:btih:0123456789abcdef0123456789abcdef01234567": "0123456789abcdef0123456789abcdef01234567",
		"magnet:?xt=urn:btih:ABCDEFGHIJKLMNOPQRSTUVWXYZ234567":              "abcdefghijklmnopqrstuvwxyz234567",
		"magnet:?xt=urn:btih:short":                                         "",
		"https://example.com/file.torrent":                                  "",
		"":                                                                  "",
	}
	for in, want := range cases {
		if got := magnetHash(in); got != want {
			t.Errorf("magnetHash(%q) = %q, want %q", in, got, want)
		}
	}
}
