package api

import (
	"net/http"
	"sort"
	"strings"

	"github.com/rdborg/mediarium/internal/library"
)

// What the Radarr and Sonarr apps ask for next to the lists they show: the
// "cutoff unmet" list, saved filters, the custom formats and the settings
// pages that only need a valid, mostly empty answer.

// arrWantedPage answers a paged list in the v3 envelope.
func arrWantedPage(w http.ResponseWriter, r *http.Request, records []any, defaultKey string) {
	page, size := paging(r, 10)
	desc := !strings.EqualFold(r.URL.Query().Get("sortDirection"), "ascending")
	key := r.URL.Query().Get("sortKey")
	if key == "" {
		key = defaultKey
	}
	if records == nil {
		records = []any{}
	}
	from, to := pageOf(len(records), page, size)
	dir := "ascending"
	if desc {
		dir = "descending"
	}
	writeJSON(w, http.StatusOK, arrPage{Page: page, PageSize: size, SortKey: key, SortDirection: dir, TotalRecords: len(records), Records: records[from:to]})
}

// sortArrMovies orders a list of movies by the sortKey an app sends (the
// title when it is missing or unknown) and sortDirection (ascending when it
// is missing, as in the real app).
func sortArrMovies(list []arrMovie, sortKey, direction string) {
	key := strings.ToLower(sortKey)
	desc := strings.HasPrefix(strings.ToLower(direction), "desc")
	cmp := func(a, b arrMovie) int {
		switch {
		case strings.Contains(key, "year"):
			if a.Year != b.Year {
				return a.Year - b.Year
			}
		case strings.Contains(key, "added"):
			if c := strings.Compare(a.Added, b.Added); c != 0 {
				return c
			}
		case strings.Contains(key, "release"), strings.Contains(key, "cinema"):
			if c := strings.Compare(a.InCinemas, b.InCinemas); c != 0 {
				return c
			}
		}
		return strings.Compare(a.SortTitle, b.SortTitle)
	}
	sort.SliceStable(list, func(i, j int) bool {
		if desc {
			return cmp(list[i], list[j]) > 0
		}
		return cmp(list[i], list[j]) < 0
	})
}

// arrWantedCutoff answers GET /wanted/cutoff: the monitored titles whose file
// is below what their profile is after, the same ones an upgrade search goes
// looking for. Radarr lists movies, Sonarr lists episodes.
func (s *Server) arrWantedCutoff(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var records []any
		switch app {
		case arrRadarr:
			movies, err := s.MovieRepo.List()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			profiles, err := s.loadProfiles()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			c := s.newArrMovieContext()
			var list []arrMovie
			for _, m := range movies {
				if !m.Monitored || m.Status != library.StatusDownloaded || m.FilePath == "" {
					continue
				}
				if !wantsUpgrade(s.profileForMovie(profiles, m), m.Quality, m.ReleaseTitle) {
					continue
				}
				list = append(list, s.toArrMovie(m, c))
			}
			sort.Slice(list, func(i, j int) bool { return list[i].SortTitle < list[j].SortTitle })
			for _, a := range list {
				records = append(records, a)
			}
		case arrSonarr:
			all, err := s.allShowEpisodes()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			profiles, err := s.loadProfiles()
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			c := s.newArrSeriesContext()
			series := map[int64]*arrSeries{}
			var list []arrEpisode
			for _, se := range all {
				if !se.series.Monitored || !se.ep.Monitored || se.ep.Status != library.StatusDownloaded {
					continue
				}
				if !wantsUpgrade(s.profileForSeries(profiles, se.series), se.ep.Quality, se.ep.ReleaseTitle) {
					continue
				}
				if series[se.series.ID] == nil {
					a := s.toArrSeries(se.series, c)
					series[se.series.ID] = &a
				}
				e := toArrEpisode(se.ep)
				e.Series = series[se.series.ID]
				list = append(list, e)
			}
			desc := !strings.EqualFold(r.URL.Query().Get("sortDirection"), "ascending")
			sort.Slice(list, func(i, j int) bool {
				if desc {
					return list[i].AirDate > list[j].AirDate
				}
				return list[i].AirDate < list[j].AirDate
			})
			for _, e := range list {
				records = append(records, e)
			}
		}
		// Lidarr: albums are not judged against a cutoff here, so the list is empty.
		arrWantedPage(w, r, records, "title")
	}
}

