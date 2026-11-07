package queue_test

import (
	"testing"

	"github.com/rdborg/mediarium/internal/queue"
)

// A release picked by hand remembers that the person agreed to replace the
// file, and a kept download remembers which file it kept: both survive a
// restart (they are columns), and an ordinary download has neither.
func TestItemRemembersReplaceByHandAndTheKeptFile(t *testing.T) {
	db := openDB(t)
	if _, err := db.Exec(`INSERT INTO movies (id, tmdb_id, title, year, status) VALUES (1, 101, 'A', 2000, 'downloaded')`); err != nil {
		t.Fatal(err)
	}
	repo := queue.NewRepo(db)
	for _, byHand := range []bool{false, true} {
		id, err := repo.Enqueue(queue.Item{MovieID: 1, ReleaseTitle: "A.2000.1080p", NZBURL: "http://x/a.nzb", ReplaceByHand: byHand})
		if err != nil {
			t.Fatal(err)
		}
		got, err := repo.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if got.ReplaceByHand != byHand || got.KeptFile != "" || got.KeptData != "" {
			t.Fatalf("by hand %v: got %+v", byHand, got)
		}
		if err := repo.SetKept(id, "/movies/A (2000)/A (2000).mkv", queue.KeptSeeding); err != nil {
			t.Fatal(err)
		}
		got, _ = repo.Get(id)
		if got.KeptFile != "/movies/A (2000)/A (2000).mkv" || got.KeptData != queue.KeptSeeding || got.ReplaceByHand != byHand {
			t.Fatalf("after SetKept: %+v", got)
		}
		all, err := repo.List()
		if err != nil {
			t.Fatal(err)
		}
		var listed *queue.Item
		for i := range all {
			if all[i].ID == id {
				listed = &all[i]
			}
		}
		if listed == nil || listed.KeptFile != got.KeptFile || listed.KeptData != queue.KeptSeeding || listed.ReplaceByHand != byHand {
			t.Fatalf("List: %+v", listed)
		}
	}
}
