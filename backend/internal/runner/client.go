// Package runner is the remote runner (RFC-106 to RFC-108): it heartbeats,
// claims leases, and runs each leased implement attempt on its own machine,
// reaching the board, the model, and MCP only through the control plane's job
// API. It never opens the database and never holds a model key.
package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"orchestrator/backend/internal/providers"
	"orchestrator/backend/internal/runnerapi"
)

// ErrProvider is a model call the control plane reported as a provider
// failure after its router's retry and fallback.
var ErrProvider = errors.New("provider_error")

// APIError is a control plane reply other than 2xx.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("control plane: %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("control plane: %d %s: %s", e.Status, e.Code, e.Message)
}

// StatusOf is the HTTP status of an APIError, or 0.
func StatusOf(err error) int {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status
	}
	return 0
}

// Client calls the runner API of one control plane. URL is its https origin.
type Client struct {
	URL    string
	Secret string
	HTTP   *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 15 * time.Minute}
}

// do sends body as JSON, or raw when it is an io.Reader, and decodes a JSON
// reply into out. It returns the reply status.
func (c *Client) do(ctx context.Context, method, path string, body, out any) (int, error) {
	var reader io.Reader
	kind := "application/json"
	switch b := body.(type) {
	case nil:
	case io.Reader:
		reader, kind = b, "application/octet-stream"
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			return 0, err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.URL, "/")+path, reader)
	if err != nil {
		return 0, err
	}
	if reader != nil {
		req.Header.Set("Content-Type", kind)
	}
	if c.Secret != "" {
		req.Header.Set("Authorization", "Runner "+c.Secret)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var reply struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply)
		return resp.StatusCode, &APIError{Status: resp.StatusCode, Code: reply.Error.Code, Message: reply.Error.Message}
	}
	if out != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

func jobPath(leaseID, route string) string {
	return "/api/runner/jobs/" + url.PathEscape(leaseID) + "/" + route
}

func (c *Client) Register(ctx context.Context, body runnerapi.Register) (runnerapi.Registered, error) {
	var out runnerapi.Registered
	_, err := c.do(ctx, http.MethodPost, "/api/runner/register", body, &out)
	return out, err
}

func (c *Client) Heartbeat(ctx context.Context, body runnerapi.Heartbeat) (runnerapi.HeartbeatReply, error) {
	var out runnerapi.HeartbeatReply
	_, err := c.do(ctx, http.MethodPost, "/api/runner/heartbeat", body, &out)
	return out, err
}

// Claim waits up to wait seconds for a lease. ok is false when nothing was
// claimed.
func (c *Client) Claim(ctx context.Context, wait int) (runnerapi.Claimed, bool, error) {
	var out runnerapi.Claimed
	status, err := c.do(ctx, http.MethodPost, "/api/runner/claim", runnerapi.Claim{WaitSeconds: wait}, &out)
	if err != nil {
		return out, false, err
	}
	return out, status == http.StatusOK, nil
}

func (c *Client) Start(ctx context.Context, leaseID string) (runnerapi.Job, error) {
	var out runnerapi.Job
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "start"), nil, &out)
	return out, err
}

func (c *Client) Board(ctx context.Context, leaseID string, op runnerapi.BoardOp) (runnerapi.BoardReply, error) {
	var out runnerapi.BoardReply
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "board"), op, &out)
	return out, err
}

func (c *Client) Model(ctx context.Context, leaseID string, call runnerapi.ModelCall) (runnerapi.ModelReply, error) {
	var out runnerapi.ModelReply
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "model"), call, &out)
	switch StatusOf(err) {
	case http.StatusRequestEntityTooLarge:
		return out, fmt.Errorf("%w: %v", providers.ErrContextFull, err)
	case http.StatusBadGateway:
		return out, fmt.Errorf("%w: %v", ErrProvider, err)
	}
	return out, err
}

func (c *Client) MCPTools(ctx context.Context, leaseID string) (runnerapi.MCPTools, error) {
	var out runnerapi.MCPTools
	_, err := c.do(ctx, http.MethodGet, jobPath(leaseID, "mcp"), nil, &out)
	return out, err
}

func (c *Client) MCPCall(ctx context.Context, leaseID string, call runnerapi.MCPCall) (runnerapi.MCPResult, error) {
	var out runnerapi.MCPResult
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "mcp"), call, &out)
	return out, err
}

// Bundle uploads a git bundle of the card branch at sha.
func (c *Client) Bundle(ctx context.Context, leaseID, sha string, bundle io.Reader) error {
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "bundle")+"?sha="+url.QueryEscape(sha), bundle, nil)
	return err
}

// CardBundle downloads the control plane's bundle of the card branch to
// dest and returns the head it names (RFC-119).
func (c *Client) CardBundle(ctx context.Context, leaseID, dest string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.URL, "/")+jobPath(leaseID, "bundle"), nil)
	if err != nil {
		return "", err
	}
	if c.Secret != "" {
		req.Header.Set("Authorization", "Runner "+c.Secret)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &APIError{Status: resp.StatusCode, Code: "bundle"}
	}
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(file, io.LimitReader(resp.Body, maxCardBundle+1)); err != nil {
		file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if info, err := os.Stat(dest); err != nil || info.Size() > maxCardBundle {
		return "", fmt.Errorf("the card bundle is over %d bytes", maxCardBundle)
	}
	return resp.Header.Get("X-Card-Sha"), nil
}

// maxCardBundle caps the card bundle a runner downloads, as the control
// plane caps the one it takes.
const maxCardBundle = 64 << 20

func (c *Client) Report(ctx context.Context, leaseID string, rep runnerapi.Report) (runnerapi.ReportReply, error) {
	var out runnerapi.ReportReply
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "report"), rep, &out)
	return out, err
}

func (c *Client) End(ctx context.Context, leaseID string) error {
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "end"), nil, nil)
	return err
}

// Release gives a lease back so the control plane requeues its card.
func (c *Client) Release(ctx context.Context, leaseID string) error {
	_, err := c.do(ctx, http.MethodPost, jobPath(leaseID, "release"), nil, nil)
	return err
}
