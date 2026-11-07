package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/parser"
	"github.com/rdborg/mediarium/internal/quality"
)

// The interactive search of the Radarr and Sonarr apps: GET /release lists
// what the indexers have for one movie, episode or season, and POST /release
// grabs one of them. The lists come from the same search the web app runs, so
// a phone app and the page show the same releases.

// arrOffer is what a release listed for an app stands for, kept so the grab
// that follows (which only names the release by its guid) knows its title and
// what it was found for.
type arrOffer struct {
	title    string
	size     int64
	protocol string
	movieID  int64
	seriesID int64
	season   int
	episode  int
	expires  time.Time
}

const (
	arrOfferTTL = 6 * time.Hour
	arrOfferMax = 5000
)

type arrOffers struct {
	mu sync.Mutex
	m  map[string]arrOffer
}

// arrOfferBooks holds each server's offers (tests run several servers in one
// process).
var arrOfferBooks sync.Map // *Server -> *arrOffers

func (s *Server) arrOfferBook() *arrOffers {
	b, _ := arrOfferBooks.LoadOrStore(s, &arrOffers{m: map[string]arrOffer{}})
	return b.(*arrOffers)
}

func (b *arrOffers) put(guid string, o arrOffer) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	if len(b.m) >= arrOfferMax {
		for k, v := range b.m {
			if now.After(v.expires) {
				delete(b.m, k)
			}
		}
		if len(b.m) >= arrOfferMax { // still full: start over rather than grow
			b.m = map[string]arrOffer{}
		}
	}
	o.expires = now.Add(arrOfferTTL)
	b.m[guid] = o
}

func (b *arrOffers) get(guid string) (arrOffer, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	o, ok := b.m[guid]
	if !ok || time.Now().After(o.expires) {
		return arrOffer{}, false
	}
	return o, true
}

// arrRelease is one release in the shape both apps list. The fields of the
// other app are left out (omitempty) rather than sent empty.
type arrRelease struct {
	GUID                 string            `json:"guid"`
	Quality              arrQualityModel   `json:"quality"`
	QualityWeight        int               `json:"qualityWeight"`
	Age                  int               `json:"age"`
	AgeHours             float64           `json:"ageHours"`
	AgeMinutes           float64           `json:"ageMinutes"`
	Size                 int64             `json:"size"`
	IndexerID            int64             `json:"indexerId"`
	Indexer              string            `json:"indexer"`
	ReleaseGroup         string            `json:"releaseGroup,omitempty"`
	SubGroup             string            `json:"subGroup,omitempty"`
	ReleaseHash          string            `json:"releaseHash"`
	Title                string            `json:"title"`
	SceneSource          bool              `json:"sceneSource"`
	Languages            []arrLanguage     `json:"languages"`
	LanguageWeight       int               `json:"languageWeight"`
	Approved             bool              `json:"approved"`
	TemporarilyRejected  bool              `json:"temporarilyRejected"`
	Rejected             bool              `json:"rejected"`
	Rejections           []string          `json:"rejections"`
	PublishDate          string            `json:"publishDate"`
	CommentURL           string            `json:"commentUrl"`
	DownloadURL          string            `json:"downloadUrl"`
	InfoURL              string            `json:"infoUrl"`
	DownloadAllowed      bool              `json:"downloadAllowed"`
	ReleaseWeight        int               `json:"releaseWeight"`
	CustomFormats        []arrCustomFormat `json:"customFormats"`
	CustomFormatScore    int               `json:"customFormatScore"`
	Protocol             string            `json:"protocol"`
	Seeders              *int              `json:"seeders"`
	Leechers             *int              `json:"leechers"`
	MagnetURL            string            `json:"magnetUrl,omitempty"`
	InfoHash             string            `json:"infoHash,omitempty"`
	IndexerFlags         any               `json:"indexerFlags"`
	MovieTitles          []string          `json:"movieTitles,omitempty"`
	Edition              string            `json:"edition,omitempty"`
	TMDBID               int               `json:"tmdbId,omitempty"`
	IMDBID               string            `json:"imdbId,omitempty"`
	MappedMovieID        int64             `json:"mappedMovieId,omitempty"`
	FullSeason           *bool             `json:"fullSeason,omitempty"`
	SeasonNumber         *int              `json:"seasonNumber,omitempty"`
	SeriesTitle          string            `json:"seriesTitle,omitempty"`
	EpisodeNumbers       []int             `json:"episodeNumbers,omitempty"`
	AbsoluteEpisodeNums  []int             `json:"absoluteEpisodeNumbers,omitempty"`
	MappedSeasonNumber   *int              `json:"mappedSeasonNumber,omitempty"`
	MappedEpisodeNumbers []int             `json:"mappedEpisodeNumbers,omitempty"`
	MappedSeriesID       int64             `json:"mappedSeriesId,omitempty"`
	Special              *bool             `json:"special,omitempty"`
	ReleaseType          string            `json:"releaseType,omitempty"`
}

