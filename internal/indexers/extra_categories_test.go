package indexers_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/rdborg/mediarium/internal/indexers"
)

func TestWithExtraCategories(t *testing.T) {
	extra := indexers.ExtraCategories{
		"movies":     {2999},
		"tv":         {105000, 105010},
		"music":      {3999},
		"books":      {7120},
		"audiobooks": {3130},
	}
	tests := []struct {
		name  string
		extra indexers.ExtraCategories
		cats  []int
		want  []int
	}{
		{"tv", extra, []int{5000}, []int{5000, 105000, 105010}},
		{"movies", extra, []int{2000}, []int{2000, 2999}},
		{"movies and tv together", extra, []int{2000, 5000}, []int{2000, 5000, 2999, 105000, 105010}},
		{"ebooks", extra, []int{7000, 7020}, []int{7000, 7020, 7120}},
		{"audiobooks are not music", extra, []int{3030}, []int{3030, 3130}},
		{"music is not audiobooks", extra, []int{3000, 3010, 3040, 3050}, []int{3000, 3010, 3040, 3050, 3999}},
		{"a kind with none set", indexers.ExtraCategories{"tv": {105000}}, []int{2000}, []int{2000}},
		{"nothing set", nil, []int{5000}, []int{5000}},
		{"a search for everything stays one", extra, nil, nil},
		{"a number already asked for is not repeated", indexers.ExtraCategories{"tv": {5000, 105000}}, []int{5000}, []int{5000, 105000}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := append([]int(nil), tt.cats...)
			got := tt.extra.WithExtraCategories(in)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(in, tt.cats) {
				t.Errorf("the list given was changed: %v, was %v", in, tt.cats)
			}
		})
	}
}

