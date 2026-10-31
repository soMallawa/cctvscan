package httptransport

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigureFromEnv(t *testing.T) {
	t.Setenv("CCTVSCAN_HTTP_PROXY", "")
	if err := ConfigureFromEnv(""); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureFromEnv("MISSING_PROXY_FOR_TEST"); err == nil {
		t.Fatal("missing selected env accepted")
	}
	t.Setenv("SELECTED_PROXY", "")
	if err := ConfigureFromEnv("SELECTED_PROXY"); err == nil {
		t.Fatal("empty selected env accepted")
	}
	t.Setenv("CCTVSCAN_HTTP_PROXY", "malformed-secret")
	if err := ConfigureFromEnv(""); err == nil {
		t.Fatal("invalid default accepted")
	}
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer proxy.Close()
	t.Setenv("SELECTED_PROXY", proxy.URL)
	if err := ConfigureFromEnv("SELECTED_PROXY"); err != nil {
		t.Fatal(err)
	}
	defer Configure("")
	client := &http.Client{Transport: NewTransport(&http.Transport{})}
	resp, err := client.Get("http://local-fixture.invalid/")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	t.Setenv("CCTVSCAN_HTTP_PROXY", proxy.URL)
	if err := ConfigureFromEnv(""); err != nil {
		t.Fatal(err)
	}
}
