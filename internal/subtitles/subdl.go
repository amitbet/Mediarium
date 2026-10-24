package subtitles

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/netguard"
)

// SubDL (subdl.com) is the second place subtitles come from, next to
// OpenSubtitles. It needs a free API key (about 2,000 searches a day) and its
// downloads are zip files, with a limit of a few hundred a day per address.
// It has no file hash, so its subtitles are picked by release name only.

const (
	SourceOpenSubtitles = "" // the zero value: results without a source are OpenSubtitles'
	SourceSubDL         = "subdl"

	subdlAPI      = "https://api.subdl.com/api/v1"
	subdlDownload = "https://dl.subdl.com"
	maxSubDLZip   = 20 << 20
)

// ErrSubDLKey means SubDL turned the API key down.
var ErrSubDLKey = errors.New("SubDL rejected the API key")

// subdlLink is the form of a SubDL download path: "/subtitle/3197651-3213944.zip",
// or one file of a pack, "/subtitle/<id>/<file id>".
var subdlLink = regexp.MustCompile(`^/subtitle/[A-Za-z0-9._~%+=-]+(?:/[A-Za-z0-9._~%+=-]+)*$`)

// ValidSubDLLink reports whether link is a SubDL download path: a plain path
// under /subtitle/ on SubDL's download host, with no query, no spaces and no
// ".." part. The host is always put in front by us, never taken from SubDL.
func ValidSubDLLink(link string) bool {
	if !subdlLink.MatchString(link) {
		return false
	}
	for _, part := range strings.Split(link, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	return true
}

type SubDL struct {
	apiKey     string
	baseURL    string
	dlURL      string
	httpClient *http.Client
}

func NewSubDL(apiKey string) *SubDL {
	return &SubDL{apiKey: strings.TrimSpace(apiKey), baseURL: subdlAPI, dlURL: subdlDownload, httpClient: netguard.Client(20 * time.Second)}
}

// NewSubDLWithBaseURL points the client at a test server.
func NewSubDLWithBaseURL(apiKey, baseURL, dlURL string) *SubDL {
	c := NewSubDL(apiKey)
	c.baseURL, c.dlURL = baseURL, dlURL
	return c
}

func (c *SubDL) HasAPIKey() bool { return c != nil && c.apiKey != "" }

// subdlLanguage is SubDL's code for one of Mediarium's.
func subdlLanguage(code string) string {
	switch strings.ToLower(code) {
	case "pt-br":
		return "BR_PT"
	case "pt-pt":
		return "PT"
	case "zh-cn", "zh-tw":
		return "ZH"
	}
	base, _, _ := strings.Cut(code, "-")
	return strings.ToUpper(base)
}

// flexInt reads a number SubDL may send as a number, a string or null.
type flexInt int

func (n *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		*n = 0
		return nil
	}
	*n = flexInt(v)
	return nil
}

type subdlResponse struct {
	Status    bool   `json:"status"`
	Error     string `json:"error"`
	Message   string `json:"message"`
	Subtitles []struct {
		ReleaseName string   `json:"release_name"`
		Name        string   `json:"name"`
		URL         string   `json:"url"`
		Lang        string   `json:"lang"`     // "english"
		Language    string   `json:"language"` // "EN"
		Season      flexInt  `json:"season"`
		Episode     flexInt  `json:"episode"`
		EpisodeFrom flexInt  `json:"episode_from"`
		EpisodeEnd  flexInt  `json:"episode_end"`
		FullSeason  bool     `json:"full_season"`
		Releases    []string `json:"releases"`
		HI          bool     `json:"hi"`      // asked for with hi=1
		Comment     string   `json:"comment"` // asked for with comment=1
	} `json:"subtitles"`
}

func (c *SubDL) get(ctx context.Context, q url.Values) (subdlResponse, error) {
	var out subdlResponse
	if !c.HasAPIKey() {
		return out, ErrNoAPIKey
	}
	q.Set("api_key", c.apiKey)
	q.Set("client", "custom_integration")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/subtitles?"+q.Encode(), nil)
	if err != nil {
		return out, netguard.CleanError(err)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return out, fmt.Errorf("search SubDL: %w", netguard.CleanError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponse))
	if err != nil {
		return out, fmt.Errorf("search SubDL: %w", err)
	}
	_ = json.Unmarshal(body, &out)
	msg := strings.ToLower(out.Error + " " + out.Message)
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden ||
		strings.Contains(msg, "api key") || strings.Contains(msg, "api_key") || strings.Contains(msg, "not_authorized") || strings.Contains(msg, "not authorized"):
		return out, ErrSubDLKey
	case resp.StatusCode == http.StatusTooManyRequests:
		return out, errors.New("SubDL says too many searches today; it tries again later")
	case resp.StatusCode != http.StatusOK && !out.Status:
		return out, fmt.Errorf("search SubDL: HTTP %d", resp.StatusCode)
	}
	return out, nil
}

