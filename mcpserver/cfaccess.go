package mcpserver

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	accessHeader      = "Cf-Access-Jwt-Assertion"
	jwksMaxBytes      = 1 << 20
	jwksFetchTimeout  = 10 * time.Second
	jwksRefetchPeriod = time.Minute
	minRSABits        = 2048
)

// AccessVerifier は Cloudflare Access が付与する JWT を検証する。
// 検証失敗はすべてエラーとして返し、呼び出し側は拒否（fail closed）する。
type AccessVerifier struct {
	teamDomain string
	aud        string
	certsURL   string
	client     *http.Client
	now        func() time.Time

	mu          sync.Mutex
	keys        map[string]*rsa.PublicKey
	fetched     bool      // 一度でも取得に成功したか
	lastAttempt time.Time // 直近の「再取得」または失敗した取得の時刻（初回の成功取得は含めない）
}

func normalizeTeamDomain(d string) string {
	d = strings.TrimSpace(d)
	d = strings.TrimPrefix(d, "https://")
	d = strings.TrimPrefix(d, "http://")
	return strings.TrimRight(d, "/")
}

// CertsURL は teamDomain の JWKS エンドポイントを返す。
func CertsURL(teamDomain string) string {
	return "https://" + normalizeTeamDomain(teamDomain) + "/cdn-cgi/access/certs"
}

// NewAccessVerifier は検証器を作る。client が nil なら 10 秒タイムアウトの既定クライアントを使う。
func NewAccessVerifier(teamDomain, aud, certsURL string, client *http.Client) *AccessVerifier {
	if client == nil {
		client = &http.Client{Timeout: jwksFetchTimeout}
	}
	return &AccessVerifier{
		teamDomain: normalizeTeamDomain(teamDomain),
		aud:        aud,
		certsURL:   certsURL,
		client:     client,
		now:        time.Now,
		keys:       map[string]*rsa.PublicKey{},
	}
}

// Verify は token の署名・aud・iss・exp を検証する。
func (a *AccessVerifier) Verify(ctx context.Context, token string) error {
	if a.teamDomain == "" || a.aud == "" {
		return errors.New("verifier is not configured (teamDomain/aud empty)")
	}
	_, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("token has no kid")
		}
		return a.keyFor(ctx, kid)
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience(a.aud),
		jwt.WithIssuer("https://"+a.teamDomain),
		jwt.WithExpirationRequired(),
	)
	return err
}

// keyFor は kid に対応する公開鍵を返す。キャッシュに無ければ JWKS を取得する。
// 取得は serialize され、最後の試行から 1 分未満なら再取得しない。
func (a *AccessVerifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if k, ok := a.keys[kid]; ok {
		return k, nil
	}
	// 初回取得は即時に許可。以降（未知の kid による再取得、および失敗後のやり直し）は
	// 前回から 1 分以上空ける。
	if !a.lastAttempt.IsZero() && a.now().Sub(a.lastAttempt) < jwksRefetchPeriod {
		return nil, fmt.Errorf("unknown kid %q (refetch rate-limited)", kid)
	}
	if a.fetched {
		a.lastAttempt = a.now()
	}
	keys, err := a.fetchKeys(ctx)
	if err != nil {
		a.lastAttempt = a.now()
		return nil, fmt.Errorf("fetch jwks: %w", err)
	}
	a.keys = keys
	a.fetched = true
	if k, ok := a.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("unknown kid %q", kid)
}

// fetchKeys は JWKS を取得する。リクエスト元のクライアントが切断しても取得は続ける
// （途中で止めると失敗扱いになり、1 分間のクールダウンで後続の正当なリクエストまで拒否してしまうため）。
// 代わりに独自の 10 秒タイムアウトで打ち切る。
func (a *AccessVerifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), jwksFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.certsURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, jwksMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > jwksMaxBytes {
		return nil, errors.New("jwks response too large")
	}
	return parseJWKS(body)
}

func parseJWKS(body []byte) (map[string]*rsa.PublicKey, error) {
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.N, "="))
		if err != nil {
			continue
		}
		eb, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(k.E, "="))
		if err != nil || len(eb) == 0 || len(eb) > 4 {
			continue
		}
		e := 0
		for _, b := range eb {
			e = e<<8 | int(b)
		}
		n := new(big.Int).SetBytes(nb)
		if e < 3 || n.BitLen() < minRSABits {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: n, E: e}
	}
	if len(keys) == 0 {
		return nil, errors.New("no usable RSA keys in jwks")
	}
	return keys, nil
}

// Middleware は Cf-Access-Jwt-Assertion を検証し、失敗したら 403 forbidden を返す。
func (a *AccessVerifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(accessHeader)
		if token == "" {
			deny(w, r, "missing "+accessHeader+" header")
			return
		}
		if err := a.Verify(r.Context(), token); err != nil {
			deny(w, r, err.Error())
			return
		}
		next.ServeHTTP(w, r)
	})
}

func deny(w http.ResponseWriter, r *http.Request, reason string) {
	log.Printf("cfaccess: denied %s %s from %s: %s", r.Method, r.URL.Path, r.RemoteAddr, reason)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	w.Write([]byte("forbidden"))
}
