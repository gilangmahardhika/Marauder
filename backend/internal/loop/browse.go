package loop

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/backend/internal/runtime"
)

const (
	listCap       = 500
	searchCap     = 100
	searchFileCap = 1 << 20
	searchLineCap = 200
)

var errListFull = errors.New("list is full")

// listFiles returns the regular files under dir, relative to root, in walk
// order. It never enters .git or follows a symlink.
func listFiles(root, dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	start, err := runtime.SafePath(root, dir)
	if err != nil {
		return "", err
	}
	var out []string
	truncated := false
	err = walkFiles(root, start, func(rel string, _ string) error {
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

// searchFiles returns path:line: text for each line under root that contains
// text. Binary files and files over 1 MiB are skipped.
func searchFiles(root, text string) (string, error) {
	if text == "" {
		return "", fmt.Errorf("text is empty")
	}
	start, err := runtime.SafePath(root, ".")
	if err != nil {
		return "", err
	}
	needle := []byte(text)
	var out []string
	truncated := false
	err = walkFiles(root, start, func(rel, full string) error {
		info, err := os.Stat(full)
		if err != nil || info.Size() > searchFileCap {
			return nil
		}
		body, err := os.ReadFile(full)
		if err != nil || bytes.IndexByte(body, 0) >= 0 || !bytes.Contains(body, needle) {
			return nil
		}
		for i, line := range bytes.Split(body, []byte("\n")) {
			if !bytes.Contains(line, needle) {
				continue
			}
			if len(out) == searchCap {
				truncated = true
				return errListFull
			}
			if len(line) > searchLineCap {
				line = line[:searchLineCap]
			}
			out = append(out, fmt.Sprintf("%s:%d: %s", rel, i+1, strings.TrimSpace(string(line))))
		}
		return nil
	})
	if err != nil && !errors.Is(err, errListFull) {
		return "", err
	}
	if len(out) == 0 {
		return "no matches", nil
	}
	result := strings.Join(out, "\n")
	if truncated {
		result += "\ntruncated: true"
	}
	return result, nil
}

func walkFiles(root, start string, visit func(rel, full string) error) error {
	return filepath.WalkDir(start, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" || d.Name() == ".orchestrator" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			return err
		}
		return visit(filepath.ToSlash(rel), full)
	})
}