// subdlLookup is one way of asking SubDL about a title.
type subdlLookup struct {
	kind, value string // "tmdb", "imdb" or "title"
	v           url.Values
}

// subdlLookups lists how q can be asked, most exact first: the TMDB id, the
// IMDb id, then the title (with the year for a film). Each is its own call, so
// SubDL never has to match several of them at once.
func subdlLookups(q Query) []subdlLookup {
	episode := q.ParentTMDBID > 0
	var out []subdlLookup
	switch {
	case episode:
		out = append(out, subdlLookup{"tmdb", strconv.Itoa(q.ParentTMDBID), url.Values{"tmdb_id": {strconv.Itoa(q.ParentTMDBID)}}})
	case q.TMDBID > 0:
		out = append(out, subdlLookup{"tmdb", strconv.Itoa(q.TMDBID), url.Values{"tmdb_id": {strconv.Itoa(q.TMDBID)}}})
	}
	if q.IMDBID != "" {
		out = append(out, subdlLookup{"imdb", q.IMDBID, url.Values{"imdb_id": {q.IMDBID}}})
	}
	name, year := q.Title, q.Year
	if name == "" {
		name, year = q.Text, 0
	}
	if name != "" {
		v := url.Values{"film_name": {name}}
		value := name
		if year > 0 && !episode {
			v.Set("year", strconv.Itoa(year))
			value += " (" + strconv.Itoa(year) + ")"
		}
		out = append(out, subdlLookup{"title", value, v})
	}
	return out
}

// Find searches SubDL for q. SubDL doesn't have every title under its TMDB id,
// so the IMDb id and then the title are tried too: a lookup that gives nothing
// usable, for whatever reason, is followed by the next. When none does, every
// lookup is tried once more with as few filters as possible (no language, no
// extras), and the language is judged here. Each lookup reports what it asked
// and what came back (see Diagnostic). A title SubDL doesn't know is no
// results, not an error.
func (c *SubDL) Find(ctx context.Context, q Query) ([]Result, error) {
	episode := q.ParentTMDBID > 0
	kind := "movie"
	if episode {
		kind = "tv"
	}
	lookups := subdlLookups(q)
	for _, bare := range []bool{false, true} {
		for _, l := range lookups {
			v := url.Values{}
			for k, vals := range l.v {
				v[k] = vals
			}
			v.Set("subs_per_page", strconv.Itoa(subdlPerPage))
			v.Set("type", kind)
			if episode {
				v.Set("season_number", strconv.Itoa(q.Season))
				v.Set("episode_number", strconv.Itoa(q.Episode))
			}
			if !bare {
				// hi=1 and comment=1 ask SubDL to include the hearing-impaired flag and the
				// uploader's comment, which is where a "forced" track is usually said so.
				v.Set("releases", "1")
				v.Set("hi", "1")
				v.Set("comment", "1")
				if q.Language != "" {
					v.Set("languages", subdlLanguage(q.Language))
				}
			} else {
				v.Del("year")
			}
			res, st, err := c.findPages(ctx, v, q, bare)
			Report(ctx, st.line(l, bare, len(res), err))
			if err != nil {
				return nil, err
			}
			if len(res) > 0 {
				return res, nil
			}
		}
	}
	return nil, nil
}

// subdlPerPage is the most SubDL gives per page.
const subdlPerPage = 30

// subdlStats is what one lookup saw, for its diagnostic line.
type subdlStats struct {
	status     bool // SubDL's own "status"
	message    string
	found      int // subtitles in the answers
	badLink    int
	badSample  string // the first link that was turned down, shortened, for the diagnostic
	wrongEp    int
	wrongLang  int
	langsFound map[string]bool
}

