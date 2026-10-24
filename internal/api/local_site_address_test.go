package api_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// A site that runs on the person's own network, like a BitMagnet: the
// definition lists an example address and the real one is the person's.
const localSiteDefinition = `---
id: localdht
name: Local DHT
description: "A search engine the person runs at home"
language: en-US
type: public
encoding: UTF-8
links:
  - http://127.0.0.1:3333/
  - http://bitmagnet:3333/

caps:
  categorymappings:
    - {id: 1, cat: Movies, desc: "Movies"}
  modes:
    search: [q]

search:
  paths:
    - path: list.json
      response:
        type: json
  inputs:
    q: "{{ .Keywords }}"
  rows:
    selector: data
  fields:
    title:
      selector: title
    category:
      text: 1
    details:
      text: "{{ .Config.sitelink }}movie"
    download:
      selector: url
    size:
      selector: size
    seeders:
      selector: seeders
    leechers:
      selector: leechers
    downloadvolumefactor:
      text: 0
    uploadvolumefactor:
      text: 1
`

const publicSiteDefinition = `---
id: publicsite
name: Public Site
description: "A site on the internet"
language: en-US
type: public
encoding: UTF-8
links:
  - https://public.example.test/

caps:
  categorymappings:
    - {id: 1, cat: Movies, desc: "Movies"}
  modes:
    search: [q]

search:
  paths:
    - path: list.json
      response:
        type: json
  rows:
    selector: data
  fields:
    title:
      selector: title
    category:
      text: 1
    download:
      selector: url
`

func localSiteArchive(t *testing.T) *httptest.Server {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, def := range map[string]string{"localdht": localSiteDefinition, "publicsite": publicSiteDefinition} {
		data := []byte(def)
		tw.WriteHeader(&tar.Header{Name: "Indexers-master/definitions/v11/" + name + ".yml", Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg})
		tw.Write(data)
	}
	tw.Close()
	gz.Close()
	archive := buf.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(archive) }))
	t.Cleanup(srv.Close)
	return srv
}

// fakeDHT answers the search of localSiteDefinition.
func fakeDHT(t *testing.T, hits *atomic.Int32) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/list.json" {
			http.NotFound(w, r)
			return
		}
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"data":[{"title":"Home.Made.Movie.2020.1080p.BluRay","url":"magnet:?xt=urn:btih:ABCDEF0123456789ABCDEF0123456789ABCDEF01","size":1073741824,"seeders":5,"leechers":1}]}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestLocalSiteTakesTheOwnersAddress(t *testing.T) {
	server, base, client := loginNewServer(t)
	server.Definitions.SourceURL = localSiteArchive(t).URL
	server.Cardigann.DefaultDelay = 0
	var hits atomic.Int32
	home := fakeDHT(t, &hits)

	defs := getJSON[map[string]any](t, client, base+"/api/indexer-definitions")
	local := map[string]bool{}
	for _, d := range defs["definitions"].([]any) {
		m := d.(map[string]any)
		local[m["id"].(string)], _ = m["local"].(bool)
	}
	if !local["localdht"] || local["publicsite"] {
		t.Fatalf("local flags = %v", local)
	}

	// the address of the person's own server is accepted and used
	created := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{
		"definitionId": "localdht", "name": "Home", "baseUrl": home.URL, "settings": map[string]any{},
	}, http.StatusCreated)
	id := int64(created["id"].(float64))
	if created["baseUrl"] != home.URL || created["localSite"] != true {
		t.Fatalf("created = %+v", created)
	}
	res := postJSON[map[string]any](t, client, fmt.Sprintf("%s/api/indexers/%d/test", base, id), nil, http.StatusOK)
	if res["ok"] != true || hits.Load() == 0 {
		t.Fatalf("test = %+v, hits %d", res, hits.Load())
	}

	// edit: another address on the home network; bad ones are refused
	url := fmt.Sprintf("%s/api/indexers/%d", base, id)
	tests := []struct {
		name string
		addr string
		want int
	}{
		{"a server by number", "http://192.168.1.10:3333", http.StatusOK},
		{"https and a name", "https://dht.lan", http.StatusOK},
		{"back to the one that works", home.URL, http.StatusOK},
		{"not a web address", "ftp://192.168.1.10/", http.StatusBadRequest},
		{"no scheme", "192.168.1.10:3333", http.StatusBadRequest},
		{"a user name in it", "http://bob:pw@192.168.1.10:3333", http.StatusBadRequest},
		{"a query", "http://192.168.1.10:3333/?a=1", http.StatusBadRequest},
		{"too long", "http://192.168.1.10/" + strings.Repeat("a", 200), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, body := doJSONStatus(t, client, http.MethodPut, url, map[string]any{"baseUrl": tt.addr})
			if code != tt.want {
				t.Fatalf("PUT %q = %d %v, want %d", tt.addr, code, body, tt.want)
			}
		})
	}
	got := getJSON[[]map[string]any](t, client, base+"/api/indexers")
	if len(got) != 1 || got[0]["baseUrl"] != home.URL || got[0]["localSite"] != true {
		t.Fatalf("list = %+v", got)
	}
}

func TestPublicSiteKeepsItsOwnAddresses(t *testing.T) {
	server, base, client := loginNewServer(t)
	server.Definitions.SourceURL = localSiteArchive(t).URL
	server.Cardigann.DefaultDelay = 0

	for _, addr := range []string{"http://192.168.1.10:3333", "http://127.0.0.1:9", "http://intranet/", "https://elsewhere.example.test/"} {
		code, body := doJSONStatus(t, client, http.MethodPost, base+"/api/indexers", map[string]any{"definitionId": "publicsite", "baseUrl": addr, "settings": map[string]any{}})
		if code != http.StatusBadRequest {
			t.Fatalf("adding at %q = %d %v, want 400", addr, code, body)
		}
	}
	created := postJSON[map[string]any](t, client, base+"/api/indexers", map[string]any{"definitionId": "publicsite", "settings": map[string]any{}}, http.StatusCreated)
	if created["baseUrl"] != "https://public.example.test/" || created["localSite"] != nil {
		t.Fatalf("created = %+v", created)
	}
	url := fmt.Sprintf("%s/api/indexers/%d", base, int64(created["id"].(float64)))
	code, body := doJSONStatus(t, client, http.MethodPut, url, map[string]any{"baseUrl": "http://192.168.1.10:3333"})
	if code != http.StatusBadRequest || !strings.Contains(fmt.Sprint(body["error"]), "own addresses") {
		t.Fatalf("PUT to a private address = %d %v", code, body)
	}
	code, _ = doJSONStatus(t, client, http.MethodPut, url, map[string]any{"baseUrl": "https://public.example.test"})
	if code != http.StatusOK {
		t.Fatalf("the site's own address (no trailing slash) = %d", code)
	}
}
