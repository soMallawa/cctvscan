package probe

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditHTMLNotCamera(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<html>Public brochure</html>")
	}))
	defer s.Close()
	r := analyzePort(context.Background(), s.Client(), s.URL)
	if r.IsCamera {
		t.Errorf("ordinary HTML page declared camera; indicators=%v", r.Indicators)
	}
}
