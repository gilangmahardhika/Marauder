package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"orchestrator/backend/internal/execx"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/mcpcatalog"
	"orchestrator/backend/internal/sandbox"
)

type MCPCatalogPage = mcpcatalog.Page
type MCPCatalogEntry = mcpcatalog.Entry

// ErrRegistry is a registry that could not be read for one entry.
var ErrRegistry = errors.New(mcpcatalog.RegistryError)

// MCPCatalog searches the catalog for an admin of the tribe (RFC-096,
// RFC-100). Source "local" lists the built-in and featured entries without
// the registry, "registry" lists one registry page, and "" lists both. A nil
// catalog has no registry and reports the notice.
func MCPCatalog(ctx context.Context, db *sql.DB, actor User, cat *mcpcatalog.Catalog, tribeID, source, q, cursor string) (MCPCatalogPage, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return MCPCatalogPage{}, err
	}
	if source != "" && source != "local" && source != "registry" {
		return MCPCatalogPage{}, &identity.Validation{Fields: map[string]string{"source": "source is local or registry"}}
	}
	if source == "local" {
		return mcpcatalog.Local(q), nil
	}
	if cat == nil {
		page := MCPCatalogPage{Entries: []MCPCatalogEntry{}, RegistryError: mcpcatalog.RegistryError}
		if source == "" && cursor == "" {
			page.Entries = mcpcatalog.Local(q).Entries
		}
		return page, nil
	}
	if source == "registry" {
		return cat.Registry(ctx, q, cursor), nil
	}
	return cat.Search(ctx, q, cursor), nil
}

// MCPCatalogItem returns one catalog entry for the setup form.
func MCPCatalogItem(ctx context.Context, db *sql.DB, actor User, cat *mcpcatalog.Catalog, tribeID, id string) (MCPCatalogEntry, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return MCPCatalogEntry{}, err
	}
	if cat == nil {
		if e, ok := mcpcatalog.LocalEntry(id); ok {
			return e, nil
		}
		return MCPCatalogEntry{}, identity.ErrNotFound
	}
	entry, err := cat.Entry(ctx, id)
	switch {
	case errors.Is(err, mcpcatalog.ErrNotFound):
		return MCPCatalogEntry{}, identity.ErrNotFound
	case err != nil:
		return MCPCatalogEntry{}, ErrRegistry
	}
	return entry, nil
}

// ImagePuller pulls a stdio server's image for the catalog (RFC-098). An
// empty Engine, or a server in fixture mode, refuses.
type ImagePuller struct {
	Engine  string
	Fixture bool
	Run     func(context.Context, execx.Child) (execx.Result, error)
}

// PullMCPImage pulls ref and returns it pinned by digest.
func PullMCPImage(ctx context.Context, db *sql.DB, actor User, p ImagePuller, tribeID, ref string) (string, error) {
	if err := ManageTribe(ctx, db, actor, tribeID); err != nil {
		return "", err
	}
	ref = strings.TrimSpace(ref)
	if p.Fixture || p.Engine == "" {
		return "", &identity.Validation{Fields: map[string]string{"image": ErrMCPStdio.Error()}}
	}
	if !sandbox.ImageRef(ref) {
		return "", &identity.Validation{Fields: map[string]string{"image": "image is a name such as ghcr.io/acme/server:1.0"}}
	}
	run := p.Run
	if run == nil {
		run = execx.Command
	}
	pinned, err := sandbox.PullImage(ctx, p.Engine, ref, run)
	if err != nil {
		return "", &identity.Validation{Fields: map[string]string{"image": sandbox.ErrPull.Error()}}
	}
	return pinned, nil
}
