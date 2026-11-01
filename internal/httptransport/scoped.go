package httptransport

import (
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"
)

// NewScopedCameraClient isolates an explicit job route without mutating global policy.
// It never follows redirects or falls back to a direct connection. Camera TLS
// compatibility does not disable verification of an HTTPS proxy's identity.
func NewScopedCameraClient(proxy *url.URL) *http.Client {
	t := &http.Transport{DialContext: (&net.Dialer{Timeout: 5 * time.Second}).DialContext, TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second, DisableKeepAlives: true, MaxConnsPerHost: 1}
	var rt http.RoundTripper = t
	if proxy != nil {
		t.Proxy = http.ProxyURL(proxy)
		if proxy.Scheme == "https" {
			t.DialTLSContext = secureProxyDialer(t, proxy)
		}
		rt = &proxiedTransport{transport: t}
	}
	return &http.Client{Transport: rt, Timeout: 7 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