func (st subdlStats) line(l subdlLookup, bare bool, kept int, err error) Diagnostic {
	d := Diagnostic{Source: "SubDL", Lookup: l.kind, Value: l.value, Bare: bare, Count: kept, Found: st.found}
	switch {
	case err != nil:
		d.Note, d.Detail = "error", err.Error()
	case kept > 0:
	case st.found == 0 && !st.status:
		d.Note, d.Detail = "no-match", st.message
	case st.found == 0:
	case st.wrongLang > 0 && st.wrongLang >= st.badLink && st.wrongLang >= st.wrongEp:
		d.Note, d.Detail = "wrong-language", joinLanguages(st.langsFound)
	case st.badLink > 0 && st.badLink >= st.wrongEp:
		d.Note, d.Detail = "bad-link", st.badSample
	case st.wrongEp > 0:
		d.Note = "wrong-episode"
	}
	if kept == 0 {
		// What a search that found nothing asked and heard, for the log.
		log.Printf("subtitles: SubDL lookup %s %q (fewer filters: %t): %d found, none kept, note %q %s", l.kind, l.value, bare, st.found, d.Note, redactSecrets(d.Detail))
	}
	return d
}

// findPages reads up to maxSearchPages pages of one search, going on while a
// page comes back full.
func (c *SubDL) findPages(ctx context.Context, v url.Values, q Query, bare bool) ([]Result, subdlStats, error) {
	var out []Result
	st := subdlStats{langsFound: map[string]bool{}}
	for page := 1; page <= maxSearchPages; page++ {
		pv := url.Values{}
		for k, vals := range v {
			pv[k] = vals
		}
		if page > 1 {
			pv.Set("page", strconv.Itoa(page))
		}
		resp, err := c.get(ctx, pv)
		if err != nil {
			if page > 1 {
				break // the first page worked: use it
			}
			return nil, st, err
		}
		if page == 1 {
			st.status = resp.Status
			st.message = strings.TrimSpace(resp.Error + " " + resp.Message)
		}
		if !resp.Status {
			break // "can't find movie or tv" and the like
		}
		out = append(out, c.results(resp, q, bare, &st)...)
		if len(resp.Subtitles) < subdlPerPage {
			break
		}
	}
	return out, st, nil
}

// sameLanguage reports whether a language as SubDL writes it ("EN", "en",
// "english", "BR_PT") is the wanted one. A spelling that isn't recognised
// counts as a match: it is better to show a doubtful result than to hide a
// good one. Regions (pt-BR, pt-PT) are not told apart here.
func sameLanguage(wanted, got string) bool {
	got = strings.TrimSpace(got)
	if got == "" || wanted == "" {
		return true
	}
	g := languageBase(got)
	return g == "" || languageBase(wanted) == g
}

// languageBase is the two-letter code of a language in any spelling, or empty
// when it isn't known.
func languageBase(s string) string {
	k := strings.ReplaceAll(fold(strings.TrimSpace(s)), "_", "-")
	if k == "br-pt" || k == "pt-br" {
		return "pt"
	}
	if code, ok := languageByName[k]; ok {
		base, _, _ := strings.Cut(code, "-")
		return base
	}
	if base, _, ok := strings.Cut(k, "-"); ok {
		if code, ok := languageByName[base]; ok {
			return code
		}
	}
	return ""
}

// downloadPath makes a SubDL download path of what SubDL gave as "url": the
// path itself, or the same with the download host in front.
func downloadPath(u string) string {
	u = strings.TrimSpace(u)
	for _, prefix := range []string{subdlDownload, "https://subdl.com", "http://dl.subdl.com", "//dl.subdl.com"} {
		u = strings.TrimPrefix(u, prefix)
	}
	// A query or a fragment is not part of the path we ask for: the download
	// host serves the file from the path alone.
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		u = u[:i]
	}
	return u
}

// results turns one answer into results, leaving out what isn't for q and
// counting why in st. A subtitle whose release name doesn't match the video is
// kept: the list is ranked by fit, not cut. With bare set the answer holds
// every language, so one that doesn't say its language is left out.
func (c *SubDL) results(resp subdlResponse, q Query, bare bool, st *subdlStats) []Result {
	var out []Result
	for _, s := range resp.Subtitles {
		st.found++
		link := downloadPath(s.URL)
		if !ValidSubDLLink(link) {
			st.badLink++
			if st.badSample == "" {
				st.badSample = shortSample(link)
			}
			continue
		}
		spelled := s.Language
		if spelled == "" {
			spelled = s.Lang
		}
		if spelled != "" {
			st.langsFound[spelled] = true
		}
		if q.ParentTMDBID > 0 {
			// Only subtitles for this one episode: a whole-season pack or a
			// different episode isn't what was asked for.
			switch {
			case s.FullSeason, s.EpisodeFrom > 0 && s.EpisodeEnd > s.EpisodeFrom:
				st.wrongEp++
				continue
			case s.Season > 0 && int(s.Season) != q.Season, s.Episode > 0 && int(s.Episode) != q.Episode:
				st.wrongEp++
				continue
			}
		}
		if !sameLanguage(q.Language, spelled) || (bare && q.Language != "" && spelled == "") {
			st.wrongLang++
			continue
		}
		release := s.ReleaseName
		if release == "" {
			release = strings.TrimSuffix(s.Name, path.Ext(s.Name))
		}
		nameHI, forced := ClassifyName(s.Name + " " + s.ReleaseName + " " + s.Comment)
		out = append(out, Result{
			Language: q.Language, Release: release, Source: SourceSubDL, Link: link, Releases: s.Releases,
			HearingImpaired: s.HI || nameHI, ForeignPartsOnly: forced && !s.HI,
		})
	}
	return out
}

