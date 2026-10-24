package library

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// What the video files hold (codec, size, HDR, audio and subtitle tracks), as
// read from their headers by internal/mediainfo. One row per file path.

// MediaSummary is the part of a file's details the Library lists filter on.
// Each list is comma separated: "en,de".
type MediaSummary struct {
	VCodec  string
	HDR     string
	Res     string
	ACodecs string
	ALangs  string
	SLangs  string
}

// MediaInfoRow is the saved details of one file.
type MediaInfoRow struct {
	MediaSummary
	Path      string
	Size      int64
	MTime     int64 // UnixNano
	ScannedAt string
	CheckedAt string
	Failed    bool   // the headers could not be read
	Info      string // the full result as JSON
	Container string
}

const mediaInfoCols = `path, size, mtime, scanned_at, checked_at, failed, info, container, vcodec, hdr, res, acodecs, alangs, slangs`

func scanMediaInfo(sc interface{ Scan(...any) error }) (MediaInfoRow, error) {
	var m MediaInfoRow
	var failed int
	err := sc.Scan(&m.Path, &m.Size, &m.MTime, &m.ScannedAt, &m.CheckedAt, &failed, &m.Info, &m.Container,
		&m.VCodec, &m.HDR, &m.Res, &m.ACodecs, &m.ALangs, &m.SLangs)
	m.Failed = failed != 0
	return m, err
}

// GetMediaInfo returns the saved details of the file at path.
func (r *Repo) GetMediaInfo(path string) (MediaInfoRow, bool, error) {
	m, err := scanMediaInfo(r.db.QueryRow(`SELECT `+mediaInfoCols+` FROM media_info WHERE path = ?`, path))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaInfoRow{}, false, nil
	}
	if err != nil {
		return MediaInfoRow{}, false, fmt.Errorf("get media info: %w", err)
	}
	return m, true, nil
}

// SaveMediaInfo stores the details of a file, replacing earlier ones.
func (r *Repo) SaveMediaInfo(m MediaInfoRow) error {
	failed := 0
	if m.Failed {
		failed = 1
	}
	if m.CheckedAt == "" {
		m.CheckedAt = m.ScannedAt
	}
	_, err := r.db.Exec(`INSERT OR REPLACE INTO media_info (`+mediaInfoCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.Path, m.Size, m.MTime, m.ScannedAt, m.CheckedAt, failed, m.Info, m.Container, m.VCodec, m.HDR, m.Res, m.ACodecs, m.ALangs, m.SLangs)
	if err != nil {
		return fmt.Errorf("save media info: %w", err)
	}
	return nil
}

// TouchMediaInfo notes that the file was looked at and had not changed.
func (r *Repo) TouchMediaInfo(path string, at time.Time) error {
	if _, err := r.db.Exec(`UPDATE media_info SET checked_at = ? WHERE path = ?`, at.UTC().Format(time.RFC3339), path); err != nil {
		return fmt.Errorf("touch media info: %w", err)
	}
	return nil
}

// downloadedFiles is every downloaded movie and episode file path, once.
const downloadedFiles = `SELECT file_path FROM movies WHERE status = 'downloaded' AND COALESCE(file_path, '') != ''
	UNION SELECT file_path FROM episodes WHERE status = 'downloaded' AND COALESCE(file_path, '') != ''`

