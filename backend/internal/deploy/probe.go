package deploy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	ErrProbe        = errors.New("probe failed")
	ErrDeployConfig = errors.New("deploy_config")
	ErrSHA          = errors.New("sha is not 40 or 64 hex")
)

var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$|^[0-9a-f]{64}$`)

func ValidSHA(s string) error {
	if !shaPattern.MatchString(s) {
		return ErrSHA
	}
	return nil
}

func ValidCommand(path, repo, data string, uid int) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("command is not absolute")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		resolved = filepath.Clean(path)
	}
	repo = resolveDir(repo)
	data = resolveDir(data)
	if inside(repo, resolved) || inside(data, resolved) {
		return fmt.Errorf("command is inside the repo or data directory")
	}
	info, err := os.Lstat(resolved)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("command is not a regular file")
	}
	if info.Mode()&0o111 == 0 {
		return fmt.Errorf("command is not executable")
	}
	if info.Mode()&0o022 != 0 {
		return fmt.Errorf("command is group or world writable")
	}
	if stat, ok := info.Sys().(interface{ Owner() int }); ok && uid >= 0 && stat.Owner() != uid {
		return fmt.Errorf("command owner mismatch")
	}
	parent, err := os.Stat(filepath.Dir(resolved))
	if err != nil {
		return err
	}
	if parent.Mode()&0o022 != 0 {
		return fmt.Errorf("command directory is group or world writable")
	}
	return nil
}

func resolveDir(path string) string {
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func inside(root, path string) bool {
	if root == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

type Resolver func(host string) ([]net.IP, error)

func Probe(raw string, hosts []string, fixture bool, resolve Resolver, client *http.Client) error {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil {
		return ErrProbe
	}
	host := strings.ToLower(u.Hostname())
	if !listed(host, hosts) {
		return ErrDeployConfig
	}
	if u.Scheme != "https" && !(fixture && u.Scheme == "http" && host == "127.0.0.1") {
		return ErrProbe
	}
	if resolve == nil {
		resolve = func(h string) ([]net.IP, error) { return net.LookupIP(h) }
	}
	ips, err := resolve(host)
	if err != nil || len(ips) == 0 {
		return ErrProbe
	}
	for _, ip := range ips {
		if refused(ip, fixture) {
			return ErrProbe
		}
	}
	if client == nil {
		client = &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrProbe
		}}
	}
	resp, err := client.Get(raw)
	if err != nil {
		return ErrProbe
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ErrProbe
	}
	return nil
}

func listed(host string, hosts []string) bool {
	for _, item := range hosts {
		if strings.ToLower(item) == host {
			return true
		}
	}
	return false
}

func refused(ip net.IP, fixture bool) bool {
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.IsLoopback() && !fixture {
		return true
	}
	return false
}
