package organizer

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"
)

func TestOpenSourceRetriesPermissionErrors(t *testing.T) {
	oldWaits, oldOpen := sourceOpenWaits, openSourceFile
	t.Cleanup(func() { sourceOpenWaits, openSourceFile = oldWaits, oldOpen })
	sourceOpenWaits = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}

	tests := []struct {
		name      string
		failures  int
		failWith  error
		wantCalls int
		wantErr   bool
	}{
		{"opens first time", 0, nil, 1, false},
		{"permission twice then opens", 2, fs.ErrPermission, 3, false},
		{"permission forever gives up", 100, fs.ErrPermission, 4, true},
		{"missing file is not retried", 100, fs.ErrNotExist, 1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			openSourceFile = func(string) (*os.File, error) {
				calls++
				if calls <= tc.failures {
					return nil, tc.failWith
				}
				return os.Open(os.DevNull)
			}
			f, err := openSource("x")
			if f != nil {
				f.Close()
			}
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, tc.failWith) {
				t.Fatalf("err = %v, want %v", err, tc.failWith)
			}
			if calls != tc.wantCalls {
				t.Fatalf("opened %d times, want %d", calls, tc.wantCalls)
			}
		})
	}
}