// MediaInfoPending lists up to limit downloaded files that have not been read
// yet, in path order, starting after the path after (empty: from the start).
func (r *Repo) MediaInfoPending(after string, limit int) ([]string, error) {
	rows, err := r.db.Query(`SELECT file_path FROM (`+downloadedFiles+`) f
		WHERE file_path > ? AND NOT EXISTS (SELECT 1 FROM media_info m WHERE m.path = f.file_path)
		ORDER BY file_path LIMIT ?`, after, limit)
	if err != nil {
		return nil, fmt.Errorf("list files to read: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan file to read: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// MediaInfoStale lists up to limit saved files not looked at since before, the
// longest-unchecked first, to see whether they changed.
func (r *Repo) MediaInfoStale(before time.Time, limit int) ([]MediaInfoRow, error) {
	rows, err := r.db.Query(`SELECT `+mediaInfoCols+` FROM media_info WHERE checked_at < ? ORDER BY checked_at LIMIT ?`,
		before.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, fmt.Errorf("list stale media info: %w", err)
	}
	defer rows.Close()
	var out []MediaInfoRow
	for rows.Next() {
		m, err := scanMediaInfo(rows)
		if err != nil {
			return nil, fmt.Errorf("scan stale media info: %w", err)
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// PruneMediaInfo forgets the details of files no movie or episode points to
// any more, and returns how many.
func (r *Repo) PruneMediaInfo() (int64, error) {
	res, err := r.db.Exec(`DELETE FROM media_info WHERE path NOT IN (SELECT file_path FROM movies WHERE COALESCE(file_path, '') != ''
		UNION SELECT file_path FROM episodes WHERE COALESCE(file_path, '') != '')`)
	if err != nil {
		return 0, fmt.Errorf("prune media info: %w", err)
	}
	return res.RowsAffected()
}

// MediaInfoCounts is how many downloaded files there are and how many of them
// have been read.
func (r *Repo) MediaInfoCounts() (done, total int, err error) {
	if err = r.db.QueryRow(`SELECT COUNT(*) FROM (` + downloadedFiles + `)`).Scan(&total); err != nil {
		return 0, 0, fmt.Errorf("count files: %w", err)
	}
	if err = r.db.QueryRow(`SELECT COUNT(*) FROM (` + downloadedFiles + `) f WHERE EXISTS (SELECT 1 FROM media_info m WHERE m.path = f.file_path)`).Scan(&done); err != nil {
		return 0, 0, fmt.Errorf("count read files: %w", err)
	}
	return done, total, nil
}

const summaryCols = `mi.vcodec, mi.hdr, mi.res, mi.acodecs, mi.alangs, mi.slangs`

// MovieMediaSummaries maps each movie with a file that has been read to what
// its file holds.
func (r *Repo) MovieMediaSummaries() (map[int64]MediaSummary, error) {
	rows, err := r.db.Query(`SELECT m.id, ` + summaryCols + ` FROM movies m JOIN media_info mi ON mi.path = m.file_path WHERE mi.failed = 0 AND m.status = 'downloaded'`)
	if err != nil {
		return nil, fmt.Errorf("movie media summaries: %w", err)
	}
	defer rows.Close()
	out := map[int64]MediaSummary{}
	for rows.Next() {
		var id int64
		var s MediaSummary
		if err := rows.Scan(&id, &s.VCodec, &s.HDR, &s.Res, &s.ACodecs, &s.ALangs, &s.SLangs); err != nil {
			return nil, fmt.Errorf("scan movie media summary: %w", err)
		}
		out[id] = s
	}
	return out, rows.Err()
}

// SeriesMediaSummaries maps each show to the different file summaries its
// downloaded episodes have (one entry per different combination).
func (r *Repo) SeriesMediaSummaries() (map[int64][]MediaSummary, error) {
	rows, err := r.db.Query(`SELECT e.series_id, ` + summaryCols + ` FROM episodes e JOIN media_info mi ON mi.path = e.file_path
		WHERE mi.failed = 0 AND e.status = 'downloaded'
		GROUP BY e.series_id, mi.vcodec, mi.hdr, mi.res, mi.acodecs, mi.alangs, mi.slangs`)
	if err != nil {
		return nil, fmt.Errorf("series media summaries: %w", err)
	}
	defer rows.Close()
	out := map[int64][]MediaSummary{}
	for rows.Next() {
		var id int64
		var s MediaSummary
		if err := rows.Scan(&id, &s.VCodec, &s.HDR, &s.Res, &s.ACodecs, &s.ALangs, &s.SLangs); err != nil {
			return nil, fmt.Errorf("scan series media summary: %w", err)
		}
		out[id] = append(out[id], s)
	}
	return out, rows.Err()
}

// MediaFilePaths maps each movie, and each show, to the paths of its
// downloaded files that have been read, for looking beside them (subtitle
// files next to the video).
func (r *Repo) MediaFilePaths() (movies map[int64]string, series map[int64][]string, err error) {
	movies, series = map[int64]string{}, map[int64][]string{}
	rows, err := r.db.Query(`SELECT m.id, m.file_path FROM movies m JOIN media_info mi ON mi.path = m.file_path WHERE mi.failed = 0 AND m.status = 'downloaded'`)
	if err != nil {
		return nil, nil, fmt.Errorf("movie file paths: %w", err)
	}
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan movie file path: %w", err)
		}
		movies[id] = p
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, fmt.Errorf("movie file paths: %w", err)
	}
	rows.Close()
	rows, err = r.db.Query(`SELECT e.series_id, e.file_path FROM episodes e JOIN media_info mi ON mi.path = e.file_path WHERE mi.failed = 0 AND e.status = 'downloaded'`)
	if err != nil {
		return nil, nil, fmt.Errorf("episode file paths: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, nil, fmt.Errorf("scan episode file path: %w", err)
		}
		series[id] = append(series[id], p)
	}
	return movies, series, rows.Err()
}

// EpisodeMediaInfo maps each downloaded episode of a show to the saved details
// of its file.
func (r *Repo) EpisodeMediaInfo(seriesID int64) (map[int64]MediaInfoRow, error) {
	rows, err := r.db.Query(`SELECT e.id, mi.path, mi.size, mi.mtime, mi.scanned_at, mi.checked_at, mi.failed, mi.info, mi.container,
		mi.vcodec, mi.hdr, mi.res, mi.acodecs, mi.alangs, mi.slangs
		FROM episodes e JOIN media_info mi ON mi.path = e.file_path WHERE e.series_id = ? AND mi.failed = 0`, seriesID)
	if err != nil {
		return nil, fmt.Errorf("episode media info: %w", err)
	}
	defer rows.Close()
	out := map[int64]MediaInfoRow{}
	for rows.Next() {
		var id int64
		var m MediaInfoRow
		var failed int
		if err := rows.Scan(&id, &m.Path, &m.Size, &m.MTime, &m.ScannedAt, &m.CheckedAt, &failed, &m.Info, &m.Container,
			&m.VCodec, &m.HDR, &m.Res, &m.ACodecs, &m.ALangs, &m.SLangs); err != nil {
			return nil, fmt.Errorf("scan episode media info: %w", err)
		}
		out[id] = m
	}
	return out, rows.Err()
}