// arrCustomFormats lists the custom formats by name. Their conditions are not
// sent: the apps only need the names and ids to label a release.
func (s *Server) arrCustomFormats(w http.ResponseWriter, r *http.Request) {
	formats, err := s.FormatRepo.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]arrCustomFormat, 0, len(formats))
	for _, f := range formats {
		out = append(out, arrCustomFormat{ID: f.ID, Name: f.Name, Specifications: []any{}})
	}
	writeJSON(w, http.StatusOK, out)
}

// arrEmptyList answers a list Mediarium has nothing to put in.
func arrEmptyList(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, []any{}) }

// arrTagDetails answers GET /tag/detail: the tags, each with the lists of
// things that use it (none are kept apart from the titles).
func (s *Server) arrTagDetails(app arrApp) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tags, err := s.MovieRepo.ArrTags()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		out := make([]map[string]any, 0, len(tags))
		for _, t := range tags {
			d := map[string]any{
				"id": t.ID, "label": t.Label, "delayProfileIds": []int{}, "importListIds": []int{}, "notificationIds": []int{},
				"restrictionIds": []int{}, "indexerIds": []int{}, "downloadClientIds": []int{}, "autoTagIds": []int{},
			}
			switch app {
			case arrRadarr:
				d["movieIds"] = []int{}
			case arrSonarr:
				d["seriesIds"] = []int{}
			default:
				d["artistIds"] = []int{}
			}
			out = append(out, d)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// arrUIConfig answers the app's display settings.
func arrUIConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"id": 1, "firstDayOfWeek": 1, "calendarWeekColumnHeader": "ddd M/D", "movieRuntimeFormat": "hoursMinutes",
		"shortDateFormat": "MMM D YYYY", "longDateFormat": "dddd, MMMM D YYYY", "timeFormat": "HH:mm",
		"showRelativeDates": true, "enableColorImpairedMode": false, "movieInfoLanguage": 1, "uiLanguage": 1, "theme": "auto",
	})
}

// arrHostConfig answers the app's host settings. The key is never included.
func arrHostConfig(app arrApp) http.HandlerFunc {
	port := map[arrApp]int{arrRadarr: 7878, arrSonarr: 8989, arrLidarr: 8686}[app]
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"id": 1, "bindAddress": "*", "port": port, "sslPort": 0, "enableSsl": false, "launchBrowser": false,
			"authenticationMethod": "forms", "authenticationRequired": "enabled", "analyticsEnabled": false,
			"username": "", "password": "", "passwordConfirmation": "", "logLevel": "info", "consoleLogLevel": "",
			"branch": "master", "apiKey": "", "sslCertPath": "", "sslCertPassword": "", "urlBase": "",
			"instanceName": "Mediarium", "applicationUrl": "", "updateAutomatically": false, "updateMechanism": "builtIn",
			"updateScriptPath": "", "proxyEnabled": false, "proxyType": "http", "proxyHostname": "", "proxyPort": 8080,
			"proxyUsername": "", "proxyPassword": "", "proxyBypassFilter": "", "proxyBypassLocalAddresses": true,
			"certificateValidation": "enabled", "backupFolder": "Backups", "backupInterval": 7, "backupRetention": 28,
		})
	}
}

// arrLogPage answers GET /log: the app's log, which is not served here.
func arrLogPage(w http.ResponseWriter, r *http.Request) {
	arrWantedPage(w, r, nil, "time")
}

// arrPushRelease answers POST /release/push. Mediarium does not take releases
// pushed by other tools; the answer says so in the shape the apps use.
func arrPushRelease(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Title       string `json:"title"`
		DownloadURL string `json:"downloadUrl"`
		Protocol    string `json:"protocol"`
		PublishDate string `json:"publishDate"`
	}
	if err := decodeJSON(r, &req); err != nil || req.Title == "" || req.DownloadURL == "" {
		arrBadRequest(w, "Title", "Send the release's title and download address.")
		return
	}
	writeJSON(w, http.StatusOK, []map[string]any{{
		"guid": req.DownloadURL, "title": req.Title, "protocol": req.Protocol, "publishDate": req.PublishDate,
		"downloadUrl": req.DownloadURL, "approved": false, "temporarilyRejected": false, "rejected": true, "downloadAllowed": false,
		"rejections":    []string{"Mediarium does not take releases pushed to it. It searches the indexers itself."},
		"customFormats": []any{}, "customFormatScore": 0,
	}})
}
