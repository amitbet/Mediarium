package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rdborg/mediarium/internal/books"
)

// Cover thumbnails. A grid of artists or albums needs a small picture, not the
// 500 px archive cover and certainly not a 3000 px scan from the album folder.
// GET .../cover?size=thumb makes a small JPEG once, keeps it under the config
// folder (music-covers/thumb-*.img) and serves that from then on. The file name
// comes from the source picture's path, size and time, so a changed cover gets
// a new thumbnail by itself and the old one is cleaned up with the cache.

const (
	// thumbMaxSide is the longest side of a thumbnail, in pixels: sharp enough
	// for a card on a high-density screen, a few kilobytes.
	thumbMaxSide = 256
	// thumbQuality is the JPEG quality of a thumbnail.
	thumbQuality = 70
	// thumbVersion changes when the size or quality above do, so old thumbnails
	// are not served in place of the new ones.
	thumbVersion = 1
	// thumbPrefix starts every thumbnail file name in the cache folder.
	thumbPrefix = "thumb-"
	// thumbMaxAge is how long a browser keeps a thumbnail before asking again
	// (it then gets a 304 while the picture is the same).
	thumbMaxAge = 3 * 24 * time.Hour
	// maxThumbSource is the biggest picture file that is read to make one.
	maxThumbSource = 40 << 20
)

// thumbFileName is the cache file for the thumbnail of the picture at src.
func thumbFileName(src string, st os.FileInfo) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d|%s|%d|%d", thumbVersion, src, st.Size(), st.ModTime().UnixNano())))
	return thumbPrefix + hex.EncodeToString(sum[:12]) + ".img"
}

// thumbFor returns the thumbnail file for the picture at src, making it when
// it is not there. Requests for the same thumbnail share one job, and only a
// few pictures are resized at once.
func (s *Server) thumbFor(ctx context.Context, src string) (string, error) {
	st, err := os.Stat(src)
	if err != nil || !st.Mode().IsRegular() {
		return "", fmt.Errorf("cover thumbnail: no picture at %s", src)
	}
	dir := s.coverCacheDir()
	name := thumbFileName(src, st)
	path := filepath.Join(dir, name)
	if ts, err := os.Stat(path); err == nil && ts.Size() > 0 {
		touchCover(path, ts)
		return path, nil
	}
	return s.music.covers.thumbs.do(ctx, name, func() (string, error) {
		if ts, err := os.Stat(path); err == nil && ts.Size() > 0 { // made meanwhile
			return path, nil
		}
		if st.Size() > maxThumbSource {
			return "", fmt.Errorf("cover thumbnail: %s is too big", src)
		}
		wait, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if err := acquireCoverWork(wait); err != nil {
			return "", fmt.Errorf("cover thumbnail: waiting for a free slot: %w", err)
		}
		defer releaseCoverWork()
		f, err := os.Open(src)
		if err != nil {
			return "", fmt.Errorf("cover thumbnail: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(f, maxThumbSource+1))
		f.Close()
		if err != nil {
			return "", fmt.Errorf("cover thumbnail: read %s: %w", src, err)
		}
		out, err := books.ResizeCoverQuality(data, thumbMaxSide, thumbMaxSide, thumbQuality)
		if err != nil {
			return "", fmt.Errorf("cover thumbnail: %w", err)
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("cover thumbnail: %w", err)
		}
		tmp, err := os.CreateTemp(dir, "thumb-*.tmp")
		if err != nil {
			return "", fmt.Errorf("cover thumbnail: %w", err)
		}
		_, werr := tmp.Write(out)
		cerr := tmp.Close()
		if werr != nil || cerr != nil {
			_ = os.Remove(tmp.Name())
			return "", fmt.Errorf("cover thumbnail: %w", errors.Join(werr, cerr))
		}
		if err := os.Rename(tmp.Name(), path); err != nil {
			_ = os.Remove(tmp.Name())
			return "", fmt.Errorf("cover thumbnail: %w", err)
		}
		s.music.covers.noteWrite(dir)
		return path, nil
	})
}
