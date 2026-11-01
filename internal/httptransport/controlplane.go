package httptransport

import (
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewAccessClient isolates the fixed-issuer identity channel from scanner job
// proxies and ambient HTTP_PROXY. No camera traffic uses this client.
func NewAccessClient() *http.Client {
	return &http.Client{Transport: &http.Transport{DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 15 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxIdleConns: 2, IdleConnTimeout: 30 * time.Second}, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("JWKS redirects disabled") }}
}

// NewProxyDiagnosticClient uses only the explicitly selected saved profile.
// Both destination and proxy TLS remain strict; no global job proxy is read.
func NewProxyDiagnosticClient(proxy *url.URL) *http.Client {
	transport := &http.Transport{Proxy: http.ProxyURL(proxy), DialContext: (&net.Dialer{Timeout: 3 * time.Second, KeepAlive: 15 * time.Second}).DialContext, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 3 * time.Second, ResponseHeaderTimeout: 5 * time.Second, MaxIdleConns: 1, MaxConnsPerHost: 1, IdleConnTimeout: 10 * time.Second}
	return &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
