package webpush

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"orchestrator/backend/internal/board/boardtest"
	"orchestrator/backend/internal/identity"
	"orchestrator/backend/internal/notify"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := decode(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestWebPushEncrypt checks encrypt against RFC 8291 Appendix A.
func TestWebPushEncrypt(t *testing.T) {
	as, err := ecdh.P256().NewPrivateKey(mustDecode(t, "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	if got := encode(as.PublicKey().Bytes()); got != "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8" {
		t.Fatalf("as_public = %s", got)
	}
	ua := mustDecode(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4")
	auth := mustDecode(t, "BTBZMqHH6r4Tts7J_aSIgg")
	salt := mustDecode(t, "DGv6ra1nlYgDCS1FRnbzlw")
	body, err := encrypt([]byte("When I grow up, I want to be a watermelon"), ua, auth, as, salt)
	if err != nil {
		t.Fatal(err)
	}
	want := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if got := encode(body); got != want {
		t.Fatalf("message =\n%s\nwant\n%s", got, want)
	}
	if _, err := encrypt(make([]byte, recordSize), ua, auth, as, salt); err != ErrPayload {
		t.Fatalf("oversized payload = %v", err)
	}
}

func TestVAPIDJWT(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	exp := time.Now().Add(jwtFor)
	jwt, err := VAPIDJWT(key, "https://fcm.googleapis.com", "mailto:admin@example.com", exp)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt = %s", jwt)
	}
	var header map[string]string
	if err := json.Unmarshal(mustDecode(t, parts[0]), &header); err != nil || header["alg"] != "ES256" || header["typ"] != "JWT" {
		t.Fatalf("header = %s", mustDecode(t, parts[0]))
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(mustDecode(t, parts[1]), &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Aud != "https://fcm.googleapis.com" || claims.Sub != "mailto:admin@example.com" || claims.Exp != exp.Unix() {
		t.Fatalf("claims = %+v", claims)
	}
	if time.Until(time.Unix(claims.Exp, 0)) > 24*time.Hour {
		t.Fatal("exp is more than 24 hours away")
	}
	sig := mustDecode(t, parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if len(sig) != 64 || !ecdsa.Verify(&key.PublicKey, digest[:], r, s) {
		t.Fatal("signature does not verify")
	}

	encoded, err := EncodeKey(key)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseKey(encoded)
	if err != nil || !back.Equal(key) {
		t.Fatalf("key round trip: %v", err)
	}
	pub, err := PublicKey(key)
	if err != nil || len(mustDecode(t, pub)) != 65 || strings.Contains(pub, encoded) {
		t.Fatalf("public key = %s %v", pub, err)
	}
}

func TestPushEndpointGuard(t *testing.T) {
	for _, ok := range []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
		"https://web.push.apple.com/QGx",
		"https://fcm.googleapis.com:443/fcm/send/abc",
	} {
		if _, err := CheckEndpoint(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://fcm.googleapis.com/fcm/send/abc",
		"https://127.0.0.1/push",
		"https://169.254.169.254/latest/meta-data",
		"https://localhost/push",
		"https://evil.example/fcm.googleapis.com",
		"https://fcm.googleapis.com.evil.example/x",
		"https://push.services.mozilla.com/x",
		"https://user:pw@fcm.googleapis.com/x",
		"https://fcm.googleapis.com:8443/x",
		"https://fcm.googleapis.com/x#frag",
		"https://fcm.googleapis.com/" + strings.Repeat("a", MaxEndpoint),
		"fcm.googleapis.com/x",
	} {
		if _, err := CheckEndpoint(bad); err == nil {
			t.Errorf("%s accepted", bad)
		}
	}
	ua, _ := ecdh.P256().GenerateKey(rand.Reader)
	good := encode(ua.PublicKey().Bytes())
	if err := CheckKeys(good, encode(make([]byte, 16))); err != nil {
		t.Fatalf("good keys: %v", err)
	}
	for _, keys := range [][2]string{
		{"not-base64!", encode(make([]byte, 16))},
		{encode(make([]byte, 65)), encode(make([]byte, 16))},
		{good, encode(make([]byte, 8))},
	} {
		if err := CheckKeys(keys[0], keys[1]); err == nil {
			t.Errorf("keys %v accepted", keys)
		}
	}
}

// browser is a subscriber that can read what it was sent.
type browser struct {
	key  *ecdh.PrivateKey
	auth []byte
}

func newBrowser(t *testing.T) browser {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	auth := make([]byte, 16)
	_, _ = rand.Read(auth)
	return browser{key: key, auth: auth}
}

// decrypt is the browser's side of RFC 8291.
func (b browser) decrypt(t *testing.T, body []byte) []byte {
	t.Helper()
	salt, rs, idlen := body[:16], binary.BigEndian.Uint32(body[16:20]), int(body[20])
	asPub, err := ecdh.P256().NewPublicKey(body[21 : 21+idlen])
	if err != nil || rs != recordSize {
		t.Fatalf("header: %v rs %d", err, rs)
	}
	shared, _ := b.key.ECDH(asPub)
	prkKey, _ := hkdf.Extract(sha256.New, shared, b.auth)
	ikm, _ := hkdf.Expand(sha256.New, prkKey, "WebPush: info\x00"+string(b.key.PublicKey().Bytes())+string(asPub.Bytes()), 32)
	prk, _ := hkdf.Extract(sha256.New, ikm, salt)
	cek, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Expand(sha256.New, prk, "Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idlen:], nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if plain[len(plain)-1] != 0x02 {
		t.Fatal("no last-record delimiter")
	}
	return plain[:len(plain)-1]
}

type received struct {
	path string
	auth string
	enc  string
	ttl  string
	body []byte
}

func TestPushGone(t *testing.T) {
	db := boardtest.Open(t)
	ctx := context.Background()
	user, err := identity.Insert(ctx, db, "owner@example.com", "password-long-1", "developer", false)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var got []received
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{path: r.URL.Path, auth: r.Header.Get("Authorization"), enc: r.Header.Get("Content-Encoding"), ttl: r.Header.Get("TTL"), body: body})
		mu.Unlock()
		switch r.URL.Path {
		case "/gone":
			w.WriteHeader(http.StatusGone)
		case "/flaky":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusCreated)
		}
	}))
	defer srv.Close()

	b := newBrowser(t)
	subscribe := func(path string) {
		t.Helper()
		if _, err := db.Exec(`
			INSERT INTO push_subscriptions (id, user_id, endpoint, p256dh, auth, created_at)
			VALUES ($1, $2, $3, $4, $5, '2026-10-14T00:00:00Z')`,
			"psub"+strings.ReplaceAll(path, "/", "_"), user.ID, srv.URL+path, encode(b.key.PublicKey().Bytes()), encode(b.auth)); err != nil {
			t.Fatal(err)
		}
	}
	subscribe("/ok")
	subscribe("/gone")
	subscribe("/flaky")
	key, err := GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	p := &Pusher{DB: db, Logf: t.Logf, Sender: &Sender{
		Client: srv.Client(), Key: key, Subject: "mailto:admin@example.com",
		Allow: func(host string) bool { return host == "127.0.0.1" },
	}}
	row := notify.Row{UserID: user.ID, Title: "Changes requested", Body: "Export CSV · Widget", URL: "/cards/card_1?project=proj_a", CardID: "card_1"}
	p.Send(ctx, []notify.Row{row})

	endpoints := func() []string {
		t.Helper()
		rows, err := db.Query(`SELECT endpoint FROM push_subscriptions ORDER BY endpoint`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var e string
			_ = rows.Scan(&e)
			out = append(out, strings.TrimPrefix(e, srv.URL))
		}
		return out
	}
	if got := strings.Join(endpoints(), ","); got != "/flaky,/ok" {
		t.Fatalf("after 410 = %s", got)
	}
	mu.Lock()
	first := got[0]
	mu.Unlock()
	if first.path != "/ok" || first.enc != "aes128gcm" || first.ttl != "86400" || !strings.HasPrefix(first.auth, "vapid t=") {
		t.Fatalf("request = %+v", first)
	}
	pub, _ := PublicKey(key)
	if !strings.HasSuffix(first.auth, ", k="+pub) {
		t.Fatalf("authorization = %s", first.auth)
	}
	var msg Message
	if err := json.Unmarshal(b.decrypt(t, first.body), &msg); err != nil {
		t.Fatal(err)
	}
	if msg != (Message{Title: "Changes requested", Body: "Export CSV · Widget", URL: "/cards/card_1?project=proj_a", Tag: "card_1"}) {
		t.Fatalf("message = %+v", msg)
	}

	for i := 1; i < MaxFailures; i++ {
		p.Send(ctx, []notify.Row{row})
	}
	if got := strings.Join(endpoints(), ","); got != "/ok" {
		t.Fatalf("after %d failures = %s", MaxFailures, got)
	}

	p.Sender.Allow = nil
	if n, err := p.SendTest(ctx, user.ID); err != nil || n != 0 {
		t.Fatalf("send to a host off the allowlist = %d %v", n, err)
	}
	if got := endpoints(); len(got) != 0 {
		t.Fatalf("a refused endpoint stays: %v", got)
	}
}
