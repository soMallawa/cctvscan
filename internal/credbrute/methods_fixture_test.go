package credbrute

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These owned fixtures document current support, without adding login methods.
func TestCurrentCredentialMethodsAgainstOwnedFixtures(t *testing.T) {
	for _, tt := range []struct {
		name, expected string
		form, correct  bool
	}{
		{"basic correct", "found", false, true},
		{"basic wrong", "no_match", false, false},
		{"HTML form is unsupported", "not_tested", true, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.form {
					_, _ = w.Write([]byte(`<form><input type="password" name="password"></form>`))
					return
				}
				u, p, ok := r.BasicAuth()
				if ok && u == "fixture-user" && p == "fixture-password" && tt.correct {
					_, _ = w.Write([]byte("authenticated fixture resource"))
					return
				}
				w.Header().Set("WWW-Authenticate", `Basic realm="owned fixture"`)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer s.Close()
			file := filepath.Join(t.TempDir(), "fixture.txt")
			if err := os.WriteFile(file, []byte("fixture-user:fixture-password\n"), 0600); err != nil {
				t.Fatal(err)
			}
			r, err := CheckCredentials(context.Background(), []string{s.URL}, file, time.Second)
			if err != nil || r.State != tt.expected {
				t.Fatalf("state=%s error=%v", r.State, err)
			}
			if tt.form && r.Attempted != 0 {
				t.Fatal("unsupported HTML form must not be labelled password-tested")
			}
			if !tt.form && r.Attempted != 1 {
				t.Fatalf("expected one owned-fixture attempt; got %d", r.Attempted)
			}
		})
	}
}
