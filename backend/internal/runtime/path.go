package runtime

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrPath = errors.New("path escapes the root")

func SafePath(root, requested string) (string, error) {
	if requested == "" || strings.ContainsRune(requested, 0) || filepath.IsAbs(requested) {
		return "", ErrPath
	}
	cleaned := filepath.Clean(requested)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", ErrPath
	}
	root = filepath.Clean(root)
	full := filepath.Join(root, cleaned)
	rel, err := filepath.Rel(root, full)
	if err != nil || escapes(rel) {
		return "", ErrPath
	}
	if forbiddenRel(rel) {
		return "", ErrPath
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	ancestor := full
	var missing []string
	for {
		_, statErr := os.Lstat(ancestor)
		if statErr == nil {
			break
		}
		if !os.IsNotExist(statErr) {
			return "", statErr
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", ErrPath
		}
		missing = append([]string{filepath.Base(ancestor)}, missing...)
		ancestor = parent
	}
	resolved, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	for _, part := range missing {
		resolved = filepath.Join(resolved, part)
	}
	resolvedRel, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil || escapes(resolvedRel) {
		return "", ErrPath
	}
	return full, nil
}

func escapes(rel string) bool {
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func forbiddenRel(rel string) bool {
	return rel == ".git" || rel == ".orchestrator" ||
		strings.HasPrefix(rel, ".git"+string(filepath.Separator)) ||
		strings.HasPrefix(rel, ".orchestrator"+string(filepath.Separator))
}
