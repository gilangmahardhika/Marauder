package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"orchestrator/backend/internal/netguard"
)

// DialHTTP connects to a Streamable HTTP server and runs initialize. hc must
// be a netguard client; headers carry the server's credentials.
func DialHTTP(ctx context.Context, endpoint string, headers map[string]string, hc *http.Client) (Client, error) {
	c, err := newClient(ctx, &httpTransport{url: endpoint, headers: headers, hc: hc})
	if err != nil {
		return nil, err
	}
	return c, nil
}

type httpTransport struct {
	url     string
	headers map[string]string
	hc      *http.Client

	mu      sync.Mutex
	session string
	ready   bool
}

func (h *httpTransport) post(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.url, bytes.NewReader(body))
	if err != nil {
		return nil, ErrUnreachable
	}
	h.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := h.hc.Do(req)
	if err != nil {
		return nil, netErr(err)
	}
	return resp, nil
}

func (h *httpTransport) setHeaders(req *http.Request) {
	for name, value := range h.headers {
		req.Header.Set(name, value)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.session != "" {
		req.Header.Set("Mcp-Session-Id", h.session)
	}
	if h.ready {
		req.Header.Set("MCP-Protocol-Version", ProtocolVersion)
	}
}

func netErr(err error) error {
	if errors.Is(err, netguard.ErrAddress) || errors.Is(err, netguard.ErrRedirect) {
		return ErrBlocked
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return ErrUnreachable
}

func (h *httpTransport) call(ctx context.Context, id int64, body []byte) (wire, error) {
	resp, err := h.post(ctx, body)
	if err != nil {
		return wire{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return wire{}, fmt.Errorf("%w: status %d", ErrServer, resp.StatusCode)
	}
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		h.mu.Lock()
		h.session = sid
		h.mu.Unlock()
	}
	limited := &capReader{r: resp.Body, left: MaxResponse}
	media, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	var msg wire
	if media == "text/event-stream" {
		msg, err = h.readStream(ctx, limited, id)
	} else {
		msg, err = readJSON(limited, id)
	}
	if err != nil {
		return wire{}, err
	}
	h.mu.Lock()
	h.ready = true
	h.mu.Unlock()
	return msg, nil
}

func readJSON(r *capReader, id int64) (wire, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return wire{}, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) > 0 && raw[0] == '[' {
		var batch []wire
		if err := json.Unmarshal(raw, &batch); err != nil {
			return wire{}, ErrProtocol
		}
		for _, msg := range batch {
			if msg.isResponse(id) {
				return msg, nil
			}
		}
		return wire{}, ErrProtocol
	}
	var msg wire
	if err := json.Unmarshal(raw, &msg); err != nil || !msg.isResponse(id) {
		return wire{}, ErrProtocol
	}
	return msg, nil
}

// readStream reads server-sent events until the response with id. A server
// request on the stream is answered with method-not-found.
func (h *httpTransport) readStream(ctx context.Context, r *capReader, id int64) (wire, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64<<10), MaxResponse)
	var data strings.Builder
	dispatch := func() (wire, bool, error) {
		if data.Len() == 0 {
			return wire{}, false, nil
		}
		raw := data.String()
		data.Reset()
		var msg wire
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			return wire{}, false, ErrProtocol
		}
		if msg.isResponse(id) {
			return msg, true, nil
		}
		if msg.isRequest() {
			h.reply(ctx, msg.ID)
		}
		return wire{}, false, nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			msg, done, err := dispatch()
			if err != nil || done {
				return msg, err
			}
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(rest, " "))
		}
	}
	if r.over {
		return wire{}, ErrTooLarge
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return wire{}, ErrTooLarge
		}
		return wire{}, netErr(err)
	}
	msg, done, err := dispatch()
	if err != nil {
		return wire{}, err
	}
	if !done {
		return wire{}, ErrProtocol
	}
	return msg, nil
}

func (h *httpTransport) reply(ctx context.Context, id json.RawMessage) {
	resp, err := h.post(ctx, methodNotFound(id))
	if err == nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
	}
}

func (h *httpTransport) notify(ctx context.Context, body []byte) error {
	resp, err := h.post(ctx, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: status %d", ErrServer, resp.StatusCode)
	}
	return nil
}

func (h *httpTransport) close() error {
	h.mu.Lock()
	session := h.session
	h.mu.Unlock()
	if session == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, h.url, nil)
	if err != nil {
		return nil
	}
	h.setHeaders(req)
	if resp, err := h.hc.Do(req); err == nil {
		resp.Body.Close()
	}
	return nil
}

// capReader reads at most left bytes and then reports ErrTooLarge.
type capReader struct {
	r    io.Reader
	left int
	over bool
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.left <= 0 {
		var one [1]byte
		if n, _ := c.r.Read(one[:]); n > 0 {
			c.over = true
			return 0, ErrTooLarge
		}
		return 0, io.EOF
	}
	if len(p) > c.left {
		p = p[:c.left]
	}
	n, err := c.r.Read(p)
	c.left -= n
	return n, err
}
