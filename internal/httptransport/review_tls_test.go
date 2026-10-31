package httptransport

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// Camera certificate compatibility must not disable authentication of an HTTPS
// proxy: otherwise credentials can be sent to an untrusted proxy endpoint.
func TestHTTPSProxyRejectsUntrustedCertificateEvenWithInsecureTargetTLS(t *testing.T) {
	var received atomic.Int32
	proxy := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	if err := Configure(proxy.URL); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := Configure(""); err != nil {
			t.Error(err)
		}
	})
	transport := NewTransport(&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}})
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
	defer client.CloseIdleConnections()
	resp, err := client.Get("http://target.invalid/")
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil || received.Load() != 0 {
		t.Fatalf("untrusted HTTPS proxy accepted with target TLS compatibility enabled: err=%v proxy requests=%d", err, received.Load())
	}
}
