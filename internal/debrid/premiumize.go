package debrid

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/rdborg/mediarium/internal/netguard"
)

// PremiumizeURL is Premiumize's API.
const PremiumizeURL = "https://www.premiumize.me/api"

const (
	// premiumizeListFor is how long one answer of /transfer/list is reused.
	// Several downloads waiting at once then ask Premiumize once, not once each.
	premiumizeListFor = 5 * time.Second
	// premiumizeCheckBatch is how many hashes go into one cache check.
	premiumizeCheckBatch = 50
	// premiumizeMaxDepth and premiumizeMaxFiles keep a strange folder tree from
	// turning into endless requests.
	premiumizeMaxDepth = 6
	premiumizeMaxFiles = 2000
	// instantPrefix marks an id for a torrent that Premiumize already had: its
	// files came from /transfer/directdl, no transfer exists on the account.
	instantPrefix = "dd-"
)

type premiumize struct {
	key  string
	http *http.Client
	base string

	// listFor is how long a transfer list is reused (0 asks every time).
	listFor time.Duration
}

// pmList is the last transfer list for one account. Every download that
// waits on Premiumize has its own client, so the list is kept here, shared by
// address and key, and they all ask Premiumize once together.
type pmList struct {
	mu   sync.Mutex
	list []pmTransfer
	at   time.Time
}

var pmLists sync.Map // address + key -> *pmList

func (p *premiumize) lists() *pmList {
	v, _ := pmLists.LoadOrStore(p.base+"\x00"+p.key, &pmList{})
	return v.(*pmList)
}

func (p *premiumize) Name() string { return Premiumize }

// pmAnswer is what every Premiumize call starts with.
type pmAnswer struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// call sends one request. The key travels in a header only, never in the
// address. out gets the whole answer (the status and message included).
func (p *premiumize) call(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string, out any) error {
	target := p.base + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return fmt.Errorf("build Premiumize request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.key)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach Premiumize: %w", netguard.CleanError(err))
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrAuth
	case http.StatusTooManyRequests:
		return errors.New("Premiumize says there were too many requests; Mediarium will try again in a moment")
	}
	var a pmAnswer
	_ = json.Unmarshal(data, &a)
	if a.Status == "error" {
		if pmIsAuthMessage(a.Message) {
			return ErrAuth
		}
		return errors.New("Premiumize: " + pmMessage(a.Message))
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("Premiumize: %s (HTTP %d)", pmMessage(a.Message), resp.StatusCode)
	}
	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("Premiumize sent an answer Mediarium couldn't read: %w", err)
	}
	return nil
}

// pmMessage is the service's own words, or a stand-in when it has none.
func pmMessage(m string) string {
	if m = strings.TrimSpace(m); m != "" {
		return m
	}
	return "it gave no reason"
}

// pmIsAuthMessage spots the ways Premiumize says the key is wrong or missing
// when it answers with an ordinary status 200.
func pmIsAuthMessage(m string) bool {
	m = strings.ToLower(m)
	return strings.Contains(m, "not logged in") || strings.Contains(m, "invalid api") ||
		strings.Contains(m, "apikey") || strings.Contains(m, "api key") || strings.Contains(m, "customer_id and pin")
}

