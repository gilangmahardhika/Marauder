package loop

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/runtime"
)

// LinkedSource is one project the card's project reads from. Git points at
// that project's clone and is only used to read and export.
type LinkedSource struct {
	ProjectID   string
	Name        string
	BaseBranch  string
	Git         runtime.Git
	SharedPaths []string
}

type linkedRef struct {
	ProjectID string `json:"projectId"`
	SHA       string `json:"sha"`
}

// prepareLinked exports each linked base SHA once under exportsRoot and reads
// its accepted requirements, decisions, and shared files. roots maps a linked
// project id to its export, the only root read_linked_file may use.
func prepareLinked(ctx context.Context, db *sql.DB, exportsRoot string, sources []LinkedSource) ([]contextpack.Linked, map[string]string, []linkedRef, error) {
	packs := make([]contextpack.Linked, 0, len(sources))
	roots := map[string]string{}
	refs := make([]linkedRef, 0, len(sources))
	for _, src := range sources {
		reqs, err := linkedRows(ctx, db, src.Name, `
			SELECT id, statement FROM requirements
			WHERE project_id = $1 AND accepted_at IS NOT NULL ORDER BY id`, src.ProjectID)
		if err != nil {
			return nil, nil, nil, err
		}
		decs, err := linkedRows(ctx, db, src.Name, `
			SELECT id, statement FROM decisions WHERE project_id = $1 ORDER BY id`, src.ProjectID)
		if err != nil {
			return nil, nil, nil, err
		}
		pack := contextpack.Linked{ProjectID: src.ProjectID, Name: src.Name, Requirements: reqs, Decisions: decs}
		if src.Git.Repo != "" && src.Git.Cloned() {
			base := src.BaseBranch
			if base == "" {
				base = "main"
			}
			sha, err := src.Git.RevParse(ctx, "refs/heads/"+base)
			if err == nil {
				dest := filepath.Join(exportsRoot, "linked-"+src.ProjectID+"-"+sha)
				if err := ensureExport(ctx, src.Git, sha, dest); err != nil {
					return nil, nil, nil, err
				}
				pack.SHA = sha
				pack.Files = sharedFiles(dest, src.SharedPaths)
				roots[src.ProjectID] = dest
			}
		}
		refs = append(refs, linkedRef{ProjectID: src.ProjectID, SHA: pack.SHA})
		packs = append(packs, pack)
	}
	return packs, roots, refs, nil
}

func linkedRows(ctx context.Context, db *sql.DB, name, query, projectID string) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id, statement string
		if err := rows.Scan(&id, &statement); err != nil {
			return nil, err
		}
		out = append(out, name+":"+id+" "+statement)
	}
	return out, rows.Err()
}

func ensureExport(ctx context.Context, git runtime.Git, sha, dest string) error {
	if info, err := os.Stat(dest); err == nil && info.IsDir() {
		return nil
	}
	tmp := dest + ".partial"
	_ = os.RemoveAll(tmp)
	if err := git.Export(ctx, sha, tmp); err != nil {
		_ = os.RemoveAll(tmp)
		return err
	}
	return os.Rename(tmp, dest)
}

// sharedFiles reads the shared paths through SafePath, up to the per-link cap.
func sharedFiles(root string, paths []string) []contextpack.File {
	var files []contextpack.File
	budget := contextpack.LinkedCap
	add := func(rel, full string) bool {
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() {
			return true
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return true
		}
		if len(body) > budget {
			body = body[:budget]
		}
		budget -= len(body)
		files = append(files, contextpack.File{Path: rel, Text: string(body)})
		return budget > 0
	}
	for _, p := range paths {
		if budget <= 0 {
			break
		}
		full, err := runtime.SafePath(root, p)
		if err != nil {
			continue
		}
		info, err := os.Lstat(full)
		if err != nil {
			continue
		}
		if info.Mode().IsRegular() {
			add(filepath.ToSlash(p), full)
			continue
		}
		if !info.IsDir() {
			continue
		}
		var found []string
		_ = filepath.WalkDir(full, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.Type().IsRegular() {
				found = append(found, path)
			}
			return nil
		})
		sort.Strings(found)
		for _, path := range found {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				continue
			}
			if _, err := runtime.SafePath(root, rel); err != nil {
				continue
			}
			if !add(filepath.ToSlash(rel), path) {
				break
			}
		}
	}
	return files
}
