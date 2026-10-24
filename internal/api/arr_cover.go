package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/metadata"
)

// Pictures and credits of the arr-style API (see arr.go): the images a title
// carries, the "/MediaCover" addresses an app builds from them, and Radarr's
// /credit.

// arrCoverImages are the pictures of a title that is in the library: the
// poster, the fanart and (for a show) the banner, each with the local address
// the real apps send ("/MediaCover/<id>/poster.jpg", answered under both the
// app's own prefix and its API) and the TMDB address in remoteUrl. A title
// with no wide picture known yet gets its poster there, so a card that wants a
// wide image still shows something. Nothing at all when there is no poster.
func arrCoverImages(app arrApp, id int64, poster, backdrop string) []arrImage {
	posterURL := metadata.PosterURL(poster)
	wideURL := metadata.BackdropURL(backdrop)
	if wideURL == "" {
		wideURL = posterURL
	}
	if posterURL == "" && wideURL == "" {
		return []arrImage{}
	}
	local := func(kind, remote string) arrImage {
		u := remote
		if id > 0 {
			u = fmt.Sprintf("/MediaCover/%d/%s.jpg", id, kind)
		}
		return arrImage{CoverType: kind, URL: u, RemoteURL: remote}
	}
	out := []arrImage{}
	if posterURL != "" {
		out = append(out, local("poster", posterURL))
	}
	out = append(out, local("fanart", wideURL))
	if app == arrSonarr {
		out = append(out, local("banner", wideURL))
	}
	return out
}

// arrCoverFile splits "poster-250.jpg" into the kind ("poster") and the size
// asked for (250; 0 when none is).
func arrCoverFile(file string) (kind string, size int) {
	name := strings.ToLower(file)
	if i := strings.LastIndex(name, "."); i > 0 {
		name = name[:i]
	}
	kind = name
	if i := strings.Index(name, "-"); i > 0 {
		kind = name[:i]
		size, _ = strconv.Atoi(name[i+1:])
	}
	return kind, size
}

// arrCoverRemote picks the TMDB address of one picture: the poster at the
// size asked for, or the wide picture (the poster when none is known).
func arrCoverRemote(kind string, size int, poster, backdrop string) string {
	switch kind {
	case "poster", "cover", "logo", "clearlogo", "disc":
		switch {
		case poster == "":
			return ""
		case size > 0 && size <= 250:
			return metadata.ImageHost + "/t/p/w342" + poster
		case size > 500:
			return metadata.ImageHost + "/t/p/w780" + poster
		}
		return metadata.PosterURL(poster)
	}
	if u := metadata.BackdropURL(backdrop); u != "" {
		return u
	}
	return metadata.PosterURL(poster)
}

// arrMediaCover answers GET /MediaCover/<id>/<poster|fanart|banner>[-250].jpg
// (and the same under the API path) by sending the app to the TMDB picture,
// with a day of caching. Nothing is downloaded or kept here.
func (s *Server) arrMediaCover(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			arrNotFound(w)
			return
		}
		kind, size := arrCoverFile(r.PathValue("file"))
		var poster string
		var tmdbID int
		art := "movie"
		if app == arrSonarr {
			art = "tv"
			sr, err := s.MovieRepo.GetSeries(id)
			if err != nil {
				arrNotFound(w)
				return
			}
			poster, tmdbID = sr.PosterPath, sr.TMDBID
		} else {
			m, err := s.MovieRepo.Get(id)
			if err != nil {
				arrNotFound(w)
				return
			}
			poster, tmdbID = m.PosterPath, m.TMDBID
		}
		backdrop := ""
		if kind != "poster" && tmdbID > 0 {
			backdrop = s.coverBackdrop(r, art, tmdbID)
		}
		if !metadata.ValidImagePath(poster) {
			poster = ""
		}
		target := arrCoverRemote(kind, size, poster, backdrop)
		if target == "" {
			arrNotFound(w)
			return
		}
		w.Header().Set("Cache-Control", "public, max-age=86400")
		http.Redirect(w, r, target, http.StatusFound)
	}
}

