package loop

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"orchestrator/backend/internal/runtime"
)

const (
	// readFileCap is the largest file read_file opens. readCap bounds the text
	// of one result, so a long file is read in ranges.
	readFileCap  = 8 << 20
	readLines    = 2000
	readLineCap  = 2000
	writeCap     = 1 << 20
	placeholder1 = "(omitted: already written to the file)"
	placeholder2 = "(older result omitted)"
)

// readFile returns lines offset to offset+limit-1 of a file as N|text, then
// the range and the total line count, and where to read next when lines
// remain. offset counts from 1. A zero limit reads readLines lines.
func readFile(root, requested string, offset, limit int) (string, error) {
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("%s is a directory; use list_files", requested)
	}
	if info.Size() > readFileCap {
		return "", fmt.Errorf("%s is over 8 MiB", requested)
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	lines := splitLines(string(body))
	total := len(lines)
	if total == 0 {
		return "lines 0 of 0 (empty file)", nil
	}
	if offset < 1 {
		offset = 1
	}
	if offset > total {
		return "", fmt.Errorf("offset %d is past the end; %s has %d lines", offset, requested, total)
	}
	if limit <= 0 || limit > readLines {
		limit = readLines
	}
	var b strings.Builder
	last := offset - 1
	for i := offset - 1; i < total && i < offset-1+limit; i++ {
		line := lines[i]
		if len(line) > readLineCap {
			line = strings.ToValidUTF8(line[:readLineCap], "") + " [line cut]"
		}
		entry := fmt.Sprintf("%d|%s\n", i+1, line)
		if b.Len()+len(entry) > readCap && i > offset-1 {
			break
		}
		b.WriteString(entry)
		last = i + 1
	}
	fmt.Fprintf(&b, "lines %d-%d of %d", offset, last, total)
	if last < total {
		fmt.Fprintf(&b, "\nmore: read from line %d", last+1)
	}
	return b.String(), nil
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// replaceInFile replaces the exact text old with new. old must occur once
// unless all is set.
func replaceInFile(root, requested, old, replacement string, all bool) (string, error) {
	if old == "" {
		return "", fmt.Errorf("old is empty; use edit_file to write a whole file")
	}
	if old == replacement {
		return "", fmt.Errorf("old and new are the same")
	}
	if hasPlaceholder(replacement) {
		return "", fmt.Errorf("new holds a history placeholder, not file text; read the file and send its real text")
	}
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s is not a regular file", requested)
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return "", err
	}
	text := string(body)
	n := strings.Count(text, old)
	switch {
	case n == 0:
		return "", fmt.Errorf("old text not found in %s; read the file again and copy the text exactly, with its indentation", requested)
	case n > 1 && !all:
		return "", fmt.Errorf("old text occurs %d times in %s; add surrounding lines so it occurs once, or set replaceAll", n, requested)
	}
	count := 1
	if all {
		count = -1
	} else {
		n = 1
	}
	out := strings.Replace(text, old, replacement, count)
	if len(out) > writeCap {
		return "", fmt.Errorf("write exceeds 1 MiB")
	}
	if err := writeAtomic(full, []byte(out), info.Mode().Perm()); err != nil {
		return "", err
	}
	if n == 1 {
		return "replaced 1 occurrence in " + requested, nil
	}
	return fmt.Sprintf("replaced %d occurrences in %s", n, requested), nil
}

// peekFileText returns the current contents of a regular file under root, or
// ("", false) when the path is missing or not a regular file.
func peekFileText(root, requested string) (string, bool) {
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return "", false
	}
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	body, err := os.ReadFile(full)
	if err != nil {
		return "", false
	}
	return string(body), true
}

// editFile writes the whole file. It refuses a history placeholder and empty
// text over a file that is not empty, which are how a model loses a file.
func editFile(root, requested, text string) error {
	if len(text) > writeCap {
		return fmt.Errorf("write exceeds 1 MiB")
	}
	if hasPlaceholder(text) {
		return fmt.Errorf("text holds a history placeholder, not file text; read the file and use replace_in_file")
	}
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if info, err := os.Lstat(full); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%s is not a regular file", requested)
		}
		if text == "" && info.Size() > 0 {
			return fmt.Errorf("refusing to empty %s; use delete_file to remove a file", requested)
		}
		mode = info.Mode().Perm()
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return writeAtomic(full, []byte(text), mode)
}

func deleteFile(root, requested string) error {
	full, err := runtime.SafePath(root, requested)
	if err != nil {
		return err
	}
	info, err := os.Lstat(full)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", requested)
	}
	return os.Remove(full)
}

func moveFile(root, from, to string) error {
	src, err := runtime.SafePath(root, from)
	if err != nil {
		return err
	}
	dest, err := runtime.SafePath(root, to)
	if err != nil {
		return err
	}
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", from)
	}
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("%s already exists", to)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.Rename(src, dest)
}

// writeAtomic writes a temporary file beside full and renames it over full,
// so a failed write never leaves half a file.
func writeAtomic(full string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(full), ".orch-write-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, full); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

func hasPlaceholder(text string) bool {
	return strings.Contains(text, placeholder1) || strings.Contains(text, placeholder2)
}
