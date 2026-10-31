package httptransport

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPSConnectViaHTTPAndHTTPSProxy(t *testing.T) {
	for _, secureProxy := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[secureProxy], func(t *testing.T) {
			var connects, origins atomic.Int32
			origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				origins.Add(1)
				if r.Header.Get("Proxy-Authorization") != "" {
					t.Error("proxy auth reached origin")
				}
				w.Write([]byte("target"))
			}))
			defer origin.Close()
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "CONNECT" {
					t.Error("expected CONNECT")
					w.WriteHeader(500)
					return
				}
				connects.Add(1)
				if r.Header.Get("Proxy-Authorization") != "Basic "+base64.StdEncoding.EncodeToString([]byte("user:password")) {
					t.Error("missing proxy auth")
				}
				upstream, err := net.Dial("tcp", origin.Listener.Addr().String())
				if err != nil {
					t.Error(err)
					return
				}
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					upstream.Close()
					t.Error(err)
					return
				}
				defer conn.Close()
				defer upstream.Close()
				rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
				rw.Flush()
				done := make(chan struct{})
				go func() { io.Copy(upstream, rw); upstream.Close(); close(done) }()
				io.Copy(conn, upstream)
				conn.Close()
				<-done
			})
			var proxy *httptest.Server
			if secureProxy {
				proxy = httptest.NewTLSServer(handler)
			} else {
				proxy = httptest.NewServer(handler)
			}
			defer proxy.Close()
			if err := Configure(strings.Replace(proxy.URL, "://", "://user:password@", 1)); err != nil {
				t.Fatal(err)
			}
			defer Configure("")
			roots := x509.NewCertPool()
			if secureProxy {
				roots.AddCert(proxy.Certificate())
			}
			tr := NewTransport(&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, RootCAs: roots}, DisableKeepAlives: true})
			client := &http.Client{Transport: tr, Timeout: time.Second * 3}
			resp, err := client.Get(origin.URL)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if string(b) != "target" || connects.Load() != 1 || origins.Load() != 1 {
				t.Fatalf("body=%q connects=%d origins=%d", b, connects.Load(), origins.Load())
			}
		})
	}
}

func TestNoProxyPreservesTransport(t *testing.T) {
	Configure("")
	base := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DisableKeepAlives: true, MaxIdleConns: 7, IdleConnTimeout: time.Second, ResponseHeaderTimeout: 2 * time.Second, TLSHandshakeTimeout: 3 * time.Second}
	tr := NewTransport(base).(*http.Transport)
	if tr == base || tr.Proxy != nil || !tr.DisableKeepAlives || tr.MaxIdleConns != 7 || tr.IdleConnTimeout != base.IdleConnTimeout || tr.ResponseHeaderTimeout != base.ResponseHeaderTimeout || tr.TLSHandshakeTimeout != base.TLSHandshakeTimeout || !tr.TLSClientConfig.InsecureSkipVerify {
		t.Fatal("caller tuning changed")
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer origin.Close()
	client := &http.Client{Transport: tr}
	resp, err := client.Get(origin.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if NewTransport(nil) != http.DefaultTransport {
		t.Fatal("default behavior changed")
	}
}