// coverBackdrop is the wide picture saved for a title, fetched from TMDB once
// when it was never asked for (and a key is set).
func (s *Server) coverBackdrop(r *http.Request, kind string, tmdbID int) string {
	if found, err := s.MovieRepo.TitleArt(kind, []int{tmdbID}); err == nil {
		if a, ok := found[tmdbID]; ok {
			return a.Backdrop
		}
	}
	c := s.TMDB()
	if c == nil || !c.HasAPIKey() {
		return ""
	}
	art, err := c.TitleArtwork(r.Context(), kind, tmdbID)
	if err != nil {
		slog.Debug("arr: couldn't fetch artwork for a cover", "kind", kind, "tmdb", tmdbID, "err", err)
		return ""
	}
	if err := s.MovieRepo.SaveTitleArt(kind, tmdbID, art.Backdrop, art.Logo, time.Now()); err != nil {
		slog.Warn("arr: couldn't save the artwork", "err", err)
	}
	return art.Backdrop
}

// arrCredit is one line of Radarr's /credit: a cast or crew member.
type arrCredit struct {
	ID              int64      `json:"id"`
	PersonName      string     `json:"personName"`
	CreditTMDBID    string     `json:"creditTmdbId"`
	PersonTMDBID    int        `json:"personTmdbId"`
	MovieMetadataID int64      `json:"movieMetadataId"`
	Images          []arrImage `json:"images"`
	Department      string     `json:"department,omitempty"`
	Job             string     `json:"job,omitempty"`
	Character       string     `json:"character,omitempty"`
	Order           int        `json:"order"`
	Type            string     `json:"type"`
}

// arrDepartment is the department of a crew job, for a crew list that does
// not carry one.
func arrDepartment(job string) string {
	switch job {
	case "Director":
		return "Directing"
	case "Screenplay", "Writer", "Story", "Novel", "Characters":
		return "Writing"
	}
	return "Crew"
}

// arrCredits answers GET /credit?movieId=: the cast and crew TMDB lists for a
// library movie, as Radarr's credits. The answer is an empty list when TMDB
// has no key here or cannot be reached.
func (s *Server) arrCredits(w http.ResponseWriter, r *http.Request) {
	out := []arrCredit{}
	id := arrIDParam(r, "movieId", "movieid", "movieMetadataId")
	if id <= 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	m, err := s.MovieRepo.Get(id)
	if err != nil {
		writeJSON(w, http.StatusOK, out)
		return
	}
	c := s.TMDB()
	if c == nil || !c.HasAPIKey() || m.TMDBID == 0 {
		writeJSON(w, http.StatusOK, out)
		return
	}
	d, err := c.GetMovieDetail(r.Context(), m.TMDBID)
	if err != nil {
		slog.Debug("arr: couldn't read the credits", "movie", id, "err", err)
		writeJSON(w, http.StatusOK, out)
		return
	}
	face := func(path string) []arrImage {
		u := metadata.ProfileURL(path)
		if u == "" {
			return []arrImage{}
		}
		return []arrImage{{CoverType: "headshot", URL: u, RemoteURL: u}}
	}
	for i, cm := range d.Credits.Cast {
		out = append(out, arrCredit{ID: int64(i + 1), PersonName: cm.Name, CreditTMDBID: strconv.Itoa(cm.ID), PersonTMDBID: cm.ID,
			MovieMetadataID: m.ID, Images: face(cm.ProfilePath), Character: cm.Character, Order: cm.Order, Type: "cast"})
	}
	n := len(out)
	for i, cm := range d.Credits.Crew {
		dep := cm.Department
		if dep == "" {
			dep = arrDepartment(cm.Job)
		}
		out = append(out, arrCredit{ID: int64(n + i + 1), PersonName: cm.Name, CreditTMDBID: strconv.Itoa(cm.ID), PersonTMDBID: cm.ID,
			MovieMetadataID: m.ID, Images: face(cm.ProfilePath), Department: dep, Job: cm.Job, Order: i, Type: "crew"})
	}
	writeJSON(w, http.StatusOK, out)
}

// arrArtOf is the saved wide pictures of the library's titles by TMDB id
// (kind "movie" or "tv"), for turning many titles into arr objects.
func (s *Server) arrArtOf(kind string) map[int]string {
	out := map[int]string{}
	if all, err := s.MovieRepo.AllTitleArt(kind); err == nil {
		for id, a := range all {
			out[id] = a.Backdrop
		}
	}
	return out
}
