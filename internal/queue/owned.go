package queue

import "fmt"

// TitleProgress is one line of what the queue says about a movie or a show:
// one per title and status, with how far the furthest row of that status has
// got.
type TitleProgress struct {
	MovieID     int64
	SeriesID    int64
	Status      Status
	ProgressPct float64
}

// TitleProgress summarises the queue rows of movies and shows (albums and
// books are left out, finished ones too) as one line per title and status.
// Discover's "owned" marks need no more than this.
func (r *Repo) TitleProgress() ([]TitleProgress, error) {
	rows, err := r.db.Query(`SELECT COALESCE(movie_id, 0), COALESCE(series_id, 0), status, MAX(progress_pct)
		FROM download_queue
		WHERE (movie_id IS NOT NULL OR series_id IS NOT NULL) AND status != 'completed'
		GROUP BY movie_id, series_id, status
		ORDER BY MIN(id)`)
	if err != nil {
		return nil, fmt.Errorf("summarise queue: %w", err)
	}
	defer rows.Close()
	var out []TitleProgress
	for rows.Next() {
		var p TitleProgress
		var status string
		if err := rows.Scan(&p.MovieID, &p.SeriesID, &status, &p.ProgressPct); err != nil {
			return nil, fmt.Errorf("scan queue summary: %w", err)
		}
		p.Status = Status(status)
		out = append(out, p)
	}
	return out, rows.Err()
}
