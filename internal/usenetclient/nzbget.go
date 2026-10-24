package usenetclient

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// nzbget talks to NZBGet's JSON-RPC API at /jsonrpc, signed in with basic
// authentication (the ControlUsername and ControlPassword of its settings).
type nzbget struct {
	base string
	user string
	pass string
	hc   *http.Client
}

func newNZBGet(base, user, pass string, hc *http.Client) *nzbget {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &nzbget{base: baseURL(base), user: user, pass: pass, hc: hc}
}

type rpcRequest struct {
	Method string `json:"method"`
	Params []any  `json:"params"`
	ID     int    `json:"id"`
}

type rpcAnswer struct {
	Result json.RawMessage `json:"result"`
	Error  *struct {
		Name    string `json:"name"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *nzbget) call(ctx context.Context, method string, params []any, out any) error {
	if params == nil {
		params = []any{}
	}
	body, err := json.Marshal(rpcRequest{Method: method, Params: params, ID: 1})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/jsonrpc", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.user != "" || c.pass != "" {
		req.SetBasicAuth(c.user, c.pass)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return cleanURLError(err)
	}
	b, err := readBody(resp)
	if err != nil {
		return err
	}
	var a rpcAnswer
	if err := jsonUnmarshal(b, &a); err != nil {
		return err
	}
	if a.Error != nil {
		msg := a.Error.Message
		if msg == "" {
			msg = a.Error.Name
		}
		return &RefusedError{Message: msg}
	}
	if out == nil {
		return nil
	}
	if len(a.Result) == 0 {
		return ErrNotThisClient
	}
	if err := json.Unmarshal(a.Result, out); err != nil {
		return fmt.Errorf("%w (%v)", ErrNotThisClient, err)
	}
	return nil
}

func (c *nzbget) Check(ctx context.Context, categories []string) (string, error) {
	var version string
	if err := c.call(ctx, "version", nil, &version); err != nil {
		return "", err
	}
	var conf []struct{ Name, Value string }
	if err := c.call(ctx, "config", nil, &conf); err != nil {
		return version, err
	}
	var have []string
	for _, e := range conf {
		if strings.HasPrefix(e.Name, "Category") && strings.HasSuffix(e.Name, ".Name") {
			have = append(have, e.Value)
		}
	}
	for _, want := range categories {
		want = strings.TrimSpace(want)
		if want != "" && !equalFold(have, want) {
			return version, &CategoryError{Name: want}
		}
	}
	return version, nil
}

func nzbgetPriority(p Priority) int {
	switch p {
	case PriorityLow:
		return -50
	case PriorityHigh:
		return 50
	case PriorityForce:
		return 900
	}
	return 0
}

func (c *nzbget) Add(ctx context.Context, o AddOptions) (string, error) {
	content := o.URL
	if content == "" {
		content = base64.StdEncoding.EncodeToString(o.NZB)
	}
	name := safeFileName(o.Name) + ".nzb"
	// append(NZBFilename, NZBContent, Category, Priority, AddToTop, AddPaused,
	// DupeKey, DupeScore, DupeMode, PPParameters)
	params := []any{name, content, o.Category, nzbgetPriority(o.Priority), false, false, "", 0, "SCORE", []any{}}
	var id int64
	if err := c.call(ctx, "append", params, &id); err != nil {
		return "", err
	}
	if id <= 0 {
		return "", &RefusedError{Message: "it didn't accept the NZB (is it empty or broken?)"}
	}
	return strconv.FormatInt(id, 10), nil
}

type nzbgetGroup struct {
	NZBID           int64  `json:"NZBID"`
	NZBName         string `json:"NZBName"`
	Name            string `json:"Name"`
	Status          string `json:"Status"`
	Category        string `json:"Category"`
	FileSizeMB      int64  `json:"FileSizeMB"`
	RemainingSizeMB int64  `json:"RemainingSizeMB"`
	DestDir         string `json:"DestDir"`
	FinalDir        string `json:"FinalDir"`
	HistoryTime     int64  `json:"HistoryTime"`
	DeleteStatus    string `json:"DeleteStatus"`
}

func (g nzbgetGroup) name() string {
	if g.NZBName != "" {
		return g.NZBName
	}
	return g.Name
}

func nzbgetGroupState(status string) State {
	switch strings.ToUpper(status) {
	case "PAUSED":
		return StatePaused
	case "DOWNLOADING", "FETCHING":
		return StateDownloading
	case "PP_QUEUED", "LOADING_PARS", "VERIFYING_SOURCES", "REPAIRING", "VERIFYING_REPAIRED", "RENAMING", "UNPACKING", "MOVING", "EXECUTING_SCRIPT", "PP_FINISHED":
		return StateProcessing
	}
	return StateQueued
}

func (g nzbgetGroup) queueJob(rate int64) Job {
	j := Job{ID: strconv.FormatInt(g.NZBID, 10), Name: g.name(), Category: g.Category, State: nzbgetGroupState(g.Status), StateText: strings.ToLower(strings.ReplaceAll(g.Status, "_", " "))}
	j.SizeBytes, j.RemainingBytes = g.FileSizeMB<<20, g.RemainingSizeMB<<20
	if g.FileSizeMB > 0 {
		j.Progress = float64(g.FileSizeMB-g.RemainingSizeMB) / float64(g.FileSizeMB) * 100
	}
	if j.State == StateProcessing {
		j.Progress = 99
	}
	if j.State == StateDownloading && rate > 0 {
		j.Speed = rate
		j.ETASeconds = j.RemainingBytes / rate
	}
	return j
}

// nzbgetFailure puts a failed history status ("FAILURE/PAR") into words, and
// says whether the release is to blame.
func nzbgetFailure(status string) (msg string, release bool) {
	_, reason, _ := strings.Cut(strings.ToUpper(status), "/")
	switch reason {
	case "HEALTH":
		return "too many articles were missing to repair it", true
	case "PAR":
		return "repairing it with the PAR2 files failed", true
	case "UNPACK":
		return "unpacking it failed", true
	case "BAD":
		return "it was marked as bad", true
	case "PASSWORD":
		return "it is protected with a password", true
	case "DAMAGED", "REPAIRABLE":
		return "it is damaged", true
	case "FETCH":
		return "NZBGet couldn't fetch the NZB file", false
	case "MOVE":
		return "NZBGet couldn't move the finished files into place", false
	case "SCRIPT":
		return "a post-processing script of NZBGet failed", false
	case "SPACE":
		return "NZBGet ran out of disk space", false
	}
	return "NZBGet reported " + strings.ToLower(status), true
}

func (g nzbgetGroup) historyJob() Job {
	j := Job{ID: strconv.FormatInt(g.NZBID, 10), Name: g.name(), Category: g.Category, StateText: strings.ToLower(g.Status), Progress: 100, SizeBytes: g.FileSizeMB << 20}
	j.Path = g.FinalDir
	if j.Path == "" {
		j.Path = g.DestDir
	}
	if g.HistoryTime > 0 {
		j.AddedAt = time.Unix(g.HistoryTime, 0)
	}
	kind, reason, _ := strings.Cut(strings.ToUpper(g.Status), "/")
	switch {
	case kind == "SUCCESS":
		j.State = StateCompleted
	case kind == "WARNING" && reason == "SCRIPT":
		j.State = StateCompleted
	case kind == "DELETED":
		j.State, j.Removed = StateFailed, true
		j.FailMessage = "it was deleted in NZBGet"
	case kind == "FAILURE" || kind == "WARNING":
		j.State = StateFailed
		j.FailMessage, j.ReleaseFault = nzbgetFailure(g.Status)
	default:
		j.State = StateProcessing
		j.Progress = 99
	}
	return j
}

func (c *nzbget) rate(ctx context.Context) int64 {
	var st struct {
		DownloadRate int64 `json:"DownloadRate"`
	}
	if err := c.call(ctx, "status", nil, &st); err != nil {
		return 0
	}
	return st.DownloadRate
}

func (c *nzbget) queue(ctx context.Context) ([]Job, error) {
	var groups []nzbgetGroup
	if err := c.call(ctx, "listgroups", []any{0}, &groups); err != nil {
		return nil, err
	}
	rate := int64(0)
	for _, g := range groups {
		if nzbgetGroupState(g.Status) == StateDownloading {
			rate = c.rate(ctx)
			break
		}
	}
	out := make([]Job, 0, len(groups))
	for _, g := range groups {
		j := g.queueJob(rate)
		out = append(out, j)
		if j.State == StateDownloading {
			rate = 0 // the speed is the whole client's; it goes with the first job
		}
	}
	return out, nil
}

func (c *nzbget) history(ctx context.Context) ([]Job, error) {
	var items []nzbgetGroup
	if err := c.call(ctx, "history", []any{false}, &items); err != nil {
		return nil, err
	}
	out := make([]Job, 0, len(items))
	for _, g := range items {
		out = append(out, g.historyJob())
	}
	return out, nil
}

func (c *nzbget) List(ctx context.Context) ([]Job, error) {
	q, err := c.queue(ctx)
	if err != nil {
		return nil, err
	}
	h, err := c.history(ctx)
	if err != nil {
		return nil, err
	}
	return append(q, h...), nil
}

func (c *nzbget) Get(ctx context.Context, id string) (Job, bool, error) {
	all, err := c.List(ctx)
	if err != nil {
		return Job{}, false, err
	}
	for _, j := range all {
		if j.ID == id {
			return j, true, nil
		}
	}
	return Job{}, false, nil
}

func (c *nzbget) edit(ctx context.Context, command, id string) error {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a NZBGet job number", id)
	}
	var ok bool
	if err := c.call(ctx, "editqueue", []any{command, "", []int64{n}}, &ok); err != nil {
		return err
	}
	if !ok {
		return &RefusedError{Message: "it didn't find that download"}
	}
	return nil
}

func (c *nzbget) Pause(ctx context.Context, id string) error  { return c.edit(ctx, "GroupPause", id) }
func (c *nzbget) Resume(ctx context.Context, id string) error { return c.edit(ctx, "GroupResume", id) }

// Delete removes the job whether it is running or finished. NZBGet's API has
// no way to delete the files of a finished job, so deleteFiles only reaches a
// job that is still running (NZBGet cleans up what it had downloaded).
func (c *nzbget) Delete(ctx context.Context, id string, deleteFiles bool) error {
	// A running job is deleted from the queue; a finished one from the history.
	// Whichever isn't there answers false, which is fine.
	cmd := "GroupDelete"
	if deleteFiles {
		cmd = "GroupFinalDelete"
	}
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return fmt.Errorf("%q is not a NZBGet job number", id)
	}
	var okQ, okH bool
	errQ := c.call(ctx, "editqueue", []any{cmd, "", []int64{n}}, &okQ)
	errH := c.call(ctx, "editqueue", []any{"HistoryFinalDelete", "", []int64{n}}, &okH)
	if errQ != nil && errH != nil {
		return errQ
	}
	return nil
}
