package app

import (
	"strings"
	"testing"
)

func TestRedactorEverySplit(t *testing.T) {
	keys := []string{"sk-short-1", "sk-ant-longer-key-0123456789"}
	text := "a " + keys[1] + " b " + keys[0] + keys[1] + " é end"
	want := strings.NewReplacer(keys[1], "[redacted]", keys[0], "[redacted]").Replace(text)
	for i := 0; i <= len(text); i++ {
		for j := i; j <= len(text); j++ {
			var sent []string
			r := newRedactor(keys, func(s string) { sent = append(sent, s) })
			r.write(text[:i])
			r.write(text[i:j])
			r.write(text[j:])
			r.flush()
			got := strings.Join(sent, "")
			if got != want {
				t.Fatalf("split %d,%d: got %q", i, j, got)
			}
			for _, piece := range sent {
				for _, key := range keys {
					if strings.Contains(piece, key[:4+len(key)/2]) {
						t.Fatalf("split %d,%d: piece %q carries part of a key", i, j, piece)
					}
				}
			}
		}
	}
}
