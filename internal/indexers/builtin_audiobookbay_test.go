package indexers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Reduced fixture shaped like the public site: format/size markup and a
// separate details page with a public info hash, without a torrent-file link.
const abbSearch = `<div class="post"><div class="postTitle"><h2><a href="/abss/example/">Example Book - Example Author</a></h2></div><div class="postContent">Posted: 11 Jan 2016<br>Format: <span>MP3 </span> / Bitrate: ?<br>File Size: <span>278.19</span> MBs</div></div>`

func TestAudioBookBayDefinition(t *testing.T) {
	def, err := ParseDefinition(builtinDefinitions()["audiobookbay"])
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Validate(); err != nil {
		t.Fatal(err)
	}
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.URL.Query().Get("tt") != "1" {
			http.NotFound(w, r)
			return
		}
		if r.UserAgent() != "Mozilla/5.0" || r.Header.Get("Accept") != "text/html" {
			t.Errorf("missing site headers on %s: UA=%q Accept=%q", r.URL, r.UserAgent(), r.Header.Get("Accept"))
		}
		if r.URL.Path == "/abss/example/" {
			fmt.Fprint(w, `<div class="postTitle"><h1>Example Book - Example Author</h1></div><table><tr><td>Info Hash:</td><td>0123456789abcdef0123456789abcdef01234567</td></tr></table>`)
			return
		}
		if q := r.URL.Query().Get("s"); q != "" && q != "example book author" {
			t.Errorf("query = %q", q)
		}
		fmt.Fprint(w, abbSearch)
	}))
	defer site.Close()
	m := fixtureManager(t, "fixturepublic")
	inst := Instance{ID: 41, Name: "AudioBook Bay", Kind: KindCardigann, DefinitionID: "audiobookbay", BaseURL: site.URL, Protocol: ProtocolTorrent}
	results, err := m.Search(context.Background(), inst, "Example & Book / Author", []int{3030})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d", len(results))
	}
	r := results[0]
	if r.Title != "Example Book - Example Author [MP3]" || r.SizeBytes != 291703357 || r.SeedersKnown || !hasCat(r.Categories, 3030) || r.PublishDate.Year() != 2016 {
		t.Fatalf("bad mapping: %+v", r)
	}
	_, link, ok := DecodeLinkRef(r.DownloadURL)
	if !ok {
		t.Fatal("missing indexer download reference")
	}
	d, err := m.Download(context.Background(), inst, link)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(d.Magnet)
	if err != nil || u.Query().Get("xt") != "urn:btih:0123456789abcdef0123456789abcdef01234567" || !strings.Contains(u.Query().Get("dn"), "Example Book") {
		t.Fatalf("bad magnet: %+v", d)
	}
	// Regression: Test used to filter out every book-only site's results.
	tr, err := m.Test(context.Background(), inst)
	if err != nil || tr.Releases != 1 {
		t.Fatalf("connection test: %+v, %v", tr, err)
	}
	other, err := m.Search(context.Background(), inst, "", []int{7020})
	if err != nil || len(other) != 0 {
		t.Fatalf("ebook search: %+v, %v", other, err)
	}
}