func (p *premiumize) post(ctx context.Context, path string, form url.Values, out any) error {
	return p.call(ctx, http.MethodPost, path, nil, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", out)
}

func (p *premiumize) Account(ctx context.Context) (Account, error) {
	var u struct {
		CustomerID   any     `json:"customer_id"`
		PremiumUntil float64 `json:"premium_until"`
		LimitUsed    float64 `json:"limit_used"`
	}
	if err := p.call(ctx, http.MethodGet, "/account/info", nil, nil, "", &u); err != nil {
		return Account{}, err
	}
	a := Account{Name: strings.TrimSpace(fmt.Sprint(u.CustomerID)), FairUse: u.LimitUsed}
	if u.CustomerID == nil {
		a.Name = ""
	}
	if u.PremiumUntil > 0 {
		a.Expires = time.Unix(int64(u.PremiumUntil), 0).UTC()
		a.Premium = a.Expires.After(time.Now())
	}
	return a, nil
}

var btihPattern = regexp.MustCompile(`(?i)xt=urn:btih:([a-z0-9]{32,40})(?:&|$)`)

// magnetHash is the info hash of a magnet link, lower case, or "".
func magnetHash(magnet string) string {
	if m := btihPattern.FindStringSubmatch(magnet); m != nil {
		return strings.ToLower(m[1])
	}
	return ""
}

// Add hands over the torrent. A magnet that Premiumize already has is read
// straight away (/transfer/directdl) and nothing is added to the account;
// anything else becomes a transfer.
func (p *premiumize) Add(ctx context.Context, magnet string, torrentFile []byte) (string, error) {
	if hash := magnetHash(magnet); hash != "" {
		files, err := p.directDL(ctx, "magnet:?xt=urn:btih:"+hash)
		switch {
		case err == nil && len(files) > 0:
			return instantPrefix + hash, nil
		case errors.Is(err, ErrAuth):
			return "", err
		}
		// Not cached, or the quick path failed: a normal transfer does it.
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if magnet != "" {
		_ = mw.WriteField("src", magnet)
	} else {
		part, err := mw.CreateFormFile("file", "release.torrent")
		if err != nil {
			return "", err
		}
		_, _ = part.Write(torrentFile)
	}
	_ = mw.Close()
	var added struct {
		ID string `json:"id"`
	}
	if err := p.call(ctx, http.MethodPost, "/transfer/create", nil, &buf, mw.FormDataContentType(), &added); err != nil {
		return "", err
	}
	if added.ID == "" {
		return "", errors.New("Premiumize didn't take the torrent")
	}
	p.forgetList()
	return added.ID, nil
}

type pmTransfer struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Message  string   `json:"message"`
	Status   string   `json:"status"`
	Progress *float64 `json:"progress"`
	Src      string   `json:"src"`
	FolderID string   `json:"folder_id"`
	FileID   string   `json:"file_id"`
}

func (p *premiumize) forgetList() {
	l := p.lists()
	l.mu.Lock()
	l.at = time.Time{}
	l.mu.Unlock()
}

// transfers is /transfer/list, reused for a few seconds.
func (p *premiumize) transfers(ctx context.Context, fresh bool) ([]pmTransfer, error) {
	cache := p.lists()
	cache.mu.Lock()
	if !fresh && !cache.at.IsZero() && time.Since(cache.at) < p.listFor {
		out := cache.list
		cache.mu.Unlock()
		return out, nil
	}
	cache.mu.Unlock()
	var l struct {
		Transfers []pmTransfer `json:"transfers"`
	}
	if err := p.call(ctx, http.MethodGet, "/transfer/list", nil, nil, "", &l); err != nil {
		return nil, err
	}
	cache.mu.Lock()
	cache.list, cache.at = l.Transfers, time.Now()
	cache.mu.Unlock()
	return l.Transfers, nil
}

func (p *premiumize) transfer(ctx context.Context, id string, fresh bool) (pmTransfer, bool, error) {
	list, err := p.transfers(ctx, fresh)
	if err != nil {
		return pmTransfer{}, false, err
	}
	for _, t := range list {
		if t.ID == id {
			return t, true, nil
		}
	}
	return pmTransfer{}, false, nil
}

// pmState maps Premiumize's transfer status onto the shared states.
func pmState(status string) State {
	switch status {
	case "finished", "seeding":
		return Ready
	case "running":
		return Downloading
	case "error", "timeout", "banned", "deleted":
		return Failed
	}
	return Waiting // waiting, queued, or anything new
}

func (t pmTransfer) progress() float64 {
	if t.Progress == nil {
		return 0
	}
	return min(max(*t.Progress, 0), 1)
}

func (p *premiumize) Status(ctx context.Context, id string) (Status, error) {
	if strings.HasPrefix(id, instantPrefix) {
		return Status{State: Ready, Progress: 1, Seeders: -1}, nil
	}
	t, found, err := p.transfer(ctx, id, false)
	if err != nil {
		return Status{}, err
	}
	if !found {
		return Status{State: Failed, Message: "The torrent is no longer on Premiumize (it was deleted there).", Seeders: -1}, nil
	}
	st := Status{State: pmState(t.Status), Progress: t.progress(), Seeders: -1}
	switch st.State {
	case Ready:
		st.Progress = 1
	case Failed:
		st.Message = "Premiumize says: " + pmMessage(t.Message)
	}
	return st, nil
}

type pmItem struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Link string `json:"link"`
}

// directDL asks for the files of a torrent Premiumize already has.
func (p *premiumize) directDL(ctx context.Context, magnet string) ([]File, error) {
	var d struct {
		Content []struct {
			Path string `json:"path"`
			Size int64  `json:"size"`
			Link string `json:"link"`
		} `json:"content"`
	}
	if err := p.post(ctx, "/transfer/directdl", url.Values{"src": {magnet}}, &d); err != nil {
		return nil, err
	}
	var out []File
	for _, c := range d.Content {
		if c.Link == "" || c.Path == "" {
			continue
		}
		out = append(out, File{Name: c.Path, Size: c.Size, URL: c.Link})
	}
	return out, nil
}

