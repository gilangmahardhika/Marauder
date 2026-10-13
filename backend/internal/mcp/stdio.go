package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"

	"orchestrator/backend/internal/execx"
)

// Pipe is a newline-delimited connection to a stdio server, such as an
// execx.Process.
type Pipe interface {
	WriteFrame(frame []byte) error
	ReadFrame() ([]byte, error)
	Close() error
}

// DialStdio runs initialize over p. Close closes p. A call whose context ends
// closes p too, because the reply would arrive out of step.
func DialStdio(ctx context.Context, p Pipe) (Client, error) {
	c, err := newClient(ctx, &stdioTransport{p: p})
	if err != nil {
		return nil, err
	}
	return c, nil
}

type stdioTransport struct {
	p    Pipe
	mu   sync.Mutex
	dead bool
}

type readResult struct {
	msg wire
	err error
}

func (s *stdioTransport) call(ctx context.Context, id int64, body []byte) (wire, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return wire{}, ErrUnreachable
	}
	if err := s.p.WriteFrame(body); err != nil {
		s.kill()
		return wire{}, ErrUnreachable
	}
	done := make(chan readResult, 1)
	go func() {
		msg, err := s.read(id)
		done <- readResult{msg, err}
	}()
	select {
	case r := <-done:
		if r.err != nil {
			s.kill()
		}
		return r.msg, r.err
	case <-ctx.Done():
		s.kill()
		<-done
		return wire{}, ctx.Err()
	}
}

// read returns the response with id. A server request is answered with
// method-not-found; a notification is skipped.
func (s *stdioTransport) read(id int64) (wire, error) {
	for {
		frame, err := s.p.ReadFrame()
		if errors.Is(err, execx.ErrFrame) {
			return wire{}, ErrTooLarge
		}
		if err != nil {
			return wire{}, ErrUnreachable
		}
		if len(frame) > MaxResponse {
			return wire{}, ErrTooLarge
		}
		frame = bytes.TrimSpace(frame)
		if len(frame) == 0 {
			continue
		}
		var msg wire
		if err := json.Unmarshal(frame, &msg); err != nil {
			return wire{}, ErrProtocol
		}
		if msg.isResponse(id) {
			return msg, nil
		}
		if msg.isRequest() {
			if err := s.p.WriteFrame(methodNotFound(msg.ID)); err != nil {
				return wire{}, ErrUnreachable
			}
		}
	}
}

func (s *stdioTransport) notify(_ context.Context, body []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dead {
		return ErrUnreachable
	}
	if err := s.p.WriteFrame(body); err != nil {
		s.kill()
		return ErrUnreachable
	}
	return nil
}

func (s *stdioTransport) kill() {
	s.dead = true
	_ = s.p.Close()
}

func (s *stdioTransport) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dead = true
	return s.p.Close()
}
