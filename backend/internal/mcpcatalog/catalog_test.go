package mcpcatalog_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/mcpcatalog"
	"orchestrator/backend/internal/netguard"
)

const registryPage = `{"servers":[
 {"server":{"name":"io.example/tickets","title":"Tickets","description":"Find tickets","version":"2.0.0",
   "websiteUrl":"https://tickets.example.com",
   "remotes":[{"type":"streamable-http","url":"https://mcp.tickets.example.com/mcp",
     "headers":[{"name":"Authorization","description":"Bearer token","isRequired":true,"isSecret":true}]}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}},
 {"server":{"name":"io.example/tickets","version":"1.0.0","remotes":[{"type":"streamable-http","url":"https://old.example.com/mcp"}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":false}}},
 {"server":{"name":"io.github.acme/docs","description":"Docs search","version":"0.3.1",
   "packages":[{"registryType":"oci","identifier":"ghcr.io/acme/docs-mcp","version":"0.3.1","transport":{"type":"stdio"},
     "packageArguments":[{"type":"positional","value":"stdio"},{"type":"named","name":"--verbose"}],
     "environmentVariables":[{"name":"DOCS_TOKEN","description":"API token","isRequired":true},{"name":"PATH"}]}]},
  "_meta":{"io.modelcontextprotocol.registry/official":{"isLatest":true}}},
 {"server":{"name":"io.github.acme/pg-npm","version":"1.0.0","packages":[{"registryType":"npm","identifier":"@acme/pg","transport":{"type":"stdio"}}]}},
 {"server":{"name":"io.github.acme/sse","version":"1.0.0","remotes":[{"type":"sse","url":"https://sse.example.com/sse"}]}},
 {"server":{"name":"io.github.acme/templated","version":"1.0.0","remotes":[{"type":"streamable-http","url":"https://{tenant}.example.com/mcp"}]}}
],"metadata":{"nextCursor":"io.github.acme/templated:1.0.0","count":6}}`

type fakeRegistry struct {
	srv   *httptest.Server
	hits  atomic.Int32
	query atomic.Value
}