// arrCustomFormat is a custom format as the apps list it next to a release.
type arrCustomFormat struct {
	ID                              int64  `json:"id"`
	Name                            string `json:"name"`
	IncludeCustomFormatWhenRenaming bool   `json:"includeCustomFormatWhenRenaming"`
	Specifications                  []any  `json:"specifications"`
}

// arrRejections lists, in plain words, everything that keeps an automatic
// search from taking the release: the profile and size reasons, and the flags
// (blocklisted, too few seeders, another language). The "why not the best"
// note is no rejection and is left out.
func arrRejections(p searchResultPayload) []string {
	out := append([]string{}, p.Rejections...)
	for _, r := range p.Reasons {
		switch r.Kind {
		case "blocklist", "seeders", "language":
			out = append(out, r.Text)
		}
	}
	return out
}

// arrReleaseOf turns one search result into the app's release.
func (s *Server) arrReleaseOf(app arrApp, p searchResultPayload, indexerIDs map[string]int64) arrRelease {
	rel := parser.Parse(p.Title)
	tier := quality.Classify(rel)
	age := time.Duration(0)
	pub := arrTime(p.PublishDate)
	if t, err := time.Parse("2006-01-02T15:04:05Z", pub); err == nil {
		age = max(time.Since(t), 0)
	} else {
		pub = "0001-01-01T00:00:00Z"
	}
	rej := arrRejections(p)
	langs := []arrLanguage{arrEnglish}
	if p.LanguageFit == "other" && p.Language != "" {
		langs = []arrLanguage{{ID: 0, Name: p.Language}}
	}
	out := arrRelease{
		GUID: p.DownloadURL, Quality: arrQualityModelOf(app, tier), QualityWeight: quality.Rank(tier) * 100,
		Age: int(age.Hours() / 24), AgeHours: age.Hours(), AgeMinutes: age.Minutes(),
		Size: p.SizeBytes, IndexerID: indexerIDs[p.IndexerName], Indexer: p.IndexerName, ReleaseGroup: p.Group,
		Title: p.Title, Languages: langs, LanguageWeight: 1,
		Approved: len(rej) == 0, Rejected: len(rej) > 0, Rejections: rej,
		PublishDate: pub, DownloadURL: p.DownloadURL, DownloadAllowed: true,
		CustomFormats: []arrCustomFormat{}, Protocol: p.Protocol, InfoHash: p.InfoHash,
	}
	if rel.Repack || rel.Proper {
		out.Quality.Revision = arrRevision{Version: 2, IsRepack: rel.Repack}
	}
	for _, f := range p.Formats {
		out.CustomFormats = append(out.CustomFormats, arrCustomFormat{ID: f.ID, Name: f.Name, Specifications: []any{}})
	}
	if p.Best {
		out.ReleaseWeight = 1
	}
	if p.FormatScore != nil {
		out.CustomFormatScore = *p.FormatScore
	}
	if p.Protocol == "torrent" {
		seeders, leechers := p.Seeders, p.Peers
		out.Seeders, out.Leechers = &seeders, &leechers
	}
	if app == arrRadarr {
		out.IndexerFlags = []string{}
		out.MovieTitles = []string{}
		out.Edition = rel.Edition
		return out
	}
	out.IndexerFlags = 0
	full := rel.Season > 0 && len(rel.Episodes) == 0
	special := rel.Special
	season := rel.Season
	out.FullSeason, out.SeasonNumber, out.Special = &full, &season, &special
	out.MappedSeasonNumber = &season
	out.EpisodeNumbers = append([]int{}, rel.Episodes...)
	out.MappedEpisodeNumbers = append([]int{}, rel.Episodes...)
	out.AbsoluteEpisodeNums = []int{}
	out.ReleaseType = "unknown"
	return out
}

// sortArrReleases puts the best release first, like the web app does: the one
// an automatic search would take, then the ones nothing speaks against, then by
// quality, custom format score, seeders and size.
func sortArrReleases(list []arrRelease) {
	seeders := func(r arrRelease) int {
		if r.Seeders == nil {
			return 0
		}
		return *r.Seeders
	}
	sort.SliceStable(list, func(i, j int) bool {
		a, b := list[i], list[j]
		switch {
		case a.ReleaseWeight != b.ReleaseWeight:
			return a.ReleaseWeight > b.ReleaseWeight
		case a.Approved != b.Approved:
			return a.Approved
		case a.QualityWeight != b.QualityWeight:
			return a.QualityWeight > b.QualityWeight
		case a.CustomFormatScore != b.CustomFormatScore:
			return a.CustomFormatScore > b.CustomFormatScore
		case seeders(a) != seeders(b):
			return seeders(a) > seeders(b)
		}
		return a.Size > b.Size
	})
}

