package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testTeamDomain = "team.example.cloudflareaccess.com"
	testAud        = "test-aud-tag"
)

// testJWKS は JWKS を返す httptest サーバーと、署名用の RSA 鍵を持つ。
// Task 7 のハンドラーテストからも再利用する。
type testJWKS struct {
	Server   *httptest.Server
	Key      *rsa.PrivateKey
	Kid      string
	Requests atomic.Int32
	// keysFn が非 nil なら、リクエスト回数(1始まり)ごとに返す鍵を決める。
	keysFn func(n int) map[string]*rsa.PublicKey
}

func newTestRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func jwksJSON(keys map[string]*rsa.PublicKey) []byte {
	type jwk struct {
		Kid string `json:"kid"`
		Kty string `json:"kty"`
		Alg string `json:"alg"`
		Use string `json:"use"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	var out struct {
		Keys []jwk `json:"keys"`
	}
	for kid, pub := range keys {
		out.Keys = append(out.Keys, jwk{
			Kid: kid, Kty: "RSA", Alg: "RS256", Use: "sig",
			N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		})
	}
	b, _ := json.Marshal(out)
	return b
}

// newTestJWKS は鍵を1つ生成し、その公開鍵を kid "test-kid" で配る JWKS サーバーを立てる。
func newTestJWKS(t *testing.T) *testJWKS {
	t.Helper()
	j := &testJWKS{Key: newTestRSAKey(t), Kid: "test-kid"}
	j.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(j.Requests.Add(1))
		keys := map[string]*rsa.PublicKey{j.Kid: &j.Key.PublicKey}
		if j.keysFn != nil {
			keys = j.keysFn(n)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(jwksJSON(keys))
	}))
	t.Cleanup(j.Server.Close)
	return j
}

// URL は NewAccessVerifier に渡す certsURL。
func (j *testJWKS) URL() string { return j.Server.URL }

// validClaims は検証を通る標準クレームを返す。
func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"aud": []string{testAud},
		"iss": "https://" + testTeamDomain,
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
}

// Sign は j の鍵・kid で RS256 署名したトークンを返す。
func (j *testJWKS) Sign(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	return signTestToken(t, j.Key, j.Kid, jwt.SigningMethodRS256, claims)
}

func signTestToken(t *testing.T, key any, kid string, m jwt.SigningMethod, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(m, claims)
	if kid != "" {
		tok.Header["kid"] = kid
	}
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

func doRequest(h http.Handler, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if token != "" {
		req.Header.Set("Cf-Access-Jwt-Assertion", token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func assertForbidden(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if rec.Body.String() != "forbidden" {
		t.Fatalf("body = %q, want %q", rec.Body.String(), "forbidden")
	}
}

func newTestVerifier(j *testJWKS) *AccessVerifier {
	return NewAccessVerifier(testTeamDomain, testAud, j.URL(), j.Server.Client())
}

func signNone(t *testing.T, claims jwt.MapClaims, kid string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestMiddleware_ValidToken(t *testing.T) {
	j := newTestJWKS(t)
	h := newTestVerifier(j).Middleware(okHandler())
	rec := doRequest(h, j.Sign(t, validClaims()))
	if rec.Code != 200 || rec.Body.String() != "ok" {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_AudAsString(t *testing.T) {
	j := newTestJWKS(t)
	h := newTestVerifier(j).Middleware(okHandler())
	c := validClaims()
	c["aud"] = testAud
	if rec := doRequest(h, j.Sign(t, c)); rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestMiddleware_TeamDomainWithScheme(t *testing.T) {
	j := newTestJWKS(t)
	for _, td := range []string{"https://" + testTeamDomain, "https://" + testTeamDomain + "/"} {
		h := NewAccessVerifier(td, testAud, j.URL(), j.Server.Client()).Middleware(okHandler())
		if rec := doRequest(h, j.Sign(t, validClaims())); rec.Code != 200 {
			t.Fatalf("%q: status = %d", td, rec.Code)
		}
	}
	if got := CertsURL("https://" + testTeamDomain); got != "https://"+testTeamDomain+"/cdn-cgi/access/certs" {
		t.Fatalf("CertsURL = %q", got)
	}
}

func TestMiddleware_MissingHeader(t *testing.T) {
	j := newTestJWKS(t)
	h := newTestVerifier(j).Middleware(okHandler())
	assertForbidden(t, doRequest(h, ""))
}

func TestMiddleware_Rejects(t *testing.T) {
	j := newTestJWKS(t)
	h := newTestVerifier(j).Middleware(okHandler())
	mod := func(f func(c jwt.MapClaims)) string {
		c := validClaims()
		f(c)
		return j.Sign(t, c)
	}
	other := newTestRSAKey(t)
	cases := map[string]string{
		"WrongAudience":       mod(func(c jwt.MapClaims) { c["aud"] = []string{"other"} }),
		"NoAudience":          mod(func(c jwt.MapClaims) { delete(c, "aud") }),
		"WrongIssuer":         mod(func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com" }),
		"IssuerTrailingSlash": mod(func(c jwt.MapClaims) { c["iss"] = "https://" + testTeamDomain + "/" }),
		"NoIssuer":            mod(func(c jwt.MapClaims) { delete(c, "iss") }),
		"Expired":             mod(func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-time.Hour).Unix() }),
		"NoExp":               mod(func(c jwt.MapClaims) { delete(c, "exp") }),
		"NotYetValid":         mod(func(c jwt.MapClaims) { c["nbf"] = time.Now().Add(time.Hour).Unix() }),
		"HS256Token":          signTestToken(t, []byte("secret"), j.Kid, jwt.SigningMethodHS256, validClaims()),
		"HS256PublicKeyAsKey": signTestToken(t, append(j.Key.PublicKey.N.Bytes(), byte(j.Key.PublicKey.E)), j.Kid, jwt.SigningMethodHS256, validClaims()),
		"AlgNone":             signNone(t, validClaims(), j.Kid),
		"UnknownKid":          signTestToken(t, other, j.Kid, jwt.SigningMethodRS256, validClaims()),
		"OtherKidOtherKey":    signTestToken(t, other, "other-kid", jwt.SigningMethodRS256, validClaims()),
		"NoKid":               signTestToken(t, j.Key, "", jwt.SigningMethodRS256, validClaims()),
		"Garbage":             "not.a.jwt",
		"RS384":               signTestToken(t, j.Key, j.Kid, jwt.SigningMethodRS384, validClaims()),
	}
	for name, tok := range cases {
		t.Run(name, func(t *testing.T) {
			assertForbidden(t, doRequest(h, tok))
		})
	}
}

func TestMiddleware_EmptyAudienceFailsClosed(t *testing.T) {
	j := newTestJWKS(t)
	c := validClaims()
	c["aud"] = ""
	tok := j.Sign(t, c)
	h := NewAccessVerifier(testTeamDomain, "", j.URL(), j.Server.Client()).Middleware(okHandler())
	assertForbidden(t, doRequest(h, tok))
}

func TestMiddleware_JWKSUnavailableFailsClosed(t *testing.T) {
	j := newTestJWKS(t)
	tok := j.Sign(t, validClaims())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", 500)
	}))
	defer srv.Close()
	h := NewAccessVerifier(testTeamDomain, testAud, srv.URL, srv.Client()).Middleware(okHandler())
	assertForbidden(t, doRequest(h, tok))
}

func TestMiddleware_OversizedJWKSFailsClosed(t *testing.T) {
	j := newTestJWKS(t)
	tok := j.Sign(t, validClaims())
	// 先頭に正しい鍵を置き、1MiB 超の padding を続ける。上限で切られて JSON が壊れる必要がある。
	valid := jwksJSON(map[string]*rsa.PublicKey{j.Kid: &j.Key.PublicKey})
	body := string(valid[:len(valid)-2]) + `],"pad":"` + strings.Repeat("a", 2<<20) + `"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer srv.Close()
	h := NewAccessVerifier(testTeamDomain, testAud, srv.URL, srv.Client()).Middleware(okHandler())
	assertForbidden(t, doRequest(h, tok))
}