func newRegistry(t *testing.T, body string, status int) *fakeRegistry {
	t.Helper()
	f := &fakeRegistry{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		f.query.Store(r.URL.RawQuery)
		if r.URL.Path != "/v0/servers" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func catalog(base string) *mcpcatalog.Catalog {
	return mcpcatalog.New(base, netguard.Policy{AllowLoopbackHTTP: true})
}

func byID(entries []mcpcatalog.Entry) map[string]mcpcatalog.Entry {
	out := map[string]mcpcatalog.Entry{}
	for _, e := range entries {
		out[e.ID] = e
	}
	return out
}

func TestMCPCatalogBuiltins(t *testing.T) {
	reg := newRegistry(t, `{"servers":[],"metadata":{}}`, 200)
	cat := catalog(reg.srv.URL)
	page := mcpcatalog.Local("")
	if len(page.Entries) != 2+len(mcpcatalog.Featured()) || page.Entries[0].ID != "builtin:postgres" || page.Entries[1].ID != "builtin:sqlserver" {
		t.Fatalf("entries = %+v", page.Entries)
	}
	pg := page.Entries[0]
	if pg.Transport != dbtools.Postgres || pg.Port != 5432 || !pg.Supported || len(pg.Tools) != 3 || pg.Secrets[0].Name != dbtools.PasswordSecret {
		t.Fatalf("postgres = %+v", pg)
	}
	if page.Entries[1].Port != 1433 {
		t.Fatalf("sqlserver port = %d", page.Entries[1].Port)
	}
	for q, want := range map[string][]string{
		"mssql":    {"builtin:sqlserver"},
		"Postgres": {"builtin:postgres"},
		"database": {"builtin:postgres", "builtin:sqlserver"},
		"tickets":  {"featured:linear"},
		"payments": {"featured:stripe"},
		"nothing":  nil,
	} {
		got := mcpcatalog.Local(q).Entries
		if len(got) != len(want) {
			t.Fatalf("%s = %+v", q, got)
		}
		for i, id := range want {
			if got[i].ID != id {
				t.Fatalf("%s = %+v", q, got)
			}
		}
	}
	if reg.hits.Load() != 0 {
		t.Fatal("a local search called the registry")
	}
	if next := cat.Search(context.Background(), "", "cursor-1").Entries; len(next) != 0 {
		t.Fatalf("local entries on a later page: %+v", next)
	}
	entry, err := cat.Entry(context.Background(), "builtin:sqlserver")
	if err != nil || entry.Title != "SQL Server" {
		t.Fatalf("entry = %+v, %v", entry, err)
	}
}

func TestMCPCatalogFeatured(t *testing.T) {
	featured := mcpcatalog.Featured()
	if len(featured) < 8 {
		t.Fatalf("featured = %d", len(featured))
	}
	seen := map[string]bool{}
	for _, e := range featured {
		if seen[e.ID] || !strings.HasPrefix(e.ID, "featured:") || e.Source != "featured" || e.Transport != "http" || !e.Supported {
			t.Fatalf("entry = %+v", e)
		}
		seen[e.ID] = true
		if _, err := netguard.CheckURL(e.URL, netguard.Policy{}); err != nil || strings.Contains(e.URL, "{") {
			t.Fatalf("%s url %q: %v", e.ID, e.URL, err)
		}
		if !strings.HasPrefix(e.WebsiteURL, "https://") || e.Argv == nil || e.Tools == nil || e.Secrets == nil {
			t.Fatalf("entry = %+v", e)
		}
		for _, s := range e.Secrets {
			if s.Name != "Authorization" && s.Name != "x-api-key" {
				t.Fatalf("%s secret %q", e.ID, s.Name)
			}
		}
		if got, ok := mcpcatalog.LocalEntry(e.ID); !ok || got.URL != e.URL {
			t.Fatalf("LocalEntry(%s) = %+v", e.ID, got)
		}
	}
	gh, _ := mcpcatalog.LocalEntry("featured:github")
	if !strings.HasSuffix(gh.URL, "/readonly") || len(gh.Secrets) != 1 || !gh.Secrets[0].Required {
		t.Fatalf("github = %+v", gh)
	}
	lin, _ := mcpcatalog.LocalEntry("featured:linear")
	if !strings.HasSuffix(lin.URL, "/readonly") {
		t.Fatalf("linear = %+v", lin)
	}
	cat := catalog(newRegistry(t, registryPage, 500).srv.URL)
	if e, err := cat.Entry(context.Background(), "featured:deepwiki"); err != nil || len(e.Secrets) != 0 {
		t.Fatalf("deepwiki = %+v, %v", e, err)
	}
}

func TestMCPCatalogRegistrySlow(t *testing.T) {
	var hits atomic.Int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	cat := catalog(srv.URL)
	cat.Timeout = 100 * time.Millisecond
	start := time.Now()
	page := cat.Registry(context.Background(), "slow", "")
	if page.RegistryError != mcpcatalog.RegistryError || len(page.Entries) != 0 || time.Since(start) > 2*time.Second {
		t.Fatalf("page = %+v after %s", page, time.Since(start))
	}
	// A failure is remembered, so the next request does not wait again.
	start = time.Now()
	if page := cat.Registry(context.Background(), "slow", ""); page.RegistryError == "" || time.Since(start) > 50*time.Millisecond || hits.Load() != 1 {
		t.Fatalf("second page = %+v after %s, hits %d", page, time.Since(start), hits.Load())
	}
	// A request the browser gave up on is not remembered as a failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cat.Registry(ctx, "gone", "")
	cat.Timeout = 50 * time.Millisecond
	cat.Registry(context.Background(), "gone", "")
	if hits.Load() != 2 {
		t.Fatalf("hits = %d", hits.Load())
	}
}

func TestMCPCatalogEntryByName(t *testing.T) {
	var paths atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths.Store(r.URL.EscapedPath())
		if r.URL.EscapedPath() != "/v0/servers/io.example%2Ftickets/versions/2.0.0" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"server":{"name":"io.example/tickets","version":"2.0.0",
			"remotes":[{"type":"streamable-http","url":"https://mcp.tickets.example.com/mcp",
			"headers":[{"name":"Authorization","isRequired":true},{"name":"API_KEY"}]}]}}`))
	}))
	t.Cleanup(srv.Close)
	cat := catalog(srv.URL)
	e, err := cat.Entry(context.Background(), "registry:io.example/tickets@2.0.0")
	if err != nil || e.URL != "https://mcp.tickets.example.com/mcp" || len(e.Secrets) != 1 || e.Secrets[0].Name != "Authorization" {
		t.Fatalf("entry = %+v, %v (path %v)", e, err, paths.Load())
	}
	if _, err := cat.Entry(context.Background(), "registry:io.example/other@1.0.0"); err != mcpcatalog.ErrNotFound {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := cat.Entry(context.Background(), "featured:nope"); err != mcpcatalog.ErrNotFound {
		t.Fatalf("unknown featured err = %v", err)
	}
}

func TestMCPCatalogRegistry(t *testing.T) {
	reg := newRegistry(t, registryPage, 200)
	cat := catalog(reg.srv.URL)
	page := cat.Search(context.Background(), "acme tickets", "")
	if page.RegistryError != "" || page.NextCursor != "io.github.acme/templated:1.0.0" {
		t.Fatalf("page = %+v", page)
	}
	q, _ := reg.query.Load().(string)
	if !strings.Contains(q, "search=acme+tickets") || !strings.Contains(q, "version=latest") || !strings.Contains(q, "limit=30") {
		t.Fatalf("query = %s", q)
	}
	got := byID(page.Entries)
	if _, old := got["registry:io.example/tickets@1.0.0"]; old {
		t.Fatal("an old version is listed")
	}
	tickets := got["registry:io.example/tickets@2.0.0"]
	if !tickets.Supported || tickets.Transport != "http" || tickets.URL != "https://mcp.tickets.example.com/mcp" ||
		len(tickets.Secrets) != 1 || tickets.Secrets[0].Name != "Authorization" || !tickets.Secrets[0].Required ||
		tickets.WebsiteURL != "https://tickets.example.com" || tickets.Title != "Tickets" {
		t.Fatalf("tickets = %+v", tickets)
	}
	docs := got["registry:io.github.acme/docs@0.3.1"]
	if !docs.Supported || docs.Transport != "stdio" || docs.Image != "ghcr.io/acme/docs-mcp:0.3.1" ||
		strings.Join(docs.Argv, " ") != "stdio" || len(docs.Secrets) != 1 || docs.Secrets[0].Name != "DOCS_TOKEN" ||
		!strings.Contains(docs.Note, "no network") || docs.Title != "docs" {
		t.Fatalf("docs = %+v", docs)
	}
	for id, want := range map[string]string{
		"registry:io.github.acme/pg-npm@1.0.0":    "npm packages",
		"registry:io.github.acme/sse@1.0.0":       "Streamable HTTP",
		"registry:io.github.acme/templated@1.0.0": "cannot fill",
	} {
		e := got[id]
		if e.Supported || !strings.Contains(e.Reason, want) {
			t.Fatalf("%s = %+v", id, e)
		}
	}

	// A repeated query is served from the cache for 10 minutes.
	now := time.Now()
	cat.Now = func() time.Time { return now }
	cat.Search(context.Background(), "x", "")
	hits := reg.hits.Load()
	cat.Search(context.Background(), "x", "")
	if reg.hits.Load() != hits {
		t.Fatal("cached query fetched again")
	}
	cat.Now = func() time.Time { return now.Add(11 * time.Minute) }
	cat.Search(context.Background(), "x", "")
	if reg.hits.Load() != hits+1 {
		t.Fatal("expired query not fetched")
	}
	entry, err := cat.Entry(context.Background(), "registry:io.github.acme/docs@0.3.1")
	if err != nil || entry.Image != "ghcr.io/acme/docs-mcp:0.3.1" {
		t.Fatalf("entry = %+v, %v", entry, err)
	}
	if _, err := cat.Entry(context.Background(), "registry:io.github.acme/missing@1.0.0"); err != mcpcatalog.ErrNotFound {
		t.Fatalf("missing entry err = %v", err)
	}
}

func TestMCPCatalogRegistryDown(t *testing.T) {
	for name, cat := range map[string]*mcpcatalog.Catalog{
		"status":  catalog(newRegistry(t, `oops`, 500).srv.URL),
		"garbage": catalog(newRegistry(t, `not json`, 200).srv.URL),
		"large":   catalog(newRegistry(t, `{"servers":[],"x":"`+strings.Repeat("a", 600<<10)+`"}`, 200).srv.URL),
		"blocked": mcpcatalog.New(newRegistry(t, registryPage, 200).srv.URL, netguard.Policy{}),
	} {
		page := cat.Search(context.Background(), "", "")
		if page.RegistryError != mcpcatalog.RegistryError || len(page.Entries) != 2+len(mcpcatalog.Featured()) || page.Entries[0].Source != "builtin" {
			t.Fatalf("%s page = %+v", name, page)
		}
		if reg := cat.Registry(context.Background(), "", ""); reg.RegistryError != mcpcatalog.RegistryError || len(reg.Entries) != 0 {
			t.Fatalf("%s registry page = %+v", name, reg)
		}
	}
}
