package contextpack

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	ConventionCap  = 16 << 10
	ConventionsCap = 64 << 10
)

// Convention is one company rule or skill a card's packs carry (RFC-055,
// RFC-056). Label is "global" or the stack name of a rule.
type Convention struct {
	Kind     string
	ID       string
	Version  int
	Hash     string
	Title    string
	Label    string
	Category string
	Slug     string
	UseWhen  string
	Body     string
}

// ConventionBlock renders one rule or skill, cut at ConventionCap.
func ConventionBlock(c Convention) string {
	var out string
	if c.Kind == "skill" {
		out = fmt.Sprintf("SKILL %s v%d: %s\nuse when: %s\n%s", c.Slug, c.Version, c.Title, c.UseWhen, c.Body)
	} else {
		out = fmt.Sprintf("CONVENTION %s (%s, %s) v%d\n%s", c.Title, c.Label, c.Category, c.Version, c.Body)
	}
	out = stripMarkers(strings.TrimRight(out, "\n"))
	if len(out) > ConventionCap {
		const note = "\ntruncated: true"
		out = cutRunes(out, ConventionCap-len(note)) + note
	}
	return out
}

// conventionsSection joins the blocks under ConventionsCap. A block that does
// not fit becomes an omitted note.
func conventionsSection(items []Convention) string {
	var blocks []string
	size := 0
	for _, c := range items {
		block := ConventionBlock(c)
		if size+len(block)+1 > ConventionsCap {
			name := "CONVENTION " + c.Title
			if c.Kind == "skill" {
				name = "SKILL " + c.Slug
			}
			block = stripMarkers(name + " omitted: the conventions section is full")
			if size+len(block)+1 > ConventionsCap {
				break
			}
		}
		blocks = append(blocks, block)
		size += len(block) + 1
	}
	return strings.Join(blocks, "\n")
}

func withConventions(untrusted string, items []Convention) string {
	section := conventionsSection(items)
	if section == "" {
		return untrusted
	}
	return section + "\n" + untrusted
}

func cutRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
