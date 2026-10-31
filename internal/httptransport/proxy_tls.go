package httptransport

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"net/url"
	"time"
)

// secureProxyDialer separates proxy TLS authentication from camera TLS policy.
// net/http otherwise reuses TLSClientConfig for BOTH the proxy and target.
func secureProxyDialer(transport *http.Transport, proxy *url.URL) func(context.Context, string, string) (net.Conn, error) {
	config := &tls.Config{ServerName: proxy.Hostname(), MinVersion: tls.VersionTLS12}
	if transport.TLSClientConfig != nil {
		config.RootCAs = transport.TLSClientConfig.RootCAs
	}
	dial := transport.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	timeout := transport.TLSHandshakeTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		secured := tls.Client(conn, config)
		handshakeCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := secured.HandshakeContext(handshakeCtx); err != nil {
			conn.Close()
			return nil, err
		}
		return secured, nil
	}
}