// Test checks the key with a search for a film everyone has.
func (c *SubDL) Test(ctx context.Context) (string, error) {
	if _, err := c.get(ctx, url.Values{"tmdb_id": {"603"}, "type": {"movie"}, "languages": {"EN"}, "subs_per_page": {"1"}}); err != nil {
		return "", err
	}
	return "SubDL accepted the key.", nil
}

// Download fetches a subtitle and returns the SRT inside it. For an episode
// (season > 0) in a zip holding several files, the one named for that
// episode is taken; otherwise the biggest.
func (c *SubDL) Download(ctx context.Context, link string, season, episode int) ([]byte, error) {
	if !ValidSubDLLink(link) {
		return nil, errors.New("download from SubDL: not a SubDL download link")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.dlURL+link, nil)
	if err != nil {
		return nil, netguard.CleanError(err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download from SubDL: %w", netguard.CleanError(err))
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, errors.New("download from SubDL: today's download limit is used up")
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("download from SubDL: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSubDLZip+1))
	if err != nil {
		return nil, fmt.Errorf("download from SubDL: %w", err)
	}
	if len(data) > maxSubDLZip {
		return nil, errors.New("download from SubDL: the file is too big to be a subtitle")
	}
	if !bytes.HasPrefix(data, []byte("PK")) {
		// One file of a pack comes as it is.
		if len(data) > MaxSubtitleBytes || !looksLikeSRT(data) {
			return nil, errors.New("download from SubDL: that isn't an SRT subtitle")
		}
		return data, nil
	}
	return srtFromZip(data, season, episode)
}

// srtFromZip picks the SRT out of a downloaded zip.
func srtFromZip(data []byte, season, episode int) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("download from SubDL: the zip can't be opened: %w", err)
	}
	var best *zip.File
	bestMatch := false
	for _, f := range zr.File {
		name := f.Name
		if f.FileInfo().IsDir() || !strings.EqualFold(path.Ext(name), ".srt") || isSamplePath(name) {
			continue
		}
		if f.UncompressedSize64 > MaxSubtitleBytes {
			continue
		}
		match := false
		if season > 0 {
			if s, eps := episodeOf(name); s == season && containsInt(eps, episode) {
				match = true
			}
		}
		switch {
		case best == nil,
			match && !bestMatch,
			match == bestMatch && f.UncompressedSize64 > best.UncompressedSize64:
			best, bestMatch = f, match
		}
	}
	if best == nil {
		return nil, errors.New("download from SubDL: no SRT subtitle in the zip")
	}
	rc, err := best.Open()
	if err != nil {
		return nil, fmt.Errorf("download from SubDL: %w", err)
	}
	defer rc.Close()
	out, err := io.ReadAll(io.LimitReader(rc, MaxSubtitleBytes+1))
	if err != nil {
		return nil, fmt.Errorf("download from SubDL: %w", err)
	}
	if len(out) > MaxSubtitleBytes {
		return nil, errors.New("download from SubDL: the subtitle is too big")
	}
	return out, nil
}

func containsInt(list []int, v int) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// looksLikeSRT checks for the "1 / 00:00:01,000 --> ..." shape near the start.
func looksLikeSRT(b []byte) bool {
	head := b[:min(len(b), 4096)]
	return bytes.Contains(head, []byte("-->"))
}

// shortSample shortens a rejected download link for the diagnostic: the first
// part of it, with a query string (which could hold a key) cut off.
func shortSample(link string) string {
	if link == "" {
		return "nothing, the link was empty"
	}
	if i := strings.IndexAny(link, "?#"); i >= 0 {
		link = link[:i] + " (and more after it)"
	}
	if len(link) > 80 {
		link = link[:80] + "..."
	}
	return link
}
