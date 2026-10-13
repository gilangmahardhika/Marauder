package netguard

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
)

type fakeResolver map[string][]string

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, s := range f[host] {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func TestNetguardRefusesPrivate(t *testing.T) {
	for _, raw := range []string{
		"http://example.com/mcp",
		"ftp://example.com/mcp",
		"https://user:pw@example.com/mcp",
		"https://example.com/mcp?token=abc",
		"https://example.com/mcp#x",
		"https:///mcp",
	} {
		if _, err := CheckURL(raw, Policy{}); !errors.Is(err, ErrURL) {
			t.Errorf("%s: err = %v, want ErrURL", raw, err)
		}
	}
	for _, raw := range []string{
		"https://127.0.0.1/mcp",
		"https://169.254.169.254/latest",
		"https://10.0.0.5/mcp",
		"https://[::ffff:127.0.0.1]/mcp",
		"https://[::1]/mcp",
		"https://100.64.1.1/mcp",
		"https://0.0.0.0/mcp",
	} {
		if _, err := CheckURL(raw, Policy{}); !errors.Is(err, ErrAddress) {
			t.Errorf("%s: err = %v, want ErrAddress", raw, err)
		}
	}
	if _, err := CheckURL("https://mcp.example.com/v1", Policy{}); err != nil {
		t.Fatalf("public url refused: %v", err)
	}

	// A public name that resolves to an internal address, as a rebinding
	// resolver would answer, is refused at dial time.
	resolver := fakeResolver{
		"rebind.example.com":   {"127.0.0.1"},
		"metadata.example.com": {"169.254.169.254"},
		"mixed.example.com":    {"93.184.216.34", "10.0.0.5"},
		"mapped.example.com":   {"::ffff:192.168.1.1"},
	}
	client := Client(Policy{}, resolver)
	for host := range resolver {
		_, err := client.Get("https://" + host + "/mcp")
		if !errors.Is(err, ErrAddress) {
			t.Errorf("%s: err = %v, want ErrAddress", host, err)
		}
	}
	for _, s := range []string{"8.8.8.8", "2606:4700::1111"} {
		if Blocked(netip.MustParseAddr(s)) {
			t.Errorf("%s blocked", s)
		}
	}
}

func TestNetguardRedirectAndProxy(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirect.Close()

	proxyHit := false
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyHit = true
		w.WriteHeader(502)
	}))
	defer proxy.Close()
	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)

	client := Client(Policy{AllowLoopbackHTTP: true}, nil)
	resp, err := client.Get(target.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || proxyHit {
		t.Fatalf("status %d proxyHit %v", resp.StatusCode, proxyHit)
	}
	_, err = client.Get(redirect.URL)
	if !errors.Is(err, ErrRedirect) {
		t.Fatalf("redirect err = %v", err)
	}

	// Without the loopback policy the same local server is refused.
	_, err = Client(Policy{}, nil).Get(target.URL)
	var ue *url.Error
	if !errors.As(err, &ue) || !strings.Contains(err.Error(), ErrAddress.Error()) {
		t.Fatalf("loopback without policy: %v", err)
	}
	if _, err := CheckURL(target.URL, Policy{}); !errors.Is(err, ErrURL) {
		t.Fatalf("http url without policy: %v", err)
	}
}