func (p *premiumize) Files(ctx context.Context, id string) ([]File, error) {
	if hash, ok := strings.CutPrefix(id, instantPrefix); ok {
		files, err := p.directDL(ctx, "magnet:?xt=urn:btih:"+hash)
		if err != nil {
			return nil, err
		}
		if len(files) == 0 {
			return nil, errors.New("Premiumize has no files for this torrent")
		}
		return files, nil
	}
	t, found, err := p.transfer(ctx, id, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errors.New("The torrent is no longer on Premiumize")
	}
	var out []File
	switch {
	case t.FolderID != "":
		if err := p.walk(ctx, t.FolderID, SafeName(t.Name), 0, &out); err != nil {
			return nil, err
		}
	case t.FileID != "":
		var err error
		if out, err = p.single(ctx, t.FileID); err != nil {
			return nil, err
		}
	}
	if len(out) == 0 {
		return nil, errors.New("Premiumize has no files for this torrent")
	}
	return out, nil
}

// single is the one file a transfer made when it did not make a folder.
func (p *premiumize) single(ctx context.Context, fileID string) ([]File, error) {
	var d pmItem
	if err := p.call(ctx, http.MethodGet, "/item/details", url.Values{"id": {fileID}}, nil, "", &d); err != nil {
		return nil, err
	}
	if d.Link == "" {
		return nil, nil
	}
	return []File{{Name: d.Name, Size: d.Size, URL: d.Link}}, nil
}

// walk lists a folder and the folders inside it. prefix is the path of the
// folder inside the torrent.
func (p *premiumize) walk(ctx context.Context, folderID, prefix string, depth int, out *[]File) error {
	if depth > premiumizeMaxDepth {
		return nil
	}
	var l struct {
		Content []pmItem `json:"content"`
	}
	if err := p.call(ctx, http.MethodGet, "/folder/list", url.Values{"id": {folderID}}, nil, "", &l); err != nil {
		return err
	}
	for _, it := range l.Content {
		if len(*out) >= premiumizeMaxFiles {
			return nil
		}
		name := prefix + "/" + SafeName(it.Name)
		switch it.Type {
		case "folder":
			if err := p.walk(ctx, it.ID, name, depth+1, out); err != nil {
				return err
			}
		case "file":
			if it.Link != "" {
				*out = append(*out, File{Name: name, Size: it.Size, URL: it.Link})
			}
		}
	}
	return nil
}

// Remove deletes the transfer and the files it made in the cloud storage, so
// a finished torrent doesn't keep using up the account's space.
func (p *premiumize) Remove(ctx context.Context, id string) error {
	if strings.HasPrefix(id, instantPrefix) {
		return nil // nothing was added to the account
	}
	t, found, err := p.transfer(ctx, id, true)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := p.post(ctx, "/transfer/delete", url.Values{"id": {id}}, nil); err != nil {
		return err
	}
	p.forgetList()
	var errs []error
	if t.FolderID != "" {
		errs = append(errs, p.post(ctx, "/folder/delete", url.Values{"id": {t.FolderID}}, nil))
	}
	if t.FileID != "" {
		errs = append(errs, p.post(ctx, "/item/delete", url.Values{"id": {t.FileID}}, nil))
	}
	return errors.Join(errs...)
}

func (p *premiumize) List(ctx context.Context) ([]Item, error) {
	list, err := p.transfers(ctx, false)
	if err != nil {
		return nil, err
	}
	out := make([]Item, 0, len(list))
	for _, t := range list {
		it := Item{ID: t.ID, Name: t.Name, State: pmState(t.Status), Progress: t.progress(), Text: t.Status, Seeders: -1}
		switch it.State {
		case Ready:
			it.Progress = 1
		case Failed:
			it.Text = pmMessage(t.Message)
		}
		if t.Message != "" && it.State != Failed {
			it.Text = t.Message
		}
		out = append(out, it)
	}
	return out, nil
}

// Cached asks Premiumize which of the info hashes it already has
// (POST /cache/check, items[]). The answer is a list of booleans in the order
// asked.
func (p *premiumize) Cached(ctx context.Context, hashes []string) (map[string]bool, error) {
	out := make(map[string]bool, len(hashes))
	var ask []string
	for _, h := range hashes {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if _, dup := out[h]; !dup {
			out[h] = false
			ask = append(ask, h)
		}
	}
	for start := 0; start < len(ask); start += premiumizeCheckBatch {
		batch := ask[start:min(start+premiumizeCheckBatch, len(ask))]
		var r struct {
			Response []bool `json:"response"`
		}
		if err := p.post(ctx, "/cache/check", url.Values{"items[]": batch}, &r); err != nil {
			return nil, err
		}
		if len(r.Response) != len(batch) {
			return nil, errors.New("Premiumize answered the cache check with a list of the wrong length")
		}
		for i, h := range batch {
			out[h] = r.Response[i]
		}
	}
	return out, nil
}