// indexerIDsByName maps the indexers' names to their ids, for the releases.
func (s *Server) indexerIDsByName() map[string]int64 {
	out := map[string]int64{}
	if insts, err := s.searchableIndexers(); err == nil {
		for _, in := range insts {
			out[in.Name] = in.ID
		}
	}
	return out
}

// arrQueryValue reads a query value whatever the casing of its name is.
func arrQueryValue(r *http.Request, key string) string {
	for k, v := range r.URL.Query() {
		if strings.EqualFold(k, key) && len(v) > 0 {
			return v[0]
		}
	}
	return ""
}

func arrQueryInt(r *http.Request, key string) (int64, bool) {
	v := strings.TrimSpace(arrQueryValue(r, key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return n, err == nil
}

// runInternalSearch calls one of the web app's search handlers and returns
// its results.
func (s *Server) runInternalSearch(r *http.Request, h http.HandlerFunc, id int64, query url.Values) ([]searchResultPayload, int, string) {
	r2 := r.Clone(r.Context())
	u := *r.URL
	u.RawQuery = query.Encode()
	r2.URL = &u
	r2.Method = http.MethodGet
	r2.SetPathValue("id", strconv.FormatInt(id, 10))
	rec := httptest.NewRecorder()
	h(rec, r2)
	if rec.Code != http.StatusOK {
		var e struct {
			Error   string `json:"error"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		msg := e.Error
		if msg == "" {
			msg = e.Message
		}
		return nil, rec.Code, msg
	}
	var out []searchResultPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		return nil, http.StatusInternalServerError, "The search answer couldn't be read."
	}
	return out, http.StatusOK, ""
}

// arrListReleases answers GET /release for a movie (Radarr) or an episode or
// season (Sonarr). Without a movie or show named, the list is empty, like in
// the real apps.
func (s *Server) arrListReleases(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var (
			results []searchResultPayload
			code    int
			msg     string
			offer   arrOffer
		)
		switch app {
		case arrRadarr:
			id, ok := arrQueryInt(r, "movieId")
			if !ok || id <= 0 {
				writeJSON(w, http.StatusOK, []arrRelease{})
				return
			}
			results, code, msg = s.runInternalSearch(r, s.handleMovieSearch, id, url.Values{})
			offer = arrOffer{movieID: id}
		default:
			q := url.Values{}
			var id int64
			if ep, ok := arrQueryInt(r, "episodeId"); ok && ep > 0 {
				e, err := s.MovieRepo.GetEpisodeByID(ep)
				if err != nil {
					arrNotFound(w)
					return
				}
				id = e.SeriesID
				q.Set("season", strconv.Itoa(e.Season))
				q.Set("episode", strconv.Itoa(e.Episode))
				offer = arrOffer{seriesID: id, season: e.Season, episode: e.Episode}
			} else if sid, ok := arrQueryInt(r, "seriesId"); ok && sid > 0 {
				season, hasSeason := arrQueryInt(r, "seasonNumber")
				if !hasSeason || season <= 0 || season > 10000 { // a whole show or the specials: searched one episode at a time
					writeJSON(w, http.StatusOK, []arrRelease{})
					return
				}
				id = sid
				q.Set("season", strconv.FormatInt(season, 10))
				offer = arrOffer{seriesID: id, season: int(season)}
			} else {
				writeJSON(w, http.StatusOK, []arrRelease{})
				return
			}
			results, code, msg = s.runInternalSearch(r, s.handleSeriesSearch, id, q)
		}
		switch {
		case code == http.StatusNotFound:
			arrNotFound(w)
			return
		case code != http.StatusOK:
			if code < 400 || code > 599 {
				code = http.StatusInternalServerError
			}
			writeJSON(w, code, map[string]string{"message": msg})
			return
		}
		ids := s.indexerIDsByName()
		book := s.arrOfferBook()
		out := make([]arrRelease, 0, len(results))
		for _, p := range results {
			out = append(out, s.arrReleaseOf(app, p, ids))
			o := offer
			o.title, o.size, o.protocol = p.Title, p.SizeBytes, p.Protocol
			book.put(p.DownloadURL, o)
		}
		sortArrReleases(out)
		writeJSON(w, http.StatusOK, out)
	}
}

// arrGrabRelease answers POST /release: the app names a release it listed
// (by guid) and Mediarium downloads it. Sonarr and Radarr send the guid and
// the indexer; the title, size and protocol the app sends along are used when
// the listing is no longer remembered.
func (s *Server) arrGrabRelease(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			GUID        string  `json:"guid"`
			Title       string  `json:"title"`
			Size        int64   `json:"size"`
			Protocol    string  `json:"protocol"`
			DownloadURL string  `json:"downloadUrl"`
			MovieID     int64   `json:"movieId"`
			SeriesID    int64   `json:"seriesId"`
			SeasonNum   *int    `json:"seasonNumber"`
			EpisodeIDs  []int64 `json:"episodeIds"`
			EpisodeID   int64   `json:"episodeId"`
		}
		if err := decodeJSON(r, &req); err != nil || (req.GUID == "" && req.DownloadURL == "") {
			arrBadRequest(w, "Guid", "Send the release's guid, as it was listed.")
			return
		}
		guid := req.GUID
		if guid == "" {
			guid = req.DownloadURL
		}
		offer, known := s.arrOfferBook().get(guid)
		if !known {
			offer = arrOffer{title: req.Title, size: req.Size, protocol: req.Protocol}
		}
		if offer.title == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": "Couldn't find the requested release in the search results. Search again and pick from the new list."})
			return
		}
		link := guid
		if !s.checkGrabURL(w, r, &link) {
			return
		}
		gr := grabRequest{ReleaseTitle: offer.title, DownloadURL: link, SizeBytes: offer.size, Protocol: offer.protocol}
		var queueID int64
		var err error
		switch app {
		case arrRadarr:
			id := offer.movieID
			if id == 0 {
				id = req.MovieID
			}
			movie, gerr := s.MovieRepo.Get(id)
			if gerr != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"message": "That movie isn't in your library."})
				return
			}
			queueID, err = s.grabRelease(movie, gr.ReleaseTitle, gr.DownloadURL, gr.SizeBytes, gr.protocol())
		default:
			seriesID, season, episode := offer.seriesID, offer.season, offer.episode
			if seriesID == 0 {
				seriesID = req.SeriesID
				if req.SeasonNum != nil {
					season = *req.SeasonNum
				}
				ep := req.EpisodeID
				if ep == 0 && len(req.EpisodeIDs) > 0 {
					ep = req.EpisodeIDs[0]
				}
				if e, eerr := s.MovieRepo.GetEpisodeByID(ep); eerr == nil {
					seriesID, season, episode = e.SeriesID, e.Season, e.Episode
				}
			}
			series, gerr := s.MovieRepo.GetSeries(seriesID)
			if gerr != nil {
				writeJSON(w, http.StatusNotFound, map[string]string{"message": "That show isn't in your library."})
				return
			}
			queueID, err = s.grabTVRelease(series, season, episode, gr.ReleaseTitle, gr.DownloadURL, gr.SizeBytes, gr.protocol())
		}
		if err != nil {
			code := http.StatusInternalServerError
			if isAlreadyDownloading(err) {
				code = http.StatusConflict
			}
			writeJSON(w, code, map[string]string{"message": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"guid": guid, "title": offer.title, "approved": true, "downloadAllowed": true, "queueId": queueID})
	}
}

// arrIndexers lists the indexers in the shape Radarr and Sonarr use, so an
// app's list of indexers (and which one a release came from) isn't empty.
func (s *Server) arrIndexers(w http.ResponseWriter, r *http.Request) {
	all, err := s.IndexerRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]map[string]any, 0, len(all))
	for _, in := range all {
		proto := "usenet"
		impl := "Newznab"
		if in.Protocol == "torrent" {
			proto, impl = "torrent", "Torznab"
		}
		out = append(out, map[string]any{
			"id": in.ID, "name": in.Name, "enableRss": in.Enabled, "enableAutomaticSearch": in.Enabled,
			"enableInteractiveSearch": in.Enabled, "supportsRss": true, "supportsSearch": true, "protocol": proto,
			"priority": 25, "downloadClientId": 0, "fields": []any{}, "implementationName": impl,
			"implementation": impl, "configContract": impl + "Settings", "tags": []int{},
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// arrDownloadClients lists a single stand-in for the download client: the
// downloader is built in.
func arrDownloadClients(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, []map[string]any{{
		"id": 1, "name": "Mediarium", "enable": true, "protocol": "usenet", "priority": 1,
		"removeCompletedDownloads": true, "removeFailedDownloads": true, "fields": []any{},
		"implementationName": "Mediarium", "implementation": "Mediarium", "configContract": "MediariumSettings", "tags": []int{},
	}})
}
