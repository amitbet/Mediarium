package library

import (
	"fmt"
	"strings"
	"time"
)

// SavedReleaseDates are a movie's cinema and digital dates as read for one
// region, with when TMDB was asked.
type SavedReleaseDates struct {
	Region, Cinema, Digital string
	CheckedAt               string
}

// MovieReleaseDates returns the saved dates of the given TMDB ids; a movie
// nobody has asked TMDB about yet is missing from the map.
func (r *Repo) MovieReleaseDates(ids []int) (map[int]SavedReleaseDates, error) {
	out := map[int]SavedReleaseDates{}
	if len(ids) == 0 {
		return out, nil
	}
	marks := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		marks[i], args[i] = "?", id
	}
	rows, err := r.db.Query(`SELECT tmdb_id, region, cinema, digital, checked_at FROM movie_release_dates WHERE tmdb_id IN (`+strings.Join(marks, ",")+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("read movie release dates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var d SavedReleaseDates
		if err := rows.Scan(&id, &d.Region, &d.Cinema, &d.Digital, &d.CheckedAt); err != nil {
			return nil, fmt.Errorf("scan movie release dates: %w", err)
		}
		out[id] = d
	}
	return out, rows.Err()
}

// ReleaseDateCandidates are the TMDB ids of the monitored main movies released
// on or after since, nearest to that date first: the ones whose digital date
// the calendar may want.
func (r *Repo) ReleaseDateCandidates(since string) ([]int, error) {
	rows, err := r.db.Query(`SELECT tmdb_id FROM movies
		WHERE copy_of IS NULL AND monitored = 1 AND COALESCE(release_date, '') >= ? AND tmdb_id > 0
		ORDER BY release_date`, since)
	if err != nil {
		return nil, fmt.Errorf("list release date candidates: %w", err)
	}
	defer rows.Close()
	var out []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan release date candidate: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DigitalCalendarMovie is a monitored movie with a digital date on the calendar.
type DigitalCalendarMovie struct {
	CalendarMovie
	Digital string
}

// DigitalCalendarMovies lists the monitored main movies whose saved digital
// date is on or after since, whatever their cinema date.
func (r *Repo) DigitalCalendarMovies(since string) ([]DigitalCalendarMovie, error) {
	rows, err := r.db.Query(`SELECT m.id, m.tmdb_id, m.title, COALESCE(m.poster_path, ''), COALESCE(m.release_date, ''), m.status, d.digital
		FROM movies m JOIN movie_release_dates d ON d.tmdb_id = m.tmdb_id
		WHERE m.copy_of IS NULL AND m.monitored = 1 AND d.digital <> '' AND d.digital >= ?
		ORDER BY d.digital`, since)
	if err != nil {
		return nil, fmt.Errorf("list digital calendar movies: %w", err)
	}
	defer rows.Close()
	var out []DigitalCalendarMovie
	for rows.Next() {
		var m DigitalCalendarMovie
		var status string
		if err := rows.Scan(&m.ID, &m.TMDBID, &m.Title, &m.PosterPath, &m.ReleaseDate, &status, &m.Digital); err != nil {
			return nil, fmt.Errorf("scan digital calendar movie: %w", err)
		}
		m.Status = Status(status)
		out = append(out, m)
	}
	return out, rows.Err()
}

// SaveMovieReleaseDates records the dates read for a movie (empty ones are
// fine: they say the movie was looked at).
func (r *Repo) SaveMovieReleaseDates(tmdbID int, region, cinema, digital string, now time.Time) error {
	_, err := r.db.Exec(`INSERT INTO movie_release_dates (tmdb_id, region, cinema, digital, checked_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (tmdb_id) DO UPDATE SET region = excluded.region, cinema = excluded.cinema, digital = excluded.digital, checked_at = excluded.checked_at`,
		tmdbID, region, cinema, digital, now.UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save movie release dates: %w", err)
	}
	return nil
}

// ClearMovieReleaseDates forgets every saved date, so the calendar reads them
// again (for another country, after the setting changed).
func (r *Repo) ClearMovieReleaseDates() error {
	if _, err := r.db.Exec(`DELETE FROM movie_release_dates`); err != nil {
		return fmt.Errorf("clear movie release dates: %w", err)
	}
	return nil
}
