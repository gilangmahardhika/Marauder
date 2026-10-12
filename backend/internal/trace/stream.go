package trace

import (
	"strings"
	"unicode/utf8"
)

// StreamRedactor passes streamed text on with secrets replaced, as Clip
// cleans stored text. It holds back the last len(longest secret)-1 bytes, and
// any secret that starts before the cut, so a secret split across pieces is
// redacted whole before it is sent (RFC-087, RFC-088).
type StreamRedactor struct {
	secrets []string
	hold    int
	pending string
	send    func(string)
}

func NewStreamRedactor(secrets []string, send func(string)) *StreamRedactor {
	r := &StreamRedactor{send: send}
	for _, secret := range secrets {
		if secret != "" {
			r.secrets = append(r.secrets, secret)
			r.hold = max(r.hold, len(secret)-1)
		}
	}
	return r
}

// Write adds text and sends what can no longer be part of a secret.
func (r *StreamRedactor) Write(text string) {
	r.pending += strings.ReplaceAll(text, "\x00", "")
	cut := len(r.pending) - r.hold
	for moved := true; moved && cut > 0; {
		moved = false
		for _, secret := range r.secrets {
			from := max(0, cut-len(secret)+1)
			if i := strings.Index(r.pending[from:], secret); i >= 0 && from+i < cut {
				cut = from + i
				moved = true
			}
		}
	}
	for cut > 0 && cut < len(r.pending) && !utf8.RuneStart(r.pending[cut]) {
		cut--
	}
	if cut <= 0 {
		return
	}
	r.emit(r.pending[:cut])
	r.pending = r.pending[cut:]
}

// Flush sends the held-back text, at the end of a reply.
func (r *StreamRedactor) Flush() {
	if r.pending != "" {
		r.emit(r.pending)
		r.pending = ""
	}
}

// Reset drops the held-back text, when a reply starts again.
func (r *StreamRedactor) Reset() {
	r.pending = ""
}

func (r *StreamRedactor) emit(text string) {
	if out := Redact(strings.ToValidUTF8(text, "\uFFFD"), r.secrets); out != "" {
		r.send(out)
	}
}
