package intake_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"orchestrator/backend/internal/intake"
)

func TestMentionGrammar(t *testing.T) {
	cases := []struct {
		text string
		want []string
	}{
		{"Read tickets with @tickets.", []string{"tickets"}},
		{"Use (@db) and @db again, then @docs!", []string{"db", "docs"}},
		{"@first at the start", []string{"first"}},
		{"Ends with @db.", []string{"db"}},
		{"Mail a@b.co or ada@example.com", []string{}},
		{"Install @scope/pkg and @sveltejs/kit", []string{}},
		{"Use @pytest.fixture and @Override", []string{}},
		{"Too short @x, too long @abcdefghijklmnopq", []string{}},
		{"Not @db_x or @db-x", []string{}},
		{"Code `@tickets` stays code", []string{}},
		{"```\n@tickets\n```\n", []string{}},
		{"    @tickets in an indented block\n", []string{}},
		{"[@tickets](https://example.com) is a link", []string{}},
		{"<https://example.com/@tickets> is an autolink", []string{}},
		{"Raw <span>@tickets</span> html", []string{}},
		{"**@tickets** is bold", []string{"tickets"}},
		{"line one\n@tickets on line two", []string{"tickets"}},
	}
	for _, tc := range cases {
		if got := intake.Mentions(tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Mentions(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}

	var many []string
	for i := 0; i < 20; i++ {
		many = append(many, fmt.Sprintf("@s%02d", i))
	}
	if got := intake.Mentions(strings.Join(many, " ")); len(got) != intake.MaxMentions || got[0] != "s00" || got[15] != "s15" {
		t.Fatalf("20 mentions kept %q", got)
	}
	if got := intake.MentionsOf("Uses @docs", []string{"Checks @tickets", "Again @docs"}); !reflect.DeepEqual(got, []string{"docs", "tickets"}) {
		t.Fatalf("MentionsOf = %q", got)
	}
}

func TestHashUnchangedWithoutMentions(t *testing.T) {
	repo := t.TempDir()
	prd := filepath.Join(repo, "docs", "prd", "one.md")
	if err := os.MkdirAll(filepath.Dir(prd), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "### REQ-001 Plain\n\nNo mention here, mail a@b.co.\n\n#### Acceptance\n\n- One check.\n\n### REQ-002 Mention\n\nRead @tickets.\n"
	if err := os.WriteFile(prd, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := intake.Parse(repo, filepath.Join(repo, "docs", "prd"), filepath.Join(repo, "docs", "rfc"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Requirements) != 2 {
		t.Fatalf("requirements = %+v", result.Requirements)
	}
	old := func(statement string, acceptance []string) string {
		sum := sha256.Sum256([]byte(statement + "\n" + strings.Join(acceptance, "\n")))
		return hex.EncodeToString(sum[:])
	}
	plain, mention := result.Requirements[0], result.Requirements[1]
	if len(plain.Mentions) != 0 || plain.Hash != old(plain.Statement, plain.Acceptance) {
		t.Fatalf("plain = %+v", plain)
	}
	if !reflect.DeepEqual(mention.Mentions, []string{"tickets"}) || mention.Hash == old(mention.Statement, mention.Acceptance) {
		t.Fatalf("mention = %+v", mention)
	}
}
