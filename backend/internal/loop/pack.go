package loop

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"orchestrator/backend/internal/contextpack"
	"orchestrator/backend/internal/runtime"
)

const (
	pathMapCap   = 12 << 10
	pathMapDepth = 4
	memoryCap    = 16 << 10
	memoryTotal  = 32 << 10
	memoryRead   = 8 << 20
	maxDecisions = 20
	cutNote      = "\ntruncated: true"
)

// memoryNames are the repository files that tell an agent how the code builds
// and tests (RFC-082). They go in the pack as data.
var memoryNames = []string{"AGENTS.md", "CLAUDE.md"}

// pathMap lists the worktree's files to pathMapDepth levels, skipping what
// list_files skips. A directory at the last level shows as dir/ (N files).
// The depth drops until the map fits pathMapCap, and a map that still does
// not fit is cut.
func pathMap(root string) (string, error) {
	var text string
	for depth := pathMapDepth; depth >= 1; depth-- {
		var err error
		if text, err = renderMap(root, depth); err != nil {
			return "", err
		}
		if len(text) <= pathMapCap {
			return text, nil
		}
	}
	cut := text[:pathMapCap-len(cutNote)]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return cut + cutNote, nil
}

func renderMap(root string, depth int) (string, error) {
	var lines []string
	err := filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if full == root {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if hiddenName(d.Name()) || (d.IsDir() && skipped[d.Name()]) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if strings.Count(rel, "/")+1 == depth {
				n := 0
				if err := walkFiles(root, full, 0, func(string, string) error { n++; return nil }); err != nil {
					return err
				}
				lines = append(lines, fmt.Sprintf("%s/ (%d files)", rel, n))
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			lines = append(lines, rel)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(lines) == 0 {
		return "no files", nil
	}
	return strings.Join(lines, "\n"), nil
}

// repoMemory reads AGENTS.md and CLAUDE.md from the worktree root and from
// each existing directory that texts name, each cut at memoryCap and all
// within memoryTotal. refs name each file by path, bytes, and sha256.
func repoMemory(root string, texts []string) ([]contextpack.File, []map[string]any, error) {
	var files []contextpack.File
	refs := []map[string]any{}
	left := memoryTotal
	for _, dir := range append([]string{"."}, namedDirs(root, texts)...) {
		for _, name := range memoryNames {
			rel := path.Join(dir, name)
			data, ok, err := readMemory(root, rel)
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				continue
			}
			limit := min(memoryCap, left)
			if limit <= len(cutNote) {
				return files, refs, nil
			}
			text := string(data)
			if len(text) > limit {
				text = strings.ToValidUTF8(text[:limit-len(cutNote)], "") + cutNote
			}
			left -= len(text)
			files = append(files, contextpack.File{Path: rel, Text: text})
			refs = append(refs, map[string]any{"path": rel, "bytes": len(data), "sha256": hashText(string(data))})
		}
	}
	return files, refs, nil
}

// readMemory reads one regular file through SafePath. A missing file, a
// symlink, or a file over memoryRead is skipped.
func readMemory(root, rel string) ([]byte, bool, error) {
	full, err := runtime.SafePath(root, rel)
	if err != nil {
		return nil, false, nil
	}
	info, err := os.Lstat(full)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() || info.Size() > memoryRead {
		return nil, false, nil
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, false, err
	}
	return data, true, nil
}

var pathToken = regexp.MustCompile(`[A-Za-z0-9_.\-/]+`)

// namedDirs are the existing directories below root that a path in texts
// names: the directory itself, or the directory of a named file. Order is
// first mention, without the root.
func namedDirs(root string, texts []string) []string {
	seen := map[string]bool{".": true}
	var out []string
	for _, text := range texts {
		for _, token := range pathToken.FindAllString(text, -1) {
			token = strings.TrimRight(strings.TrimPrefix(token, "./"), "./")
			if token == "" || strings.HasPrefix(token, "..") {
				continue
			}
			full, err := runtime.SafePath(root, token)
			if err != nil {
				continue
			}
			info, err := os.Lstat(full)
			if err != nil {
				continue
			}
			dir := token
			if info.Mode().IsRegular() {
				dir = path.Dir(token)
			} else if !info.IsDir() {
				continue
			}
			dir = path.Clean(dir)
			if !seen[dir] {
				seen[dir] = true
				out = append(out, dir)
			}
		}
	}
	return out
}

// requirementBlocks render each requirement as its heading, its statement,
// and its acceptance lines (RFC-082).
func requirementBlocks(ctx context.Context, db *sql.DB, projectID string, ids []string) ([]string, error) {
	var out []string
	for _, id := range ids {
		var heading, statement, acceptanceJSON string
		err := db.QueryRowContext(ctx, `
			SELECT heading, statement, acceptance_json FROM requirements WHERE id = $1 AND project_id = $2`,
			id, projectID).Scan(&heading, &statement, &acceptanceJSON)
		if err != nil {
			return nil, err
		}
		title := strings.TrimSpace(heading)
		if !strings.HasPrefix(title, id) {
			title = strings.TrimSpace(id + " " + title)
		}
		block := title + "\n" + strings.TrimSpace(statement)
		var acceptance []string
		_ = json.Unmarshal([]byte(acceptanceJSON), &acceptance)
		if len(acceptance) > 0 {
			block += "\nacceptance:\n- " + strings.Join(acceptance, "\n- ")
		}
		out = append(out, block)
	}
	return out, nil
}

// decisionLines are the project's decisions whose statement names one of
// ids, as "RFC-x: statement", up to maxDecisions.
func decisionLines(ctx context.Context, db *sql.DB, projectID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var names []string
	for _, id := range ids {
		names = append(names, regexp.QuoteMeta(id))
	}
	named := regexp.MustCompile(`(^|[^A-Za-z0-9-])(` + strings.Join(names, "|") + `)([^0-9]|$)`)
	rows, err := db.QueryContext(ctx, `SELECT id, statement FROM decisions WHERE project_id = $1 ORDER BY id`, projectID)
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
		if named.MatchString(statement) && len(out) < maxDecisions {
			out = append(out, id+": "+strings.TrimSpace(statement))
		}
	}
	return out, rows.Err()
}
