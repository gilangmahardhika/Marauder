// Package webpush sends browser push notifications (RFC-113): the payload is
// encrypted for the browser with RFC 8291 aes128gcm and the request is
// signed with an RFC 8292 VAPID JWT. It uses the standard library only.
package webpush

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrEndpoint = errors.New("endpoint must be https on a known push service")
	ErrKeys     = errors.New("subscription keys are not a P-256 key and a 16-byte secret")
	ErrPayload  = errors.New("payload does not fit in one record")
	// ErrGone means the push service no longer knows the subscription.
	ErrGone = errors.New("subscription is gone")
)

const (
	// recordSize is the aes128gcm record size; a payload fits in one record.
	recordSize = 4096
	// TTL is how long a push service keeps a message for an offline browser.
	TTL = 24 * time.Hour
	// jwtFor is how long a VAPID JWT is valid; RFC 8292 allows up to 24 hours.
	jwtFor = 12 * time.Hour
	// MaxEndpoint caps the stored endpoint URL.
	MaxEndpoint = 2048
)

// pushHosts are the push services of the major browsers. A leading dot
// matches any subdomain.
var pushHosts = []string{
	"fcm.googleapis.com",
	".push.services.mozilla.com",
	".notify.windows.com",
	"web.push.apple.com",
	".push.apple.com",
}

// KnownHost reports whether host is a browser push service.
func KnownHost(host string) bool {
	host = strings.ToLower(host)
	for _, h := range pushHosts {
		if strings.HasPrefix(h, ".") {
			if strings.HasSuffix(host, h) && len(host) > len(h) {
				return true
			}
		} else if host == h {
			return true
		}
	}
	return false
}

// CheckEndpoint refuses an endpoint that is not https on a known push
// service, carries user info or a fragment, or names another port.
func CheckEndpoint(raw string) (*url.URL, error) {
	if len(raw) > MaxEndpoint {
		return nil, ErrEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" {
		return nil, ErrEndpoint
	}
	if port := u.Port(); port != "" && port != "443" {
		return nil, ErrEndpoint
	}
	if !KnownHost(u.Hostname()) {
		return nil, ErrEndpoint
	}
	return u, nil
}

// decode accepts base64url with or without padding, as browsers send it.
func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

func encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// CheckKeys reports whether p256dh is an uncompressed P-256 point and auth a
// 16-byte secret, both base64url.
func CheckKeys(p256dh, auth string) error {
	pub, err := decode(p256dh)
	if err != nil {
		return ErrKeys
	}
	if _, err := ecdh.P256().NewPublicKey(pub); err != nil {
		return ErrKeys
	}
	secret, err := decode(auth)
	if err != nil || len(secret) != 16 {
		return ErrKeys
	}
	return nil
}

// Encrypt encrypts plaintext for the browser that holds p256dh and auth.
func Encrypt(plaintext []byte, p256dh, auth string) ([]byte, error) {
	if err := CheckKeys(p256dh, auth); err != nil {
		return nil, err
	}
	ua, _ := decode(p256dh)
	secret, _ := decode(auth)
	as, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	return encrypt(plaintext, ua, secret, as, salt)
}

// encrypt is RFC 8291 section 3 with one aes128gcm record (RFC 8188).
func encrypt(plaintext, uaPublic, authSecret []byte, as *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext)+1+16 > recordSize {
		return nil, ErrPayload
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, ErrKeys
	}
	shared, err := as.ECDH(ua)
	if err != nil {
		return nil, err
	}
	asPublic := as.PublicKey().Bytes()
	prkKey, err := hkdf.Extract(sha256.New, shared, authSecret)
	if err != nil {
		return nil, err
	}
	keyInfo := "WebPush: info\x00" + string(uaPublic) + string(asPublic)
	ikm, err := hkdf.Expand(sha256.New, prkKey, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	prk, err := hkdf.Extract(sha256.New, ikm, salt)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	header := make([]byte, 0, 16+4+1+len(asPublic))
	header = append(header, salt...)
	header = binary.BigEndian.AppendUint32(header, recordSize)
	header = append(header, byte(len(asPublic)))
	header = append(header, asPublic...)
	record := append(append([]byte{}, plaintext...), 0x02)
	return gcm.Seal(header, nonce, record, nil), nil
}

// GenerateKey makes a VAPID key pair.
func GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// EncodeKey is the private key as base64url of its 32-byte scalar, the form
// kept in secrets.json.
func EncodeKey(key *ecdsa.PrivateKey) (string, error) {
	raw, err := key.Bytes()
	if err != nil {
		return "", err
	}
	return encode(raw), nil
}

func ParseKey(s string) (*ecdsa.PrivateKey, error) {
	raw, err := decode(s)
	if err != nil {
		return nil, err
	}
	return ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
}

// PublicKey is the base64url uncompressed public key a browser subscribes
// with (applicationServerKey).
func PublicKey(key *ecdsa.PrivateKey) (string, error) {
	raw, err := key.PublicKey.Bytes()
	if err != nil {
		return "", err
	}
	return encode(raw), nil
}

// VAPIDJWT signs the RFC 8292 token for audience (the endpoint's origin).
func VAPIDJWT(key *ecdsa.PrivateKey, audience, subject string, exp time.Time) (string, error) {
	header := encode([]byte(`{"typ":"JWT","alg":"ES256"}`))
	claims, err := json.Marshal(map[string]any{"aud": audience, "exp": exp.Unix(), "sub": subject})
	if err != nil {
		return "", err
	}
	signing := header + "." + encode(claims)
	digest := sha256.Sum256([]byte(signing))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signing + "." + encode(sig), nil
}

// Subscription is one browser's push subscription.
type Subscription struct {
	ID       string
	UserID   string
	Endpoint string
	P256dh   string
	Auth     string
}

// Message is what a notification shows. It holds no reason, note, code,
// log, or token, since it passes through the browser vendor's push service.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// Sender posts encrypted messages to push services.
type Sender struct {
	// Client must be a netguard client in production.
	Client  *http.Client
	Key     *ecdsa.PrivateKey
	Subject string
	// Allow replaces KnownHost in tests.
	Allow func(host string) bool
	Now   func() time.Time
}

// Send delivers msg to sub. It returns ErrGone when the push service answers
// 404 or 410.
func (s *Sender) Send(ctx context.Context, sub Subscription, msg Message) error {
	u, err := url.Parse(sub.Endpoint)
	if err != nil {
		return ErrEndpoint
	}
	if s.Allow != nil {
		if !s.Allow(u.Hostname()) {
			return ErrEndpoint
		}
	} else if _, err := CheckEndpoint(sub.Endpoint); err != nil {
		return err
	}
	plain, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	body, err := Encrypt(plain, sub.P256dh, sub.Auth)
	if err != nil {
		return err
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	jwt, err := VAPIDJWT(s.Key, u.Scheme+"://"+u.Host, s.Subject, now().Add(jwtFor))
	if err != nil {
		return err
	}
	pub, err := PublicKey(s.Key)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+pub)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", fmt.Sprint(int(TTL.Seconds())))
	req.Header.Set("Urgency", "normal")
	resp, err := s.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrGone
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	default:
		return fmt.Errorf("push service answered %d", resp.StatusCode)
	}
}
