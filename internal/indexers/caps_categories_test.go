package indexers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
)

// The capabilities of an indexer become suggestions: every category and
// sub-category it lists, by number, with a name that says where it belongs.
func TestIndexerCategoriesFromCaps(t *testing.T) {
	const caps = `<?xml version="1.0" encoding="UTF-8"?><caps><server title="x"/><categories>
<category id="7000" name="Books"><subcat id="7020" name="Ebook"/><subcat id="7120" name="German"/></category>
<category id="105000" name="TV Special"/>
<category id="2000" name="Movies"/>
<category id="abc" name="Broken"/>
<category id="7020" name="Again"/>
</categories></caps>`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("t") != "caps" {
			t.Errorf("asked for t=%q, want caps", r.URL.Query().Get("t"))
		}
		_, _ = w.Write([]byte(caps))
	}))
	defer srv.Close()
	got, err := indexers.NewNewznabClient("x", srv.URL, "k").Categories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []indexers.CapCategory{{ID: 2000, Name: "Movies"}, {ID: 7000, Name: "Books"}, {ID: 7020, Name: "Books/Ebook"}, {ID: 7120, Name: "Books/German"}, {ID: 105000, Name: "TV Special"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
