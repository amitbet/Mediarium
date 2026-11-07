package library

import "fmt"

// OwnedMovie is the little of a movie that "is it in my library, and how is
// it doing" needs: Discover marks titles with it.
type OwnedMovie struct {
	ID        int64
	TMDBID    int
	Status    Status
	Monitored bool
}

// OwnedShow is the same for a show, with its episode counts.
type OwnedShow struct {
	ID              int64
	TMDBID          int
	Monitored       bool
	EpisodeCount    int
	DownloadedCount int
}

// OwnedMovies lists the library's movies (second copies left out, newest
// first, like List) with only the columns Discover needs: one narrow table
// read instead of the whole movie row.
func (r *Repo) OwnedMovies() ([]OwnedMovie, error) {
	rows, err := r.db.Query(`SELECT id, tmdb_id, status, monitored FROM movies WHERE copy_of IS NULL ORDER BY added_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list owned movies: %w", err)
	}
	defer rows.Close()
	var out []OwnedMovie
	for rows.Next() {
		var m OwnedMovie
		var status string
		if err := rows.Scan(&m.ID, &m.TMDBID, &status, &m.Monitored); err != nil {
			return nil, fmt.Errorf("scan owned movie: %w", err)
		}
		m.Status = Status(status)
		out = append(out, m)
	}
	return out, rows.Err()
}

// OwnedSeries lists the library's shows (second copies left out, newest first,
// like ListSeries) with the same episode counts, but none of the other fields
// and none of the alias joins.
func (r *Repo) OwnedSeries() ([]OwnedShow, error) {
	rows, err := r.db.Query(`
		SELECT s.id, s.tmdb_id, s.monitored,
		       (SELECT COUNT(*) FROM episodes e WHERE e.series_id = s.id AND (e.season > 0 OR e.monitored = 1 OR e.status = 'downloaded')),
		       (SELECT COUNT(*) FROM episodes e WHERE e.series_id = s.id AND e.status = 'downloaded')
		FROM series s WHERE s.copy_of IS NULL ORDER BY s.added_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list owned shows: %w", err)
	}
	defer rows.Close()
	var out []OwnedShow
	for rows.Next() {
		var s OwnedShow
		if err := rows.Scan(&s.ID, &s.TMDBID, &s.Monitored, &s.EpisodeCount, &s.DownloadedCount); err != nil {
			return nil, fmt.Errorf("scan owned show: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
