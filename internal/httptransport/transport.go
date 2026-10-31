// Package httptransport centralizes first-party HTTP transport policy.
package httptransport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Captured only when installing a policy; Configure is startup-only.
var originalDefault http.RoundTripper

var policy struct {
	sync.RWMutex
	proxy *url.URL
}

// Configure installs the explicit proxy policy. Call before starting any workers.
// An empty value preserves the callers' original transport behavior.
func Configure(raw string) error {
	var proxy *url.URL
	var err error
	if raw != "" {
		proxy, err = url.Parse(raw)
		if err != nil || (proxy.Scheme != "http" && proxy.Scheme != "https") || proxy.Hostname() == "" || proxy.Opaque != "" || (proxy.Path != "" && proxy.Path != "/") || proxy.RawQuery != "" || proxy.ForceQuery || proxy.Fragment != "" || strings.HasSuffix(proxy.Host, ":") {
			return errors.New("invalid HTTP proxy configuration")
		}
		if port := proxy.Port(); port != "" {
			n, e := strconv.Atoi(port)
			if e != nil || n < 1 || n > 65535 {
				return errors.New("invalid HTTP proxy configuration")
			}
		}
	}
	resetClients()
	if proxy == nil {
		policy.Lock()
		policy.proxy = nil
		policy.Unlock()
		if originalDefault != nil {
			if c, ok := http.DefaultTransport.(interface{ CloseIdleConnections() }); ok {
				c.CloseIdleConnections()
			}
			http.DefaultTransport = originalDefault
			originalDefault = nil
		}
		return nil
	}
	base := originalDefault
	if base == nil {
		base = http.DefaultTransport
	}
	transport, ok := base.(*http.Transport)
	if !ok {
		return errors.New("cannot configure HTTP proxy with unsupported default transport")
	}
	if originalDefault == nil {
		originalDefault = base
	}
	policy.Lock()
	policy.proxy = proxy
	policy.Unlock()
	if c, ok := http.DefaultTransport.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
	http.DefaultTransport = NewTransport(transport)
	return nil
}

// NewTransport clones the caller's transport without changing its tuning.
func NewTransport(base *http.Transport) http.RoundTripper {
	if base == nil {
		return http.DefaultTransport
	}
	transport := base.Clone()
	policy.RLock()
	proxy := policy.proxy
	policy.RUnlock()
	if proxy != nil {
		transport.Proxy = http.ProxyURL(proxy)
		if proxy.Scheme == "https" {
			transport.DialTLSContext = secureProxyDialer(transport, proxy)
		}
		previous := transport.OnProxyConnectResponse
		transport.OnProxyConnectResponse = func(ctx context.Context, u *url.URL, req *http.Request, resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return ErrProxyConnect
			}
			if previous != nil {
				return previous(ctx, u, req, resp)
			}
			return nil
		}
		return &proxiedTransport{transport}
	}
	return transport
}

// These errors deliberately carry no original error chain: network and proxy
// errors can embed proxy credentials, addresses, or attacker-controlled status text.
var (
	ErrProxyConnect   = errors.New("HTTP proxy connection failed")
	ErrProxyAuth      = errors.New("HTTP proxy authentication required")
	ErrProxiedRequest = errors.New("HTTP request through proxy failed")
)

type proxiedTransport struct{ transport *http.Transport }

func (p *proxiedTransport) CloseIdleConnections() { p.transport.CloseIdleConnections() }
func (p *proxiedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := p.transport.RoundTrip(req)
	if err != nil {
		kind := ErrProxiedRequest
		var op *net.OpError
		if errors.Is(err, ErrProxyConnect) || (errors.As(err, &op) && op.Op == "proxyconnect") {
			kind = ErrProxyConnect
		}
		var ne net.Error
		return nil, &safeError{kind: kind, timeout: errors.As(err, &ne) && ne.Timeout(), canceled: errors.Is(err, context.Canceled), deadline: errors.Is(err, context.DeadlineExceeded)}
	}
	if req.URL.Scheme == "http" && resp.StatusCode == http.StatusProxyAuthRequired {
		resp.Body.Close()
		return nil, ErrProxyAuth
	}
	return resp, nil
}

type safeError struct {
	kind                        error
	timeout, canceled, deadline bool
}

func (e *safeError) Error() string   { return e.kind.Error() }
func (e *safeError) Timeout() bool   { return e.timeout }
func (e *safeError) Temporary() bool { return false }
func (e *safeError) Is(target error) bool {
	return target == e.kind || (e.canceled && target == context.Canceled) || (e.deadline && target == context.DeadlineExceeded)
}
