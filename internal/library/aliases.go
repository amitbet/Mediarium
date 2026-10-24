package library

import (
	"encoding/json"
	"strings"
)

// A show's other names (the romaji title of an anime, the names of its
// seasons) are kept with its numbering in series_numbering, which the
// animemap package fills. The library only reads them, so release names that
// use one of them are taken for the show. A second copy of a show uses its
// main show's names.

func decodeAliases(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

// loadAliases fills in s.Aliases: the names a person added for the show first,
// then the ones read from the services. A problem reading them leaves the show
// without: they only add matches.
func (r *Repo) loadAliases(s *Series) {
	var raw, own string
	err := r.db.QueryRow(`SELECT COALESCE((SELECT n.aliases FROM series_numbering n WHERE n.series_id = COALESCE(s.copy_of, s.id)), ''),
		COALESCE((SELECT c.names FROM series_numbering_custom c WHERE c.series_id = COALESCE(s.copy_of, s.id)), '')
		FROM series s WHERE s.id = ?`, s.ID).Scan(&raw, &own)
	if err != nil {
		return
	}
	s.Aliases = mergeAliases(decodeAliases(own), decodeAliases(raw))
}

// mergeAliases joins lists of names, dropping one that is already there
// (ignoring case).
func mergeAliases(lists ...[]string) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range lists {
		for _, a := range l {
			if k := strings.ToLower(strings.TrimSpace(a)); k != "" && !seen[k] {
				seen[k] = true
				out = append(out, a)
			}
		}
	}
	return out
}

// loadAllAliases fills in the aliases of every show in list with two queries.
func (r *Repo) loadAllAliases(list []Series) {
	if len(list) == 0 {
		return
	}
	read := func(query string) map[int64][]string {
		out := map[int64][]string{}
		rows, err := r.db.Query(query)
		if err != nil {
			return out
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var raw string
			if rows.Scan(&id, &raw) == nil {
				out[id] = decodeAliases(raw)
			}
		}
		return out
	}
	byID := read(`SELECT s.id, n.aliases FROM series s JOIN series_numbering n ON n.series_id = COALESCE(s.copy_of, s.id) WHERE n.aliases != ''`)
	own := read(`SELECT s.id, c.names FROM series s JOIN series_numbering_custom c ON c.series_id = COALESCE(s.copy_of, s.id) WHERE c.names != ''`)
	for i := range list {
		list[i].Aliases = mergeAliases(own[list[i].ID], byID[list[i].ID])
	}
}
