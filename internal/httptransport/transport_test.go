package httptransport

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProxyFailuresAreRedactedAndNeverFallBack(t *testing.T) {
	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1) }))
	defer target.Close()
	for _, scenario := range []string{"forward-auth", "connect-rejected", "closed"} {
		t.Run(scenario, func(t *testing.T) {
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "CONNECT" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					defer conn.Close()
					fmt.Fprint(conn, "HTTP/1.1 502 secret-password-proxy-url\r\nContent-Length: 0\r\n\r\n")
					return
				}
				w.WriteHeader(407)
			}))
			defer proxy.Close()
			raw := strings.Replace(proxy.URL, "://", "://secret-user:secret-password@", 1)
			if err := Configure(raw); err != nil {
				t.Fatal(err)
			}
			defer Configure("")
			if scenario == "closed" {
				proxy.Close()
			}
			client := &http.Client{Transport: NewTransport(&http.Transport{})}
			u := target.URL
			if scenario == "connect-rejected" {
				u = strings.Replace(u, "http:", "https:", 1)
			}
			resp, err := client.Get(u)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil {
				t.Fatal("proxy failure treated as target response")
			}
			for _, secret := range []string{"secret-user", "secret-password", proxy.URL, strings.TrimPrefix(proxy.URL, "http://")} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("leaked proxy detail: %v", err)
				}
			}
		})
	}
	if direct.Load() != 0 {
		t.Fatal("direct fallback reached target")
	}
}

func TestUnconfiguredDefaultIsUntouched(t *testing.T) {
	Configure("")
	original := http.DefaultTransport
	custom := &http.Transport{DisableKeepAlives: true}
	http.DefaultTransport = custom
	defer func() { http.DefaultTransport = original }()
	if err := Configure(""); err != nil {
		t.Fatal(err)
	}
	if http.DefaultTransport != custom {
		t.Fatal("unconfigured default transport was replaced")
	}
}

func TestDefaultTransportUsesProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer proxy.Close()
	original := http.DefaultTransport
	if err := Configure(proxy.URL); err != nil {
		t.Fatal(err)
	}
	defer Configure("")
	for _, transport := range []http.RoundTripper{nil, NewTransport(nil)} {
		client := &http.Client{Transport: transport}
		resp, err := client.Get("http://default.invalid/")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	Configure("")
	if http.DefaultTransport != original {
		t.Fatal("default not restored")
	}
}

func TestInvalidProxyIsRedacted(t *testing.T) {
	for _, raw := range []string{"not-a-url", "socks5://secret:password@host:80", "http://", "http://secret:password@host:99999", "http://host:", "http://host/path", "http://host?password=secret", "http://host#secret", "http://secret:%zz@host", " http://host", "http://host with space"} {
		err := Configure(raw)
		if err == nil {
			t.Errorf("accepted malformed proxy case")
			continue
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), raw) {
			t.Errorf("configuration error leaked input: %s", err)
		}
	}
	for _, raw := range []string{"http://user:pass@localhost:8080", "https://localhost", "http://[::1]:8080"} {
		if err := Configure(raw); err != nil {
			t.Fatal(err)
		}
	}
	Configure("")
}

func TestExplicitProxyMethodsAndRedirects(t *testing.T) {
	t.Setenv("NO_PROXY", "*")
	var seen []string
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		seen = append(seen, r.Method+" "+r.URL.String()+" "+string(b))
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "http://other.invalid/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	if err := Configure(proxy.URL); err != nil {
		t.Fatal(err)
	}
	defer Configure("")
	client := &http.Client{Transport: NewTransport(&http.Transport{})}
	for _, method := range []string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "TRACE", "PROPFIND"} {
		req, _ := http.NewRequest(method, "http://target.invalid/path", strings.NewReader("payload"))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	resp, err := client.Get("http://target.invalid/redirect")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	want := []string{"GET http://target.invalid/path payload", "HEAD http://target.invalid/path payload", "POST http://target.invalid/path payload", "PUT http://target.invalid/path payload", "PATCH http://target.invalid/path payload", "DELETE http://target.invalid/path payload", "OPTIONS http://target.invalid/path payload", "TRACE http://target.invalid/path payload", "PROPFIND http://target.invalid/path payload", "GET http://target.invalid/redirect ", "GET http://other.invalid/final "}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests = %q", seen)
	}
}