func TestExtraCategoriesEncodeDecode(t *testing.T) {
	tests := []struct {
		name string
		in   indexers.ExtraCategories
		want string
	}{
		{"empty", nil, ""},
		{"only empty lists", indexers.ExtraCategories{"tv": {}}, ""},
		{"sorted and without repeats", indexers.ExtraCategories{"tv": {105010, 105000, 105000}, "books": {7120}}, `{"books":[7120],"tv":[105000,105010]}`},
		{"zero and negative numbers are dropped", indexers.ExtraCategories{"tv": {0, -4, 105000}}, `{"tv":[105000]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Encode()
			if got != tt.want {
				t.Fatalf("Encode = %q, want %q", got, tt.want)
			}
			if back := indexers.DecodeExtraCategories(got); back.Encode() != got {
				t.Errorf("decoded %q and encoded again as %q", got, back.Encode())
			}
		})
	}
	for _, bad := range []string{"", "  ", "not json", `{"tv":"105000"}`, `[1,2]`} {
		if got := indexers.DecodeExtraCategories(bad); len(got) != 0 {
			t.Errorf("DecodeExtraCategories(%q) = %v, want nothing", bad, got)
		}
	}
	if got := indexers.DecodeExtraCategories(`{"games":[4000],"tv":[105000]}`); !reflect.DeepEqual(got, indexers.ExtraCategories{"tv": {105000}}) {
		t.Errorf("an unknown kind should be dropped, got %v", got)
	}
}

func TestExtraCategoriesValidate(t *testing.T) {
	tooMany := make([]int, indexers.MaxExtraCategoriesOf+1)
	for i := range tooMany {
		tooMany[i] = 100000 + i
	}
	tests := []struct {
		name    string
		in      indexers.ExtraCategories
		problem string // part of the message; "" = fine
	}{
		{"fine", indexers.ExtraCategories{"tv": {105000}, "books": {7120}}, ""},
		{"nothing", nil, ""},
		{"unknown kind", indexers.ExtraCategories{"games": {4000}}, "not for"},
		{"zero", indexers.ExtraCategories{"tv": {0}}, "not a category number"},
		{"negative", indexers.ExtraCategories{"tv": {-5}}, "not a category number"},
		{"too big", indexers.ExtraCategories{"tv": {indexers.MaxExtraCategory + 1}}, "not a category number"},
		{"the biggest allowed", indexers.ExtraCategories{"tv": {indexers.MaxExtraCategory}}, ""},
		{"too many", indexers.ExtraCategories{"tv": tooMany}, "too many"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in.Validate()
			if tt.problem == "" && got != "" || tt.problem != "" && !strings.Contains(got, tt.problem) {
				t.Errorf("Validate = %q, want it to contain %q", got, tt.problem)
			}
		})
	}
}

func TestParseCategoryList(t *testing.T) {
	tests := []struct {
		in      string
		want    []int
		wantErr bool
	}{
		{"", nil, false},
		{"105000", []int{105000}, false},
		{"105000, 105010", []int{105000, 105010}, false},
		{" 7120 ;7121\t7122 ", []int{7120, 7121, 7122}, false},
		{"105000,", []int{105000}, false},
		{"abc", nil, true},
		{"7120, TV", nil, true},
		{"12.5", nil, true},
		{"0", nil, true},
		{"-7", nil, true},
		{"1000000", nil, true},
	}
	for _, tt := range tests {
		got, err := indexers.ParseCategoryList(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseCategoryList(%q) error = %v, want error %v", tt.in, err, tt.wantErr)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseCategoryList(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestExtraCategoriesAreStored(t *testing.T) {
	repo := newTestRepo(t)
	made, err := repo.Create(indexers.Instance{Name: "A", DefinitionID: "newznab", BaseURL: "https://a.test", Enabled: true,
		ExtraCategories: indexers.ExtraCategories{"tv": {105010, 105000}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := repo.Get(made.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := (indexers.ExtraCategories{"tv": {105000, 105010}}); !reflect.DeepEqual(got.ExtraCategories, want) {
		t.Errorf("stored %v, want %v", got.ExtraCategories, want)
	}
	got.ExtraCategories = indexers.ExtraCategories{"books": {7120}}
	if err := repo.Update(got, nil); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.Get(made.ID); !reflect.DeepEqual(again.ExtraCategories, indexers.ExtraCategories{"books": {7120}}) {
		t.Errorf("after the edit: %v", again.ExtraCategories)
	}
	got.ExtraCategories = nil
	if err := repo.Update(got, nil); err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.Get(made.ID); len(again.ExtraCategories) != 0 {
		t.Errorf("cleared, but still %v", again.ExtraCategories)
	}
	plain, err := repo.Create(indexers.Instance{Name: "B", DefinitionID: "newznab", BaseURL: "https://b.test", Enabled: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := repo.Get(plain.ID); len(again.ExtraCategories) != 0 {
		t.Errorf("a new indexer starts without extra categories, got %v", again.ExtraCategories)
	}
}

// The indexer is asked for the standard categories and its own extra ones, in
// one request, and another indexer is not given them.
func TestSearchAsksForTheExtraCategories(t *testing.T) {
	const feed = `<?xml version="1.0" encoding="UTF-8"?><rss version="2.0"><channel></channel></rss>`
	var mu sync.Mutex
	asked := map[string]string{}
	mk := func(name string) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			asked[name] = r.URL.Query().Get("cat")
			mu.Unlock()
			_, _ = w.Write([]byte(feed))
		}))
	}
	withExtra, plain := mk("extra"), mk("plain")
	defer withExtra.Close()
	defer plain.Close()

	list := []indexers.Instance{
		{ID: 1, Name: "extra", BaseURL: withExtra.URL, Enabled: true, Protocol: indexers.ProtocolUsenet,
			ExtraCategories: indexers.ExtraCategories{"tv": {105000}, "books": {7120}}},
		{ID: 2, Name: "plain", BaseURL: plain.URL, Enabled: true, Protocol: indexers.ProtocolUsenet},
	}
	for _, o := range indexers.SearchAll(context.Background(), list, "show", []int{5000}) {
		if o.Err != nil {
			t.Fatalf("%s: %v", o.IndexerName, o.Err)
		}
	}
	if asked["extra"] != "5000,105000" {
		t.Errorf("the indexer with extra categories was asked for cat=%q, want 5000,105000", asked["extra"])
	}
	if asked["plain"] != "5000" {
		t.Errorf("the other indexer was asked for cat=%q, want 5000", asked["plain"])
	}
}
