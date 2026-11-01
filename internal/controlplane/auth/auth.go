// Package auth implements the single-operator trust boundary. Forwarded identity
// headers are never credentials. Access keys come only from the fixed issuer.
package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/postfix/cctvscan/internal/httptransport"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Mode, Issuer, Audience, TrustCIDR string
	Client                            *http.Client
}
type Boundary struct {
	config  Config
	trusted netip.Prefix
	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	expires time.Time
}

func New(c Config) (*Boundary, error) {
	if c.Mode == "" {
		c.Mode = "access"
	}
	b := &Boundary{config: c}
	switch c.Mode {
	case "local":
		if c.TrustCIDR != "" {
			p, e := netip.ParsePrefix(c.TrustCIDR)
			if e != nil {
				return nil, errors.New("invalid local trust CIDR")
			}
			good := false
			for _, raw := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"} {
				v := netip.MustParsePrefix(raw)
				if p.Addr().Is4() && p.Bits() >= v.Bits() && v.Contains(p.Masked().Addr()) {
					good = true
				}
			}
			if !good {
				return nil, errors.New("local trust CIDR must be an RFC1918 subset")
			}
			b.trusted = p.Masked()
		}
	case "access":
		u, e := url.Parse(c.Issuer)
		if e != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || !strings.HasSuffix(u.Host, ".cloudflareaccess.com") || strings.Count(u.Host, ".") != 2 || c.Audience == "" {
			return nil, errors.New("access requires fixed HTTPS Cloudflare team issuer and audience")
		}
		b.config.Issuer = strings.TrimSuffix(c.Issuer, "/")
	default:
		return nil, errors.New("auth mode must be access or explicit local")
	}
	if b.config.Client == nil {
		b.config.Client = httptransport.NewAccessClient()
	}
	return b, nil
}
func (b *Boundary) Protect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		good := false
		if b.config.Mode == "local" {
			host, _, e := net.SplitHostPort(r.RemoteAddr)
			if e == nil {
				a, e := netip.ParseAddr(host)
				good = e == nil && (a.IsLoopback() || (b.trusted.IsValid() && b.trusted.Contains(a.Unmap())))
			}
		} else {
			good = b.verify(r.Context(), r.Header.Get("Cf-Access-Jwt-Assertion")) == nil
		}
		if !good {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, "{\"error\":\"authentication required\"}\n")
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (b *Boundary) verify(ctx context.Context, token string) error {
	deny := errors.New("invalid access token")
	if len(token) > 16384 {
		return deny
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return deny
	}
	decode := base64.RawURLEncoding.DecodeString
	h, e := decode(parts[0])
	if e != nil {
		return deny
	}
	var head struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if json.Unmarshal(h, &head) != nil || head.Alg != "RS256" || head.Kid == "" {
		return deny
	}
	p, e := decode(parts[1])
	if e != nil {
		return deny
	}
	var claims struct {
		Issuer    string          `json:"iss"`
		Audience  json.RawMessage `json:"aud"`
		Expiry    int64           `json:"exp"`
		NotBefore int64           `json:"nbf"`
	}
	if json.Unmarshal(p, &claims) != nil {
		return deny
	}
	now := time.Now().Unix()
	if claims.Issuer != b.config.Issuer || claims.Expiry <= now || claims.NotBefore > now {
		return deny
	}
	var audiences []string
	var single string
	if json.Unmarshal(claims.Audience, &audiences) != nil {
		if json.Unmarshal(claims.Audience, &single) != nil {
			return deny
		}
		audiences = []string{single}
	}
	found := false
	for _, v := range audiences {
		if v == b.config.Audience {
			found = true
		}
	}
	if !found {
		return deny
	}
	key, e := b.key(ctx, head.Kid)
	if e != nil {
		return deny
	}
	sig, e := decode(parts[2])
	if e != nil {
		return deny
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	return rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], sig)
}
func (b *Boundary) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if time.Now().Before(b.expires) {
		if k := b.keys[kid]; k != nil {
			return k, nil
		}
		return nil, errors.New("unknown key")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", b.config.Issuer+"/cdn-cgi/access/certs", nil)
	if e != nil {
		return nil, e
	}
	res, e := b.config.Client.Do(req)
	if e != nil {
		return nil, errors.New("JWKS unavailable")
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("JWKS unavailable")
	}
	var set struct {
		Keys []struct{ Kty, Kid, Alg, N, E string }
	}
	if json.NewDecoder(io.LimitReader(res.Body, 262144)).Decode(&set) != nil {
		return nil, errors.New("invalid JWKS")
	}
	keys := map[string]*rsa.PublicKey{}
	for _, j := range set.Keys {
		if j.Kty != "RSA" || (j.Alg != "" && j.Alg != "RS256") {
			continue
		}
		n, e1 := base64.RawURLEncoding.DecodeString(j.N)
		exp, e2 := base64.RawURLEncoding.DecodeString(j.E)
		if e1 != nil || e2 != nil || len(n) < 256 || len(n) > 1024 || len(exp) > 4 || len(exp) == 0 {
			continue
		}
		v := new(big.Int).SetBytes(exp).Int64()
		if v < 3 || v%2 == 0 {
			continue
		}
		keys[j.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(v)}
	}
	b.keys = keys
	b.expires = time.Now().Add(5 * time.Minute)
	if k := keys[kid]; k != nil {
		return k, nil
	}
	return nil, errors.New("unknown key")
}