func TestVerifier_RefetchOnUnknownKid(t *testing.T) {
	keyA, keyB := newTestRSAKey(t), newTestRSAKey(t)
	j := newTestJWKS(t)
	j.keysFn = func(n int) map[string]*rsa.PublicKey {
		if n == 1 {
			return map[string]*rsa.PublicKey{"kid-a": &keyA.PublicKey}
		}
		return map[string]*rsa.PublicKey{"kid-b": &keyB.PublicKey}
	}
	v := newTestVerifier(j)
	ctx := t.Context()

	if err := v.Verify(ctx, signTestToken(t, keyA, "kid-a", jwt.SigningMethodRS256, validClaims())); err != nil {
		t.Fatalf("A: %v", err)
	}
	if err := v.Verify(ctx, signTestToken(t, keyB, "kid-b", jwt.SigningMethodRS256, validClaims())); err != nil {
		t.Fatalf("B: %v", err)
	}
	if n := j.Requests.Load(); n != 2 {
		t.Fatalf("requests = %d, want 2", n)
	}
}

func TestVerifier_RefetchRateLimited(t *testing.T) {
	j := newTestJWKS(t)
	v := newTestVerifier(j)
	now := time.Now()
	v.now = func() time.Time { return now }
	ctx := t.Context()
	other := newTestRSAKey(t)
	bad := signTestToken(t, other, "unknown", jwt.SigningMethodRS256, validClaims())

	for i := 0; i < 3; i++ {
		if err := v.Verify(ctx, bad); err == nil {
			t.Fatal("expected error")
		}
	}
	if n := j.Requests.Load(); n > 2 {
		t.Fatalf("requests = %d, want <= 2", n)
	}
	before := j.Requests.Load()

	now = now.Add(2 * time.Minute)
	_ = v.Verify(ctx, bad)
	if n := j.Requests.Load(); n != before+1 {
		t.Fatalf("after interval requests = %d, want %d", n, before+1)
	}
}

func TestVerifier_NoRefetchWhenKidKnown(t *testing.T) {
	j := newTestJWKS(t)
	v := newTestVerifier(j)
	for i := 0; i < 5; i++ {
		if err := v.Verify(t.Context(), j.Sign(t, validClaims())); err != nil {
			t.Fatal(err)
		}
	}
	if n := j.Requests.Load(); n != 1 {
		t.Fatalf("requests = %d, want 1", n)
	}
}

// クライアントが切断済み（ctx キャンセル済み）でも JWKS の初回取得は完了し、
// 失敗扱いのクールダウンに入らないこと。
func TestVerifier_FetchSurvivesCancelledRequest(t *testing.T) {
	j := newTestJWKS(t)
	v := newTestVerifier(j)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.Verify(ctx, j.Sign(t, validClaims())); err != nil {
		t.Fatalf("verify with cancelled ctx: %v", err)
	}
	if n := j.Requests.Load(); n != 1 {
		t.Fatalf("jwks requests = %d, want 1", n)
	}
	h := v.Middleware(okHandler())
	if rec := doRequest(h, j.Sign(t, validClaims())); rec.Code != http.StatusOK {
		t.Fatalf("following request: status = %d, want 200", rec.Code)
	}
}
