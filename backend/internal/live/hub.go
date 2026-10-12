// Package live fans out the model's reply text, as it is generated, to the
// Trace page viewers of a card (RFC-088). Nothing here is stored.
package live

import (
	"errors"
	"sync"
	"unicode/utf8"
)

// Frame kinds a loop publishes, and Resync, which ends a viewer that fell
// behind.
const (
	Turn    = "turn"
	Delta   = "delta"
	Reset   = "reset"
	TurnEnd = "turn_end"
	Resync  = "resync"
)

const (
	// MaxText caps one turn's live text; the rest of the reply arrives with
	// its stored event.
	MaxText = 64 << 10
	// Buffer is how many frames a viewer may fall behind before it is dropped.
	Buffer = 256
	// PerCard and Total cap the viewers.
	PerCard = 16
	Total   = 64
)

// ErrFull means the hub already has as many viewers as it allows.
var ErrFull = errors.New("too many trace viewers")

// Frame is one live update for a card. Turn carries Role and ModelID, Delta
// carries Text; every frame names its attempt and turn.
type Frame struct {
	Kind      string `json:"-"`
	AttemptID string `json:"attemptId"`
	Turn      int    `json:"turn"`
	Role      string `json:"role,omitempty"`
	ModelID   string `json:"modelId,omitempty"`
	Text      string `json:"text,omitempty"`
}

// Publisher is what a loop needs from the hub.
type Publisher interface {
	Publish(cardID string, f Frame)
}

type card struct {
	subs map[*Sub]struct{}
	turn *Frame
	text []byte
}

// Hub holds each card's viewers and the turn being written. The zero value is
// not usable; call New.
type Hub struct {
	mu     sync.Mutex
	cards  map[string]*card
	total  int
	closed bool
}

func New() *Hub {
	return &Hub{cards: map[string]*card{}}
}

// Sub is one viewer. C closes when the viewer is dropped, the hub closes, or
// Close is called.
type Sub struct {
	C       <-chan Frame
	c       chan Frame
	hub     *Hub
	cardID  string
	dropped bool
	done    bool
}

// Subscribe adds a viewer of cardID and returns the frames that rebuild the
// turn being written, if any: its Turn frame and one Delta with the text so far.
func (h *Hub) Subscribe(cardID string) (*Sub, []Frame, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := h.cards[cardID]
	if h.closed || h.total >= Total || (c != nil && len(c.subs) >= PerCard) {
		return nil, nil, ErrFull
	}
	if c == nil {
		c = &card{subs: map[*Sub]struct{}{}}
		h.cards[cardID] = c
	}
	ch := make(chan Frame, Buffer)
	s := &Sub{C: ch, c: ch, hub: h, cardID: cardID}
	c.subs[s] = struct{}{}
	h.total++
	var snapshot []Frame
	if c.turn != nil {
		snapshot = append(snapshot, *c.turn)
		if len(c.text) > 0 {
			snapshot = append(snapshot, Frame{Kind: Delta, AttemptID: c.turn.AttemptID, Turn: c.turn.Turn, Text: string(c.text)})
		}
	}
	return s, snapshot, nil
}

// Dropped reports whether the hub ended this viewer because it fell behind.
func (s *Sub) Dropped() bool {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	return s.dropped
}

// Close removes the viewer.
func (s *Sub) Close() {
	s.hub.mu.Lock()
	defer s.hub.mu.Unlock()
	s.hub.remove(s)
}

// Publish records f as the card's current turn state and sends it to every
// viewer without blocking. A viewer whose buffer is full is dropped. A nil hub
// publishes nothing.
func (h *Hub) Publish(cardID string, f Frame) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	c := h.cards[cardID]
	if c == nil {
		if f.Kind != Turn {
			return
		}
		c = &card{subs: map[*Sub]struct{}{}}
		h.cards[cardID] = c
	}
	switch f.Kind {
	case Turn:
		turn := f
		c.turn, c.text = &turn, nil
	case Delta:
		if c.turn == nil || c.turn.AttemptID != f.AttemptID || c.turn.Turn != f.Turn || len(c.text) >= MaxText {
			return
		}
		if room := MaxText - len(c.text); len(f.Text) > room {
			for room > 0 && !utf8.RuneStart(f.Text[room]) {
				room--
			}
			f.Text = f.Text[:room]
		}
		c.text = append(c.text, f.Text...)
	case Reset:
		c.text = nil
	case TurnEnd:
		c.turn, c.text = nil, nil
	}
	for s := range c.subs {
		select {
		case s.c <- f:
		default:
			s.dropped = true
			h.remove(s)
		}
	}
	if len(c.subs) == 0 && c.turn == nil {
		delete(h.cards, cardID)
	}
}

// Close ends every viewer and ignores later publishes, at server shutdown.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, c := range h.cards {
		for s := range c.subs {
			h.remove(s)
		}
	}
	h.cards = map[string]*card{}
}

func (h *Hub) remove(s *Sub) {
	if s.done {
		return
	}
	s.done = true
	close(s.c)
	h.total--
	if c := h.cards[s.cardID]; c != nil {
		delete(c.subs, s)
		if len(c.subs) == 0 && c.turn == nil {
			delete(h.cards, s.cardID)
		}
	}
}
