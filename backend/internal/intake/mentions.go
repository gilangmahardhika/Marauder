package intake

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// MaxMentions caps the mentions kept for one requirement (RFC-101).
const MaxMentions = 16

const (
	mentionBefore = "([{\"',;:"
	mentionAfter  = ")]}\"',;:!?"
	// mentionGap stands for text that is not read, such as inline code. It
	// is neither a space nor an allowed mark, so a mention cannot touch it.
	mentionGap = '\x00'
)

// Mentions returns the @slug mentions of one markdown text in order of first
// use, without repeats, at most MaxMentions. Text in inline code, code
// blocks, links, autolinks, images, and raw HTML is not read (RFC-101).
func Mentions(markdown string) []string {
	source := []byte(markdown)
	doc := goldmark.New().Parser().Parse(text.NewReader(source))
	return addMentions(nil, nodeMentions(source, doc))
}

// MentionsOf is the union of the mentions of a statement and its acceptance
// lines, each line read as its own markdown.
func MentionsOf(statement string, acceptance []string) []string {
	out := addMentions(nil, Mentions(statement))
	for _, line := range acceptance {
		out = addMentions(out, Mentions(line))
	}
	return out
}

// FindMentions returns the byte ranges of the mentions in plain text, as
// [start, end) pairs that cover the @ and the slug.
func FindMentions(s string) [][2]int {
	var out [][2]int
	for i := 0; i < len(s); i++ {
		if s[i] != '@' {
			continue
		}
		if i > 0 {
			prev, _ := utf8.DecodeLastRuneInString(s[:i])
			if !unicode.IsSpace(prev) && !strings.ContainsRune(mentionBefore, prev) {
				continue
			}
		}
		end := i + 1
		for end < len(s) && (isLower(s[end]) || (end > i+1 && isDigit(s[end]))) {
			end++
		}
		n := end - i - 1
		if n < 2 || n > 16 {
			i = end - 1
			continue
		}
		if end < len(s) && !mentionEnds(s[end:]) {
			i = end - 1
			continue
		}
		out = append(out, [2]int{i, end})
		i = end - 1
	}
	return out
}

func mentionEnds(rest string) bool {
	r, size := utf8.DecodeRuneInString(rest)
	if unicode.IsSpace(r) || strings.ContainsRune(mentionAfter, r) {
		return true
	}
	if r != '.' {
		return false
	}
	next, _ := utf8.DecodeRuneInString(rest[size:])
	return len(rest) == size || !(unicode.IsLetter(next) || unicode.IsDigit(next))
}

func isLower(b byte) bool { return b >= 'a' && b <= 'z' }
func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// nodeMentions reads the mentions of the text under n.
func nodeMentions(source []byte, n ast.Node) []string {
	var b strings.Builder
	_ = ast.Walk(n, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.CodeSpan, *ast.Link, *ast.AutoLink, *ast.Image, *ast.RawHTML:
			b.WriteRune(mentionGap)
			return ast.WalkSkipChildren, nil
		case *ast.FencedCodeBlock, *ast.CodeBlock, *ast.HTMLBlock:
			b.WriteByte('\n')
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			b.Write(node.Segment.Value(source))
			if node.SoftLineBreak() || node.HardLineBreak() {
				b.WriteByte('\n')
			}
		case *ast.String:
			b.Write(node.Value)
		default:
			if node.Type() == ast.TypeBlock {
				b.WriteByte('\n')
			}
		}
		return ast.WalkContinue, nil
	})
	s := b.String()
	var out []string
	for _, span := range FindMentions(s) {
		out = addMentions(out, []string{s[span[0]+1 : span[1]]})
	}
	return out
}

func addMentions(list, more []string) []string {
	for _, slug := range more {
		if len(list) >= MaxMentions {
			break
		}
		seen := false
		for _, have := range list {
			if have == slug {
				seen = true
				break
			}
		}
		if !seen {
			list = append(list, slug)
		}
	}
	if list == nil {
		return []string{}
	}
	return list
}

// hashRequirement is the content hash of a requirement. The mentions join it
// only when there are any, so a requirement with none keeps its old hash.
func hashRequirement(statement string, acceptance, mentions []string) string {
	if len(mentions) == 0 {
		return hashParts(statement, acceptance)
	}
	return hashText(statement + "\n" + strings.Join(acceptance, "\n") + "\nmentions:" + strings.Join(mentions, ","))
}
