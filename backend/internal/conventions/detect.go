package conventions

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	detectDepth = 2
	maxManifest = 1 << 20
)

var (
	skipDirs = map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "out": true,
		"target": true, ".svelte-kit": true, ".next": true, "__pycache__": true, ".venv": true, "venv": true,
	}
	pyName    = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)`)
	pyQuoted  = regexp.MustCompile(`["']([A-Za-z0-9][A-Za-z0-9._-]*)`)
	tomlKey   = regexp.MustCompile(`^\s*([A-Za-z0-9][A-Za-z0-9._-]*)\s*=`)
	tomlTable = regexp.MustCompile(`^\s*\[+([^\]]+)\]+\s*$`)
)

// Detect returns the ids of the stacks whose marker files exist or whose npm or
// Python dependencies are named in the repo root or two directory levels below
// it. Links are not followed and dependency folders and build output are
// skipped.
func Detect(root string, stacks []Stack) ([]string, error) {
	files := map[string]bool{}
	npm := map[string]bool{}
	py := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, string(filepath.Separator)) + 1
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || depth > detectDepth) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		files[name] = true
		switch {
		case name == "package.json":
			readNpm(path, npm)
		case name == "pyproject.toml":
			readPyproject(path, py)
		case strings.HasPrefix(name, "requirements") && strings.HasSuffix(name, ".txt"):
			readRequirements(path, py)
		}
		return nil
	})
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, s := range stacks {
		if matches(s.Detect, files, npm, py) {
			out = append(out, s.ID)
		}
	}
	sort.Strings(out)
	return out, nil
}

func matches(d Markers, files, npm, py map[string]bool) bool {
	for _, pattern := range d.Files {
		for name := range files {
			if ok, _ := filepath.Match(pattern, name); ok {
				return true
			}
		}
	}
	for _, dep := range d.NpmDeps {
		if npm[dep] {
			return true
		}
	}
	for _, dep := range d.PyDeps {
		if py[normalizePy(dep)] {
			return true
		}
	}
	return false
}

func normalizePy(name string) string {
	return strings.ReplaceAll(strings.ReplaceAll(strings.ToLower(name), "_", "-"), ".", "-")
}

func readLimited(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, maxManifest))
}

func readNpm(path string, out map[string]bool) {
	raw, err := readLimited(path)
	if err != nil {
		return
	}
	var pkg struct {
		Dependencies     map[string]any `json:"dependencies"`
		DevDependencies  map[string]any `json:"devDependencies"`
		PeerDependencies map[string]any `json:"peerDependencies"`
	}
	if json.Unmarshal(raw, &pkg) != nil {
		return
	}
	for _, deps := range []map[string]any{pkg.Dependencies, pkg.DevDependencies, pkg.PeerDependencies} {
		for name := range deps {
			out[name] = true
		}
	}
}

func readRequirements(path string, out map[string]bool) {
	raw, err := readLimited(path)
	if err != nil {
		return
	}
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if m := pyName.FindStringSubmatch(line); m != nil {
			out[normalizePy(m[1])] = true
		}
	}
}

// readPyproject reads the [project] dependencies list and the keys of any
// poetry dependencies table. It is a line reader, not a TOML parser.
func readPyproject(path string, out map[string]bool) {
	raw, err := readLimited(path)
	if err != nil {
		return
	}
	section := ""
	inList := false
	sc := bufio.NewScanner(strings.NewReader(string(raw)))
	for sc.Scan() {
		line := sc.Text()
		if m := tomlTable.FindStringSubmatch(line); m != nil {
			section = strings.TrimSpace(m[1])
			inList = false
			continue
		}
		if inList {
			for _, m := range pyQuoted.FindAllStringSubmatch(line, -1) {
				out[normalizePy(m[1])] = true
			}
			if strings.Contains(line, "]") {
				inList = false
			}
			continue
		}
		switch {
		case section == "project" || strings.HasPrefix(section, "project.optional-dependencies"):
			key := tomlKey.FindStringSubmatch(line)
			if key == nil {
				continue
			}
			if section == "project" && key[1] != "dependencies" {
				continue
			}
			rest := line[strings.Index(line, "=")+1:]
			if !strings.Contains(rest, "[") {
				continue
			}
			for _, m := range pyQuoted.FindAllStringSubmatch(rest, -1) {
				out[normalizePy(m[1])] = true
			}
			inList = !strings.Contains(rest, "]")
		case strings.HasPrefix(section, "tool.poetry") && strings.HasSuffix(section, "dependencies"):
			if m := tomlKey.FindStringSubmatch(line); m != nil && m[1] != "python" {
				out[normalizePy(m[1])] = true
			}
		}
	}
}
