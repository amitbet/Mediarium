package indexers

import (
	"context"
	"testing"
)

func defWithCategories(t *testing.T, mappings string) *Definition {
	t.Helper()
	def, err := ParseDefinition([]byte("id: x\nname: X\nlinks: [https://x.test/]\ncaps:\n  categorymappings:\n" + mappings + "\nsearch:\n  paths: [{path: s}]\n  rows: {selector: tr}\n  fields: {title: {selector: a}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	return def
}

func TestDefinitionServesCategories(t *testing.T) {
	var (
		ebooks     = []int{7000, 7020}
		audiobooks = []int{3030}
		music      = []int{3000, 3010, 3040, 3050}
	)
	tests := []struct {
		name     string
		mappings string
		cats     []int
		want     bool
	}{
		{"movie site, ebooks", "    - {id: 1, cat: Movies/HD, desc: HD}\n    - {id: 2, cat: TV/HD, desc: TV}", ebooks, false},
		{"movie site, audiobooks", "    - {id: 1, cat: Movies/HD, desc: HD}", audiobooks, false},
		{"movie site, music", "    - {id: 1, cat: Movies/HD, desc: HD}", music, false},
		{"book site, ebooks", "    - {id: 7, cat: Books/EBook, desc: Books}", ebooks, true},
		{"book site, audiobooks", "    - {id: 7, cat: Books/EBook, desc: Books}", audiobooks, false},
		{"comics only still counts as books", "    - {id: 7, cat: Books/Comics, desc: Comics}", ebooks, true},
		{"audiobook category", "    - {id: 9, cat: Audio/Audiobook, desc: Audiobooks}", audiobooks, true},
		{"audio parent may hold audiobooks", "    - {id: 9, cat: Audio, desc: Audio}", audiobooks, true},
		{"mp3 only does not hold audiobooks", "    - {id: 9, cat: Audio/MP3, desc: MP3}", audiobooks, false},
		{"lossless only, music", "    - {id: 9, cat: Audio/Lossless, desc: FLAC}", music, true},
		{"mixed site", "    - {id: 1, cat: Movies/HD, desc: HD}\n    - {id: 7, cat: Books/EBook, desc: Books}", ebooks, true},
		{"nothing requested", "    - {id: 1, cat: Movies/HD, desc: HD}", nil, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := defWithCategories(t, tc.mappings).servesCategories(tc.cats); got != tc.want {
				t.Errorf("servesCategories(%v) = %v, want %v", tc.cats, got, tc.want)
			}
		})
	}
}

// A site whose definition lists no categories is not given up on: its search
// asks for none either.
func TestDefinitionWithoutCategoriesServesEverything(t *testing.T) {
	def, err := ParseDefinition([]byte("id: x\nname: X\nlinks: [https://x.test/]\nsearch:\n  paths: [{path: s}]\n  rows: {selector: tr}\n  fields: {title: {selector: a}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !def.servesCategories([]int{7000, 7020}) {
		t.Error("a definition with no category list was skipped")
	}
}

func TestSplitByCategoriesAlwaysAsksApiIndexers(t *testing.T) {
	api := Instance{ID: 1, Name: "Newznab", Kind: KindNewznab, Enabled: true}
	broken := Instance{ID: 2, Name: "Site", Kind: KindCardigann, Enabled: true} // no manager: can't tell, so it is asked
	capable, skipped := SplitByCategories(context.Background(), []Instance{api, broken}, []int{7000})
	if len(capable) != 2 || len(skipped) != 0 {
		t.Errorf("capable %d, skipped %d; want 2 and 0", len(capable), len(skipped))
	}
}
