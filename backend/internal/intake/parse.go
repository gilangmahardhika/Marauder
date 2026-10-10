package intake

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

var idPattern = regexp.MustCompile(`\b((?:REQ|RFC)-[0-9]+)\b`)

type Requirement struct {
	ID         string
	Heading    string
	Statement  string
	Acceptance []string
	SourcePath string
	SourceKind string
	Hash       string
}

type Decision struct {
	ID         string
	Statement  string
	SourcePath string
	Hash       string
}

type Result struct {
	Requirements []Requirement
	Decisions    []Decision
	DraftPending bool
}

type section struct {
	heading  string
	level    int
	text     string
	items    []string
	children []*section
}

func Parse(repo, prdDir, rfcDir string) (Result, error) {
	var result Result
	prdFiles, err := markdownFiles(prdDir)
	if err != nil {
		return Result{}, err
	}
	sawPRD := false
	missingIDs := false
	for _, path := range prdFiles {
		sawPRD = true
		reqs, decs, sawReq, err := parseFile(repo, path, "prd")
		if err != nil {
			return Result{}, err
		}
		if !sawReq {
			missingIDs = true
		}
		result.Requirements = append(result.Requirements, reqs...)
		result.Decisions = append(result.Decisions, decs...)
	}
	result.DraftPending = sawPRD && missingIDs

	rfcFiles, err := markdownFiles(rfcDir)
	if err != nil {
		return Result{}, err
	}
	for _, path := range rfcFiles {
		reqs, decs, _, err := parseFile(repo, path, "rfc")
		if err != nil {
			return Result{}, err
		}
		result.Requirements = append(result.Requirements, reqs...)
		result.Decisions = append(result.Decisions, decs...)
	}
	return result, nil
}

func parseFile(repo, path, sourceKind string) ([]Requirement, []Decision, bool, error) {
	source, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false, err
	}
	rel, err := filepath.Rel(repo, path)
	if err != nil {
		return nil, nil, false, err
	}
	sourcePath := filepath.ToSlash(rel)
	root := parseSections(source)
	var reqs []Requirement
	var decs []Decision
	sawReq := false
	var walk func(*section)
	walk = func(n *section) {
		if n.heading != "" {
			if id, kind, ok := headingID(n.heading); ok {
				if kind == "REQ" {
					sawReq = true
					acceptance := acceptanceItems(n)
					statement := statementBeforeAcceptance(n)
					reqs = append(reqs, Requirement{
						ID:         id,
						Heading:    strings.TrimSpace(n.heading),
						Statement:  statement,
						Acceptance: acceptance,
						SourcePath: sourcePath,
						SourceKind: sourceKind,
						Hash:       hashParts(statement, acceptance),
					})
				} else {
					statement := sectionText(n)
					decs = append(decs, Decision{
						ID:         id,
						Statement:  statement,
						SourcePath: sourcePath,
						Hash:       hashText(statement),
					})
				}
			}
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(root)
	return reqs, decs, sawReq, nil
}

func parseSections(source []byte) *section {
	root := &section{}
	stack := []*section{root}
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if heading, ok := n.(*ast.Heading); ok {
			level := heading.Level
			for len(stack) > 1 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			child := &section{heading: plainText(source, heading), level: level}
			stack[len(stack)-1].children = append(stack[len(stack)-1].children, child)
			stack = append(stack, child)
			continue
		}
		cur := stack[len(stack)-1]
		switch block := n.(type) {
		case *ast.List:
			cur.children = append(cur.children, &section{items: listItems(source, block)})
		case *ast.FencedCodeBlock:
			cur.children = append(cur.children, &section{text: codeText(source, block.Lines())})
		case *ast.CodeBlock:
			cur.children = append(cur.children, &section{text: codeText(source, block.Lines())})
		default:
			if text := plainText(source, n); text != "" {
				cur.children = append(cur.children, &section{text: text})
			}
		}
	}
	return root
}

func statementBeforeAcceptance(n *section) string {
	var parts []string
	var walk func(*section, bool) bool
	walk = func(c *section, top bool) bool {
		if !top && c.heading != "" {
			if c.heading == "Acceptance" {
				return false
			}
			if _, _, ok := headingID(c.heading); ok {
				return true
			}
			parts = append(parts, c.heading)
		}
		if len(c.items) > 0 {
			parts = append(parts, c.items...)
		} else if c.text != "" {
			parts = append(parts, c.text)
		}
		for _, child := range c.children {
			if child.heading == "Acceptance" {
				return false
			}
			if !walk(child, false) {
				return false
			}
		}
		return true
	}
	walk(n, true)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func acceptanceItems(n *section) []string {
	found := findAcceptance(n)
	if found == nil {
		return []string{}
	}
	var items []string
	var walk func(*section)
	walk = func(c *section) {
		if len(c.items) > 0 {
			items = append(items, c.items...)
		}
		for _, child := range c.children {
			walk(child)
		}
	}
	walk(found)
	if items == nil {
		return []string{}
	}
	return items
}

func findAcceptance(n *section) *section {
	for _, child := range n.children {
		if child.heading == "Acceptance" {
			return child
		}
		if _, _, ok := headingID(child.heading); ok {
			continue
		}
		if found := findAcceptance(child); found != nil {
			return found
		}
	}
	return nil
}

func sectionText(n *section) string {
	var parts []string
	var walk func(*section, bool)
	walk = func(c *section, top bool) {
		if !top && c.heading != "" {
			if _, _, ok := headingID(c.heading); ok {
				return
			}
			parts = append(parts, c.heading)
		}
		if len(c.items) > 0 {
			parts = append(parts, c.items...)
		} else if c.text != "" {
			parts = append(parts, c.text)
		}
		for _, child := range c.children {
			if !top && child.heading != "" {
				if _, _, ok := headingID(child.heading); ok {
					continue
				}
			}
			walk(child, false)
		}
	}
	walk(n, true)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func headingID(heading string) (string, string, bool) {
	match := idPattern.FindStringSubmatch(heading)
	if match == nil {
		return "", "", false
	}
	id := match[1]
	kind := "REQ"
	if strings.HasPrefix(id, "RFC-") {
		kind = "RFC"
	}
	return id, kind, true
}

func hashParts(statement string, acceptance []string) string {
	return hashText(statement + "\n" + strings.Join(acceptance, "\n"))
}

func hashText(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func markdownFiles(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, os.ErrInvalid
	}
	var files []string
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func plainText(source []byte, n ast.Node) string {
	var b strings.Builder
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		textNode, ok := node.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		b.Write(textNode.Segment.Value(source))
		if textNode.SoftLineBreak() {
			b.WriteByte(' ')
		}
		if textNode.HardLineBreak() {
			b.WriteByte('\n')
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

func listItems(source []byte, list *ast.List) []string {
	var items []string
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if text := plainText(source, item); text != "" {
			items = append(items, text)
		}
	}
	return items
}

func codeText(source []byte, lines *text.Segments) string {
	if lines == nil {
		return ""
	}
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		b.Write(segment.Value(source))
	}
	return strings.TrimSpace(b.String())
}
