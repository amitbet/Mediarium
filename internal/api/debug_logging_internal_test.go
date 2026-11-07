package api

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/rdborg/mediarium/internal/logbuf"
	"github.com/rdborg/mediarium/internal/settings"
)

func TestDetailedLoggingFollowsTheSavedSwitchAtStart(t *testing.T) {
	tests := []struct {
		saved string
		want  bool
	}{{"", false}, {"0", false}, {"1", true}, {"yes", false}}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("saved %q", tc.saved), func(t *testing.T) {
			t.Cleanup(func() { logbuf.SetDebug(false) })
			db, err := OpenTestDB(filepath.Join(t.TempDir(), "app.db"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Close() })
			s := &Server{Settings: settings.New(db, nil)}
			if tc.saved != "" {
				if err := s.Settings.Set(settings.KeyDebugLogging, tc.saved, false); err != nil {
					t.Fatal(err)
				}
			}
			logbuf.SetDebug(!tc.want) // start from the opposite so the call has to change it
			s.applyDebugLogging()
			if logbuf.Debug() != tc.want {
				t.Fatalf("saved %q: detailed logging is %v, want %v", tc.saved, logbuf.Debug(), tc.want)
			}
		})
	}
}
