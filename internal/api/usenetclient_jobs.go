package api

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/rdborg/mediarium/internal/queue"
	"github.com/rdborg/mediarium/internal/usenetclient"
)

// What the Activity page's "Download clients" tab shows for the Usenet
// downloader the person set up (SABnzbd, NZBGet): its jobs, with pause, resume
// and remove next to each. Administrators only.

// usenetHistoryShown is how many finished jobs are listed when everything in
// the downloader is shown (its history can be very long).
const usenetHistoryShown = 20

type usenetJob struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Stage       string  `json:"stage"` // downloading, queued, paused, processing, done, problem
	StageText   string  `json:"stageText"`
	Progress    float64 `json:"progress"` // 0 to 100
	SizeBytes   int64   `json:"sizeBytes"`
	DownSpeed   int64   `json:"downSpeed"`
	EtaSeconds  int64   `json:"etaSeconds"` // 0 = unknown
	Category    string  `json:"category,omitempty"`
	Mine        bool    `json:"mine"` // sent by Mediarium
	QueueID     int64   `json:"queueId,omitempty"`
	CanPause    bool    `json:"canPause"`
	CanResume   bool    `json:"canResume"`
	AddedAt     string  `json:"addedAt,omitempty"`
	FailMessage string  `json:"failMessage,omitempty"`
}

type usenetJobsAnswer struct {
	Configured bool        `json:"configured"`
	Active     bool        `json:"active"` // Usenet releases go to it
	Kind       string      `json:"kind,omitempty"`
	Label      string      `json:"label,omitempty"`
	Category   string      `json:"category,omitempty"`
	Error      string      `json:"error,omitempty"`
	Jobs       []usenetJob `json:"jobs"`
}

// usenetStage puts a job's state into the few words the page shows.
func usenetStage(state usenetclient.State) string {
	switch state {
	case usenetclient.StateDownloading:
		return "downloading"
	case usenetclient.StatePaused:
		return "paused"
	case usenetclient.StateProcessing:
		return "processing"
	case usenetclient.StateCompleted:
		return "done"
	case usenetclient.StateFailed:
		return "problem"
	}
	return "queued"
}

func toUsenetJob(j usenetclient.Job, categories []string, queueIDs map[string]int64) usenetJob {
	stage := usenetStage(j.State)
	out := usenetJob{
		ID: j.ID, Name: j.Name, Stage: stage, StageText: j.StateText, Progress: j.Progress, SizeBytes: j.SizeBytes,
		DownSpeed: j.Speed, EtaSeconds: j.ETASeconds, Category: j.Category, QueueID: queueIDs[j.ID], FailMessage: j.FailMessage,
		CanPause:  stage == "downloading" || stage == "queued",
		CanResume: stage == "paused",
	}
	out.Mine = out.QueueID > 0 || (j.Category != "" && anyEqualFold(categories, j.Category))
	if !j.AddedAt.IsZero() {
		out.AddedAt = j.AddedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func anyEqualFold(list []string, v string) bool {
	for _, c := range list {
		if strings.EqualFold(c, v) {
			return true
		}
	}
	return false
}

// usenetQueueIDs finds which downloads of Mediarium are waiting on which job:
// the job's id is kept in the download's own folder.
func (s *Server) usenetQueueIDs() map[string]int64 {
	out := map[string]int64{}
	items, err := s.QueueRepo.ListUnfinished()
	if err != nil {
		return out
	}
	for _, it := range items {
		if it.Protocol == queue.ProtocolTorrent {
			continue
		}
		if dir, ok := s.existingWorkDir(it.ID); ok {
			if id := readUsenetJob(dir); id != "" {
				out[id] = it.ID
			}
		}
	}
	return out
}

// handleUsenetJobs is GET /api/usenet-client/jobs?scope=mine|all: the jobs in
// the Usenet downloader, only Mediarium's own or everything in it. Always
// answers 200: a downloader that can't be reached says so in "error" so the
// page can show it.
func (s *Server) handleUsenetJobs(w http.ResponseWriter, r *http.Request) {
	cfg := s.usenetClientConfig()
	out := usenetJobsAnswer{Jobs: []usenetJob{}}
	if !s.usenetClientSet() {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.Configured, out.Active, out.Kind, out.Label = true, cfg.Use, cfg.Kind, usenetclient.Label(cfg.Kind)
	out.Category = strings.Join(cfg.allCategories(), ", ")
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	list, err := newUsenetClient(cfg).List(ctx)
	if r.Context().Err() == nil {
		s.noteClientCheck(watchUsenetClient, out.Label, err)
	}
	if err != nil {
		out.Error = clientErrorText(out.Label, usenetClientError(cfg.Kind, err))
		writeJSON(w, http.StatusOK, out)
		return
	}
	all := r.URL.Query().Get("scope") == "all"
	cats := cfg.allCategories()
	ids := s.usenetQueueIDs()
	var running, finished []usenetJob
	for _, j := range list {
		u := toUsenetJob(j, cats, ids)
		if !all && !u.Mine {
			continue
		}
		switch u.Stage {
		case "done", "problem":
			// A finished job is only news while Mediarium is still importing it
			// (or when everything is shown); the history is long.
			if u.QueueID > 0 || all {
				finished = append(finished, u)
			}
		default:
			running = append(running, u)
		}
	}
	sort.SliceStable(finished, func(i, j int) bool { return finished[i].AddedAt > finished[j].AddedAt })
	if all && len(finished) > usenetHistoryShown {
		finished = finished[:usenetHistoryShown]
	}
	sort.SliceStable(running, func(i, j int) bool {
		a, b := running[i], running[j]
		if (a.Stage == "downloading") != (b.Stage == "downloading") {
			return a.Stage == "downloading"
		}
		return false
	})
	out.Jobs = append(append(out.Jobs, running...), finished...)
	writeJSON(w, http.StatusOK, out)
}

var usenetJobIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

// handleUsenetJobAction is POST /api/usenet-client/jobs/{id}/{action} with
// action pause, resume, remove (keep the files) or remove-files.
func (s *Server) handleUsenetJobAction(w http.ResponseWriter, r *http.Request) {
	id, action := r.PathValue("id"), r.PathValue("action")
	if !usenetJobIDPattern.MatchString(id) {
		writeError(w, http.StatusBadRequest, "That isn't a download.")
		return
	}
	if !s.usenetClientSet() {
		writeError(w, http.StatusConflict, "No Usenet downloader is set up. Add one under Settings > Downloading > Usenet.")
		return
	}
	cfg := s.usenetClientConfig()
	label := usenetclient.Label(cfg.Kind)
	c := newUsenetClient(cfg)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var err error
	var done string
	switch action {
	case "pause":
		err, done = c.Pause(ctx, id), "paused"
	case "resume":
		err, done = c.Resume(ctx, id), "started again"
	case "remove", "remove-files":
		name := id
		if j, ok, e := c.Get(ctx, id); e == nil && ok {
			name = j.Name
		}
		err, done = c.Delete(ctx, id, action == "remove-files"), "removed"
		if err == nil {
			_ = s.QueueRepo.LogActivity(0, "removed", fmt.Sprintf("%s was removed from %s by hand", name, label))
		}
	default:
		writeError(w, http.StatusBadRequest, "That isn't something a download can do here.")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, clientErrorText(label, usenetClientError(cfg.Kind, err)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"done": done})
}
