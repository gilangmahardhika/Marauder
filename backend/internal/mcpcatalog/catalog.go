// Package mcpcatalog lists the MCP servers a tribe admin can set up: the
// built-in database connectors and the featured servers, which answer at
// once, and a search of the official MCP Registry (RFC-096, RFC-098,
// RFC-099, RFC-100).
package mcpcatalog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"orchestrator/backend/internal/dbtools"
	"orchestrator/backend/internal/netguard"
	"orchestrator/backend/internal/sandbox"
)

const (
	DefaultRegistry = "https://registry.modelcontextprotocol.io"
	// RegistryError is the fixed notice when the registry cannot be read.
	RegistryError = "The MCP Registry could not be reached"

	maxBody        = 512 << 10
	fetchTimeout   = 60 * time.Second
	failTTL        = 30 * time.Second
	maxFetches     = 4
	cacheTTL       = 10 * time.Minute
	maxCached      = 128
	pageSize       = 30
	maxDescription = 512
	maxQuery       = 100
	officialMeta   = "io.modelcontextprotocol.registry/official"
	noNetwork      = "Runs with no network. Pull and pin the image before saving."
)

var ErrNotFound = errors.New("catalog entry not found")

// headerName is the header rule of a saved server; a registry header that
// breaks it is left for the admin to add by hand.
var headerName = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)

// Secret is a header, environment variable, or password the entry needs.
type Secret struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// Tool is a built-in entry's fixed tool, in the allowlist's shape.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
}

// Entry is one server the admin can pick. Supported is false with a Reason
// for a server the harness cannot run.
type Entry struct {
	ID          string   `json:"id"`
	Source      string   `json:"source"`
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	WebsiteURL  string   `json:"websiteUrl"`
	Version     string   `json:"version"`
	Transport   string   `json:"transport"`
	Supported   bool     `json:"supported"`
	Reason      string   `json:"reason,omitempty"`
	Note        string   `json:"note,omitempty"`
	URL         string   `json:"url,omitempty"`
	Image       string   `json:"image,omitempty"`
	Argv        []string `json:"argv"`
	Port        int      `json:"port,omitempty"`
	Secrets     []Secret `json:"secrets"`
	Tools       []Tool   `json:"tools"`
}

// Page is one catalog search. RegistryError is set when the registry could
// not be read; the built-in entries are still listed.
type Page struct {
	Entries       []Entry `json:"entries"`
	NextCursor    string  `json:"nextCursor"`
	RegistryError string  `json:"registryError,omitempty"`
}

// Catalog searches the registry through Client and caches each query and
// cursor for 10 minutes. A failed query is remembered for 30 seconds, and at
// most 4 fetches run at once. Timeout is the budget of one fetch.
type Catalog struct {
	Base    string
	Client  *http.Client
	Policy  netguard.Policy
	Now     func() time.Time
	Timeout time.Duration

	mu     sync.Mutex
	cache  map[string]cached
	failed map[string]time.Time
	slots  chan struct{}
}

type cached struct {
	at      time.Time
	entries []Entry
	next    string
}

// New is a catalog of the registry at base, read with the netguard client.
// The registry may take the whole fetch budget before its headers arrive, so
// the catalog's transport waits that long; MCP servers keep netguard's own.
func New(base string, policy netguard.Policy) *Catalog {
	client := netguard.Client(policy, nil)
	if t, ok := client.Transport.(*http.Transport); ok {
		t = t.Clone()
		t.ResponseHeaderTimeout = fetchTimeout
		client.Transport = t
	}
	return &Catalog{Base: strings.TrimRight(base, "/"), Client: client, Policy: policy}
}

func (c *Catalog) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return fetchTimeout
}

