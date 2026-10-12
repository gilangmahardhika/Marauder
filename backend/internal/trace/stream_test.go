package trace_test

import (
	"strings"
	"testing"

	"orchestrator/backend/internal/trace"
)

func TestStreamRedactorEverySplit(t *testing.T) {
	secrets := []string{"sk-short-1", "sk-ant-longer-key-0123456789", "postgres://orch:pw-0123@127.0.0.1:5432/orch"}
	text := "a " + secrets[1] + " b " + secrets[0] + secrets[1] + " é\x00 " + secrets[2] + " end"
	want := strings.NewReplacer(secrets[2], "[redacted]", secrets[1], "[redacted]", secrets[0], "[redacted]", "\x00", "").Replace(text)
	for i := 0; i <= len(text); i++ {
		for j := i; j <= len(text); j++ {
			var sent []string
			r := trace.NewStreamRedactor(secrets, func(s string) { sent = append(sent, s) })
			r.Write(text[:i])
			r.Write(text[i:j])
			r.Write(text[j:])
			r.Flush()
			if got := strings.Join(sent, ""); got != want {
				t.Fatalf("split %d,%d: got %q", i, j, got)
			}
			for _, piece := range sent {
				for _, secret := range secrets {
					if strings.Contains(piece, secret[:4+len(secret)/2]) {
						t.Fatalf("split %d,%d: piece %q carries part of a secret", i, j, piece)
					}
				}
			}
		}
	}
}

func TestStreamRedactorReset(t *testing.T) {
	var sent []string
	r := trace.NewStreamRedactor([]string{"sk-ant-longer-key-0123456789"}, func(s string) { sent = append(sent, s) })
	r.Write("partial sk-ant-lon")
	r.Reset()
	r.Write("fresh reply")
	r.Flush()
	if got := strings.Join(sent, ""); got != "fresh reply" {
		t.Fatalf("got %q", got)
	}
}
