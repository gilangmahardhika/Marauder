// Package notify pushes board changes and personal notifications to open
// streams, and decides who is notified of a card event (RFC-113).
package notify

import (
	"errors"
	"sync"
)

// Frame kinds a stream receives from the hub.
const (
	// Changed means a card or run request of the project changed.
	Changed = "changed"
	// Notified carries the id of the user's new notification.
	Notified = "notified"
	// Resync means frames may have been lost; the stream sends everything again.
	Resync = "resync"
)

const (
	// Buffer is how many frames a stream may fall behind before its frames
	// are replaced by one Resync.
	Buffer = 64
	// PerUser and Total cap the open streams.
	PerUser = 8
	Total   = 256
)

// ErrFull means the user or the hub already has as many streams as allowed.
var ErrFull = errors.New("too many streams")

type Frame struct {
	Kind           string
	ProjectID      string
	NotificationID string
}

// Sub is one open stream. C closes when the hub closes or Close is called.
type Sub struct {
	C         <-chan Frame
	c         chan Frame
	hub       *Hub
	userID    string
	projectID string
	done      bool
}

// Hub holds the open streams. The zero value is not usable; call NewHub.
type Hub struct {
	mu     sync.Mutex
	subs   map[*Sub]struct{}
	users  map[string]int
	closed bool
}

func NewHub() *Hub {
	return &Hub{subs: map[*Sub]struct{}{}, users: map[string]int{}}
}

// Subscribe opens a stream for userID. projectID "" receives notifications
// only.
func (h *Hub) Subscribe(userID, projectID string) (*Sub, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.subs) >= Total || h.users[userID] >= PerUser {
		return nil, ErrFull
	}
	c := make(chan Frame, Buffer)
	s := &Sub{C: c, c: c, hub: h, userID: userID, projectID: projectID}
	h.subs[s] = struct{}{}
	h.users[userID]++
	return s, nil
}

func (s *Sub) Close() {
	h := s.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	h.drop(s)
}

func (h *Hub) drop(s *Sub) {
	if s.done {
		return
	}
	s.done = true
	delete(h.subs, s)
	if h.users[s.userID]--; h.users[s.userID] <= 0 {
		delete(h.users, s.userID)
	}
	close(s.c)
}

// Changed tells the streams of projectID to send the board again.
func (h *Hub) Changed(projectID string) {
	h.each(func(s *Sub) bool { return s.projectID != "" && s.projectID == projectID },
		Frame{Kind: Changed, ProjectID: projectID})
}

// Notified tells userID's streams about a new notification.
func (h *Hub) Notified(userID, id string) {
	h.each(func(s *Sub) bool { return s.userID == userID }, Frame{Kind: Notified, NotificationID: id})
}

// Resync tells every stream to send everything again, after the listener
// lost its connection.
func (h *Hub) Resync() {
	h.each(func(*Sub) bool { return true }, Frame{Kind: Resync})
}

func (h *Hub) each(match func(*Sub) bool, f Frame) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs {
		if match(s) {
			send(s, f)
		}
	}
}

// send never blocks. A stream that fell behind loses its queued frames and
// gets one Resync, since a resync sends everything those frames would have.
func send(s *Sub, f Frame) {
	select {
	case s.c <- f:
		return
	default:
	}
drain:
	for {
		select {
		case <-s.c:
		default:
			break drain
		}
	}
	select {
	case s.c <- Frame{Kind: Resync}:
	default:
	}
}

// Close ends every stream; Subscribe fails afterwards.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for s := range h.subs {
		h.drop(s)
	}
}
