package deploy_test

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"orchestrator/backend/internal/deploy"
)

func TestSHAArg(t *testing.T) {
	if err := deploy.ValidSHA("dev;id"); err == nil {
		t.Fatal("dev;id accepted")
	}
	if err := deploy.ValidSHA("0123456789abcdef0123456789abcdef01234567"); err != nil {
		t.Fatal(err)
	}
}

func TestDeployCommandPath(t *testing.T) {
	repo := t.TempDir()
	data := t.TempDir()
	outside := filepath.Join(t.TempDir(), "deploy")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := deploy.ValidCommand(outside, repo, data, -1); err != nil {
		t.Fatal(err)
	}
	insideRepo := filepath.Join(repo, "deploy")
	if err := os.WriteFile(insideRepo, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := deploy.ValidCommand(insideRepo, repo, data, -1); err == nil {
		t.Fatal("repo command accepted")
	}
	insideData := filepath.Join(data, "deploy")
	if err := os.WriteFile(insideData, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := deploy.ValidCommand(insideData, repo, data, -1); err == nil {
		t.Fatal("data command accepted")
	}
	writable := filepath.Join(t.TempDir(), "deploy")
	if err := os.WriteFile(writable, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(writable, 0o770); err != nil {
		t.Fatal(err)
	}
	if err := deploy.ValidCommand(writable, repo, data, -1); err == nil {
		t.Fatal("group writable command accepted")
	}
}

func TestHealthLinkLocal(t *testing.T) {
	err := deploy.Probe("https://meta.example/health", []string{"meta.example"}, false, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("169.254.169.254")}, nil
	}, nil)
	if err == nil {
		t.Fatal("link-local was probed")
	}
}

func TestHealthLoopback(t *testing.T) {
	err := deploy.Probe("https://dev.example/health", []string{"dev.example"}, false, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}, nil)
	if err == nil {
		t.Fatal("loopback was allowed")
	}
}

func TestHealthPrivateListed(t *testing.T) {
	probed := false
	client := &http.Client{Transport: round(func(r *http.Request) (*http.Response, error) {
		probed = true
		return &http.Response{StatusCode: 200, Body: http.NoBody, Request: r}, nil
	})}
	err := deploy.Probe("https://dev.example/health", []string{"dev.example"}, false, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.0.0.5")}, nil
	}, client)
	if err != nil || !probed {
		t.Fatalf("err=%v probed=%v", err, probed)
	}
}

func TestHealthUnlisted(t *testing.T) {
	err := deploy.Probe("https://evil.example/health", []string{"dev.example"}, false, nil, nil)
	if err != deploy.ErrDeployConfig {
		t.Fatalf("err = %v", err)
	}
}

func TestHealthRedirect(t *testing.T) {
	second := 0
	other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		second++
	}))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL, http.StatusFound)
	}))
	defer first.Close()
	err := deploy.Probe(first.URL, []string{"127.0.0.1"}, true, func(string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("127.0.0.1")}, nil
	}, &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return deploy.ErrProbe
	}})
	if err == nil || second != 0 {
		t.Fatalf("err=%v second=%d", err, second)
	}
}

type round func(*http.Request) (*http.Response, error)

func (r round) RoundTrip(req *http.Request) (*http.Response, error) { return r(req) }
