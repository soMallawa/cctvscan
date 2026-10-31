package probe

import (
	"context"
	"github.com/postfix/cctvscan/internal/httptransport"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
)

func TestAuditPublicPageNotLogin(t *testing.T) {
	auditProxy(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>Public brochure</html>")
	})
	r := FindLoginPages(context.Background(), "fixture.invalid", []int{80})
	if len(r) > 0 {
		t.Errorf("public brochure HTTP 200 is reported as %d login pages", len(r))
	}
}
func TestAuditMetadataNotFirstPortOnly(t *testing.T) {
	var second atomic.Int32
	auditProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Port() == "8080" {
			second.Add(1)
			w.Header().Set("Server", "Hikvision")
			io.WriteString(w, "Hikvision camera")
		} else {
			w.Header().Set("Server", "nginx")
			io.WriteString(w, "ordinary website")
		}
	})
	r := ProbeHTTPMeta(context.Background(), "fixture.invalid", []int{80, 8080})
	if second.Load() == 0 {
		t.Errorf("second HTTP port never probed; metadata only %q", r.Server)
	}
}
func TestAuditHeadUnsupportedFallback(t *testing.T) {
	auditProxy(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "HEAD" {
			w.WriteHeader(405)
		} else {
			w.WriteHeader(401)
		}
	})
	r := FindLoginPages(context.Background(), "fixture.invalid", []int{80})
	if len(r) == 0 {
		t.Error("HEAD 405 prevents discovery of GET 401 authentication challenge")
	}
}
func TestAuditProxyFailureNotEmptyResult(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	if e := httptransport.Configure(s.URL); e != nil {
		t.Fatal(e)
	}
	defer httptransport.Configure("")
	good := ProbeHTTPMeta(context.Background(), "fixture.invalid", []int{80})
	s.Close()
	failed := ProbeHTTPMeta(context.Background(), "fixture.invalid", []int{80})
	if reflect.DeepEqual(good, failed) {
		t.Error("unreachable proxy and successful empty response both produce indistinguishable empty metadata")
	}
}