func (c *Catalog) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Builtins are the database connectors.
func Builtins() []Entry {
	var tools []Tool
	for _, t := range dbtools.Tools() {
		tools = append(tools, Tool{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	password := []Secret{{Name: dbtools.PasswordSecret, Description: "Password of a login that can only read.", Required: false}}
	return []Entry{
		{
			ID: "builtin:postgres", Source: "builtin", Name: "postgres", Title: "PostgreSQL",
			Description: "Read-only SQL on a Postgres database. The harness connects itself, to any host but the board database.",
			Transport:   dbtools.Postgres, Supported: true, Port: dbtools.DefaultPort(dbtools.Postgres),
			Argv: []string{}, Secrets: password, Tools: tools,
		},
		{
			ID: "builtin:sqlserver", Source: "builtin", Name: "sqlserver", Title: "SQL Server",
			Description: "Read-only SQL on a Microsoft SQL Server database. Use a login with db_datareader only.",
			Transport:   dbtools.SQLServer, Supported: true, Port: dbtools.DefaultPort(dbtools.SQLServer),
			Argv: []string{}, Secrets: password, Tools: tools,
		},
	}
}

// Featured are well-known remote servers that work with a static header or
// none (RFC-099). They ship with the harness; GitHub and Linear use their
// read-only endpoints.
func Featured() []Entry {
	bearer := func(hint string, required bool) []Secret {
		return []Secret{{Name: "Authorization", Description: hint, Required: required}}
	}
	entry := func(name, title, description, site, u string, s []Secret) Entry {
		if s == nil {
			s = []Secret{}
		}
		return Entry{
			ID: "featured:" + name, Source: "featured", Name: name, Title: title, Description: description,
			WebsiteURL: site, Transport: "http", Supported: true, URL: u,
			Argv: []string{}, Secrets: s, Tools: []Tool{},
		}
	}
	return []Entry{
		entry("github", "GitHub", "Read repositories, issues, pull requests, and code search through GitHub's read-only endpoint.",
			"https://github.com/github/github-mcp-server", "https://api.githubcopilot.com/mcp/readonly",
			bearer("Bearer, then a fine-grained personal access token with read access.", true)),
		entry("linear", "Linear", "Read Linear issues, projects, and comments through Linear's read-only endpoint.",
			"https://linear.app/docs/mcp", "https://mcp.linear.app/mcp/readonly",
			bearer("Bearer, then a Linear API key.", true)),
		entry("stripe", "Stripe", "Look up Stripe customers, payments, subscriptions, and docs.",
			"https://docs.stripe.com/mcp", "https://mcp.stripe.com",
			bearer("Bearer, then a restricted key tagged for agents, with read permissions only.", true)),
		entry("context7", "Context7", "Current documentation and code examples for libraries and frameworks.",
			"https://context7.com", "https://mcp.context7.com/mcp",
			bearer("Bearer, then a Context7 API key, for higher rate limits.", false)),
		entry("huggingface", "Hugging Face", "Search models, datasets, Spaces, and papers on the Hugging Face Hub.",
			"https://huggingface.co/docs/hub/agents-mcp", "https://huggingface.co/mcp",
			bearer("Bearer, then a Hugging Face access token.", false)),
		entry("exa", "Exa", "Web and code search built for agents.",
			"https://exa.ai/docs/get-started/exa-mcp", "https://mcp.exa.ai/mcp",
			[]Secret{{Name: "x-api-key", Description: "An Exa API key, for higher limits.", Required: false}}),
		entry("firecrawl", "Firecrawl", "Search the web and read pages as clean markdown.",
			"https://docs.firecrawl.dev/mcp-server", "https://mcp.firecrawl.dev/v2/mcp",
			bearer("Bearer, then a Firecrawl API key.", false)),
		entry("microsoftlearn", "Microsoft Learn", "Search and read official Microsoft and Azure documentation.",
			"https://github.com/MicrosoftDocs/mcp", "https://learn.microsoft.com/api/mcp", nil),
		entry("awsknowledge", "AWS Knowledge", "Search AWS documentation, API references, and architecture guidance.",
			"https://github.com/awslabs/mcp", "https://knowledge-mcp.global.api.aws", nil),
		entry("cloudflaredocs", "Cloudflare Docs", "Search Cloudflare's developer documentation.",
			"https://github.com/cloudflare/mcp-server-cloudflare", "https://docs.mcp.cloudflare.com/mcp", nil),
		entry("deepwiki", "DeepWiki", "Ask questions about public GitHub repositories, answered from generated wiki pages.",
			"https://deepwiki.com", "https://mcp.deepwiki.com/mcp", nil),
	}
}

var keywords = map[string][]string{
	"builtin:postgres":        {"postgres", "postgresql", "pg", "psql", "database", "sql", "db"},
	"builtin:sqlserver":       {"sqlserver", "sql server", "mssql", "microsoft", "azure sql", "database", "sql", "db", "tsql"},
	"featured:github":         {"github", "git", "repository", "code", "pull request", "issues"},
	"featured:linear":         {"linear", "issues", "tickets", "project"},
	"featured:stripe":         {"stripe", "payments", "billing"},
	"featured:context7":       {"context7", "docs", "documentation", "library"},
	"featured:huggingface":    {"huggingface", "hugging face", "models", "datasets", "ml"},
	"featured:exa":            {"exa", "web search", "search"},
	"featured:firecrawl":      {"firecrawl", "web", "scrape", "crawl", "search"},
	"featured:microsoftlearn": {"microsoft", "azure", "learn", "docs", "documentation", "dotnet"},
	"featured:awsknowledge":   {"aws", "amazon", "docs", "documentation", "cloud"},
	"featured:cloudflaredocs": {"cloudflare", "workers", "docs", "documentation"},
	"featured:deepwiki":       {"deepwiki", "wiki", "repository", "github", "docs"},
}

func matches(e Entry, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(e.Title+" "+e.Description), q) {
		return true
	}
	for _, w := range keywords[e.ID] {
		if strings.Contains(w, q) {
			return true
		}
	}
	return false
}

func cleanQuery(q string) string {
	q = strings.TrimSpace(q)
	if len(q) > maxQuery {
		q = strings.ToValidUTF8(q[:maxQuery], "")
	}
	return q
}

// Local lists the built-in and featured entries that match q. It never
// calls the registry.
func Local(q string) Page {
	lower := strings.ToLower(cleanQuery(q))
	page := Page{Entries: []Entry{}}
	for _, e := range append(Builtins(), Featured()...) {
		if matches(e, lower) {
			page.Entries = append(page.Entries, e)
		}
	}
	return page
}

// Registry is one page of registry servers for q and cursor.
func (c *Catalog) Registry(ctx context.Context, q, cursor string) Page {
	entries, next, err := c.registry(ctx, cleanQuery(q), cursor)
	if err != nil {
		return Page{Entries: []Entry{}, RegistryError: RegistryError}
	}
	return Page{Entries: entries, NextCursor: next}
}

// Search is Local on the first page, then one page of registry servers.
func (c *Catalog) Search(ctx context.Context, q, cursor string) Page {
	page := Page{Entries: []Entry{}}
	if cursor == "" {
		page = Local(q)
	}
	reg := c.Registry(ctx, q, cursor)
	page.Entries = append(page.Entries, reg.Entries...)
	page.NextCursor = reg.NextCursor
	page.RegistryError = reg.RegistryError
	return page
}

// LocalEntry returns a built-in or featured entry by id.
func LocalEntry(id string) (Entry, bool) {
	for _, e := range append(Builtins(), Featured()...) {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Entry returns one entry by id, for the setup form. A registry id is found
// in the cached pages first, then by its exact name and version.
func (c *Catalog) Entry(ctx context.Context, id string) (Entry, error) {
	if e, ok := LocalEntry(id); ok {
		return e, nil
	}
	rest, ok := strings.CutPrefix(id, "registry:")
	if !ok {
		return Entry{}, ErrNotFound
	}
	name, version := rest, "latest"
	if i := strings.LastIndex(rest, "@"); i > 0 {
		name, version = rest[:i], rest[i+1:]
	}
	if name == "" || version == "" {
		return Entry{}, ErrNotFound
	}
	c.mu.Lock()
	for _, hit := range c.cache {
		if c.now().Sub(hit.at) >= cacheTTL {
			continue
		}
		for _, e := range hit.entries {
			if e.ID == id {
				c.mu.Unlock()
				return e, nil
			}
		}
	}
	c.mu.Unlock()
	return c.fetchOne(ctx, name, version)
}

func (c *Catalog) registry(ctx context.Context, q, cursor string) ([]Entry, string, error) {
	key := q + "\x00" + cursor
	c.mu.Lock()
	if hit, ok := c.cache[key]; ok && c.now().Sub(hit.at) < cacheTTL {
		c.mu.Unlock()
		return hit.entries, hit.next, nil
	}
	if at, ok := c.failed[key]; ok && c.now().Sub(at) < failTTL {
		c.mu.Unlock()
		return nil, "", errors.New("registry failed recently")
	}
	c.mu.Unlock()

	var entries []Entry
	var next string
	err := c.slot(ctx, func() error {
		var err error
		entries, next, err = c.fetch(ctx, q, cursor)
		return err
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		if ctx.Err() == nil {
			if c.failed == nil || len(c.failed) >= maxCached {
				c.failed = map[string]time.Time{}
			}
			c.failed[key] = c.now()
		}
		return nil, "", err
	}
	delete(c.failed, key)
	if c.cache == nil {
		c.cache = map[string]cached{}
	}
	if len(c.cache) >= maxCached {
		for k, hit := range c.cache {
			if c.now().Sub(hit.at) >= cacheTTL {
				delete(c.cache, k)
			}
		}
		if len(c.cache) >= maxCached {
			c.cache = map[string]cached{}
		}
	}
	c.cache[key] = cached{at: c.now(), entries: entries, next: next}
	return entries, next, nil
}

// slot runs fn while holding one of the catalog's fetch slots, waiting for
// one until ctx ends.
func (c *Catalog) slot(ctx context.Context, fn func() error) error {
	c.mu.Lock()
	if c.slots == nil {
		c.slots = make(chan struct{}, maxFetches)
	}
	slots := c.slots
	c.mu.Unlock()
	select {
	case slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-slots }()
	return fn()
}

type regInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	IsRequired  bool   `json:"isRequired"`
}

type regArgument struct {
	Type    string `json:"type"`
	Value   string `json:"value"`
	Default string `json:"default"`
}

type regServer struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
	WebsiteURL  string `json:"websiteUrl"`
	Repository  struct {
		URL string `json:"url"`
	} `json:"repository"`
	Remotes []struct {
		Type    string     `json:"type"`
		URL     string     `json:"url"`
		Headers []regInput `json:"headers"`
	} `json:"remotes"`
	Packages []struct {
		RegistryType string `json:"registryType"`
		Identifier   string `json:"identifier"`
		Version      string `json:"version"`
		Transport    struct {
			Type string `json:"type"`
		} `json:"transport"`
		EnvironmentVariables []regInput    `json:"environmentVariables"`
		PackageArguments     []regArgument `json:"packageArguments"`
	} `json:"packages"`
}

type regItem struct {
	Server regServer                  `json:"server"`
	Meta   map[string]json.RawMessage `json:"_meta"`
}

type regList struct {
	Servers  []regItem `json:"servers"`
	Metadata struct {
		NextCursor string `json:"nextCursor"`
	} `json:"metadata"`
}

// get reads one registry path within the fetch budget and 512 KiB. A 404 is
// ErrNotFound.
func (c *Catalog) get(ctx context.Context, path string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("registry status " + resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxBody {
		return nil, errors.New("registry response over 512 KiB")
	}
	return body, nil
}

// fetchOne reads one server version by its exact name, which the registry
// answers far faster than a search.
func (c *Catalog) fetchOne(ctx context.Context, name, version string) (Entry, error) {
	var item regItem
	err := c.slot(ctx, func() error {
		body, err := c.get(ctx, "/v0/servers/"+url.PathEscape(name)+"/versions/"+url.PathEscape(version))
		if err != nil {
			return err
		}
		return json.Unmarshal(body, &item)
	})
	if err != nil {
		return Entry{}, err
	}
	if item.Server.Name != name {
		return Entry{}, ErrNotFound
	}
	return c.fromRegistry(item.Server), nil
}

func (c *Catalog) fetch(ctx context.Context, q, cursor string) ([]Entry, string, error) {
	params := url.Values{}
	params.Set("limit", "30")
	params.Set("version", "latest")
	if q != "" {
		params.Set("search", q)
	}
	if cursor != "" {
		params.Set("cursor", cursor)
	}
	body, err := c.get(ctx, "/v0/servers?"+params.Encode())
	if err != nil {
		return nil, "", err
	}
	var list regList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, "", err
	}
	seen := map[string]bool{}
	entries := []Entry{}
	for _, item := range list.Servers {
		if !latest(item.Meta) || item.Server.Name == "" || seen[item.Server.Name] {
			continue
		}
		seen[item.Server.Name] = true
		entries = append(entries, c.fromRegistry(item.Server))
		if len(entries) == pageSize {
			break
		}
	}
	return entries, list.Metadata.NextCursor, nil
}

func latest(meta map[string]json.RawMessage) bool {
	raw, ok := meta[officialMeta]
	if !ok {
		return true
	}
	var m struct {
		IsLatest *bool `json:"isLatest"`
	}
	if json.Unmarshal(raw, &m) != nil || m.IsLatest == nil {
		return true
	}
	return *m.IsLatest
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.ToValidUTF8(s[:n], "")
}

func secrets(in []regInput, ok func(string) bool) []Secret {
	out := []Secret{}
	for _, v := range in {
		if v.Name == "" || !ok(v.Name) {
			continue
		}
		out = append(out, Secret{Name: v.Name, Description: clip(v.Description, maxDescription), Required: v.IsRequired})
	}
	return out
}

// fromRegistry maps a registry server to an entry: a Streamable HTTP remote
// first, then an OCI image with stdio, else an entry that cannot be set up.
func (c *Catalog) fromRegistry(s regServer) Entry {
	e := Entry{
		ID: "registry:" + s.Name + "@" + s.Version, Source: "registry", Name: s.Name,
		Title: s.Title, Description: clip(s.Description, maxDescription), Version: s.Version,
		Argv: []string{}, Secrets: []Secret{}, Tools: []Tool{},
	}
	if e.Title == "" {
		e.Title = s.Name[strings.LastIndex(s.Name, "/")+1:]
	}
	for _, site := range []string{s.WebsiteURL, s.Repository.URL} {
		if strings.HasPrefix(site, "https://") {
			e.WebsiteURL = site
			break
		}
	}
	reason := ""
	for _, r := range s.Remotes {
		switch {
		case r.Type != "streamable-http":
			if reason == "" {
				reason = "SSE servers are not supported. Only Streamable HTTP is."
			}
		case strings.Contains(r.URL, "{"):
			if reason == "" {
				reason = "The server URL needs values the catalog cannot fill."
			}
		default:
			if _, err := netguard.CheckURL(r.URL, c.Policy); err != nil {
				if reason == "" {
					reason = "The server URL is not public https without a query string."
				}
				continue
			}
			e.Transport = "http"
			e.Supported = true
			e.URL = r.URL
			e.Secrets = secrets(r.Headers, headerName.MatchString)
			return e
		}
	}
	for _, p := range s.Packages {
		switch {
		case p.RegistryType == "oci" && p.Transport.Type == "stdio" && p.Identifier != "":
			e.Transport = "stdio"
			e.Supported = true
			e.Note = noNetwork
			e.Image = imageRef(p.Identifier, p.Version)
			for _, a := range p.PackageArguments {
				v := a.Value
				if v == "" {
					v = a.Default
				}
				if a.Type == "positional" && v != "" {
					e.Argv = append(e.Argv, v)
				}
			}
			e.Secrets = secrets(p.EnvironmentVariables, func(n string) bool { return !sandbox.ReservedEnv(n) })
			return e
		case reason != "":
		case p.RegistryType == "oci":
			reason = "This image serves HTTP, which the sandbox cannot expose."
		case p.RegistryType == "npm" || p.RegistryType == "pypi":
			reason = p.RegistryType + " packages need a package runner, which the sandbox does not have."
		default:
			reason = p.RegistryType + " packages are not supported."
		}
	}
	if reason == "" {
		reason = "The registry lists no way to run this server."
	}
	e.Reason = reason
	if e.Transport == "" {
		e.Transport = "stdio"
		if len(s.Remotes) > 0 {
			e.Transport = "http"
		}
	}
	return e
}

// imageRef adds the package version as a tag when the identifier has none.
func imageRef(identifier, version string) string {
	last := identifier[strings.LastIndex(identifier, "/")+1:]
	if version == "" || strings.ContainsAny(last, ":@") {
		return identifier
	}
	return identifier + ":" + version
}
