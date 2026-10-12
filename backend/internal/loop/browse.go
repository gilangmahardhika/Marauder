package loop

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"orchestrator/backend/internal/runtime"
)

const (
	listCap       = 500
	searchCap     = 100
	searchFileCap = 1 << 20
	searchLineCap = 200
	contextCap    = 5
)

var errListFull = errors.New("list is full")

// skipped are directories list_files and search_files do not enter below
// their start, besides .git and .orchestrator, which they never enter.
var skipped = map[string]bool{"node_modules": true, "vendor": true, "dist": true}

// listFiles returns the regular files under dir, relative to root, in walk
// order. glob filters the paths, and depth above zero limits how many
// directories deep below dir it goes. It never enters .git or follows a symlink.
func listFiles(root, dir, glob string, depth int) (string, error) {
	if dir == "" {
		dir = "."
	}
	start, err := runtime.SafePath(root, dir)
	if err != nil {
		return "", err
	}
	match, err := globMatcher(glob)
	if err != nil {
		return "", err
	}
	var out []string
	truncated := false
	err = walkFiles(root, start, depth, func(rel string, _ string) error {
		if !match(rel) {
			return nil
		}
		if len(out) == listCap {
			truncated = true
			return errListFull
		}
		out = append(out, rel)
		return nil
	})
	if err != nil && !errors.Is(err, errListFull) {
		return "", err
	}
	if len(out) == 0 {
		return "no files", nil
	}
	text := strings.Join(out, "\n")
	if truncated {
		text += "\ntruncated: true"
	}
	return text, nil
}

type searchOpts struct {
	Text       string
	Regex      bool
	Path       string
	Glob       string
	IgnoreCase bool
	Context    int
}

// searchFiles returns path:line: text for each matching line under the
// search path, with path-line- text for context lines. Binary files and files
// over 1 MiB are skipped.
func searchFiles(root string, opts searchOpts) (string, error) {
	if opts.Text == "" {
		return "", fmt.Errorf("text is empty")
	}
	pattern := opts.Text
	if !opts.Regex {
		pattern = regexp.QuoteMeta(pattern)
	}
	if opts.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("regex: %v", err)
	}
	dir := opts.Path
	if dir == "" {
		dir = "."
	}
	start, err := runtime.SafePath(root, dir)
	if err != nil {
		return "", err
	}
	match, err := globMatcher(opts.Glob)
	if err != nil {
		return "", err
	}
	around := min(max(opts.Context, 0), contextCap)
	var out []string
	matches := 0
	truncated := false
	err = walkFiles(root, start, 0, func(rel, full string) error {
		if !match(rel) {
			return nil
		}
		info, err := os.Stat(full)
		if err != nil || info.Size() > searchFileCap {
			return nil
		}
		body, err := os.ReadFile(full)
		if err != nil || bytes.IndexByte(body, 0) >= 0 || !re.Match(body) {
			return nil
		}
		lines := bytes.Split(body, []byte("\n"))
		shown := -1
		for i, line := range lines {
			if !re.Match(line) {
				continue
			}
			if matches == searchCap {
				truncated = true
				return errListFull
			}
			matches++
			for j := max(i-around, shown+1); j < i; j++ {
				out = append(out, searchLine(rel, j, "-", lines[j]))
			}
			out = append(out, searchLine(rel, i, ":", line))
			shown = i
			for j := i + 1; j <= i+around && j < len(lines); j++ {
				if re.Match(lines[j]) {
					break
				}
				out = append(out, searchLine(rel, j, "-", lines[j]))
				shown = j
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, errListFull) {
		return "", err
	}
	if matches == 0 {
		return "no matches for " + opts.Text, nil
	}
	result := strings.Join(out, "\n")
	if truncated {
		result += "\ntruncated: true"
	}
	return result, nil
}

func searchLine(rel string, i int, sep string, line []byte) string {
	if len(line) > searchLineCap {
		line = line[:searchLineCap]
	}
	return fmt.Sprintf("%s%s%d%s %s", rel, sep, i+1, sep, strings.TrimSpace(strings.ToValidUTF8(string(line), "")))
}

// walkFiles visits the regular files under start. depth above zero limits how
// many directories deep below start it goes.
func walkFiles(root, start string, depth int, visit func(rel, full string) error) error {
	return filepath.WalkDir(start, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if hiddenName(d.Name()) {
				return filepath.SkipDir
			}
			if full != start && skipped[d.Name()] {
				return filepath.SkipDir
			}
			if depth > 0 && full != start && levels(start, full) >= depth {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || hiddenName(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		return visit(filepath.ToSlash(rel), full)
	})
}

// hiddenName is never listed: .git, which is a file in a linked worktree, and
// .orchestrator.
func hiddenName(name string) bool {
	return name == ".git" || name == ".orchestrator"
}

func levels(start, full string) int {
	rel, err := filepath.Rel(start, full)
	if err != nil {
		return 0
	}
	return strings.Count(filepath.ToSlash(rel), "/") + 1
}

// globMatcher matches a repository-relative path. A glob without a slash
// matches the file name; ** matches any number of directories.
func globMatcher(glob string) (func(string) bool, error) {
	if glob == "" {
		return func(string) bool { return true }, nil
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(glob); i++ {
		c := glob[i]
		switch {
		case c == '*' && i+1 < len(glob) && glob[i+1] == '*':
			i++
			if i+1 < len(glob) && glob[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			end := strings.IndexByte(glob[i:], '}')
			if end < 0 {
				b.WriteString(regexp.QuoteMeta(string(c)))
				continue
			}
			var alts []string
			for _, alt := range strings.Split(glob[i+1:i+end], ",") {
				alts = append(alts, regexp.QuoteMeta(alt))
			}
			b.WriteString("(?:")
			b.WriteString(strings.Join(alts, "|"))
			b.WriteString(")")
			i += end
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		return nil, fmt.Errorf("glob: %v", err)
	}
	base := !strings.Contains(glob, "/")
	return func(rel string) bool {
		if base {
			return re.MatchString(filepath.Base(rel))
		}
		return re.MatchString(rel)
	}, nil
}
