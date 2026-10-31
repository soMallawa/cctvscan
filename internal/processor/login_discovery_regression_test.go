package processor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/postfix/cctvscan/internal/httptransport"
)

type loginDiscoveryTransport func(*http.Request) (*http.Response, error)

func (f loginDiscoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type loginReadFailure struct{ err error }

func (r loginReadFailure) Read([]byte) (int, error) { return 0, r.err }
func (r loginReadFailure) Close() error             { return nil }

func TestCredentialDiscoveryFailurePreservesRootEvidence(t *testing.T) {
	for _, failure := range []string{"request", "read"} {
		t.Run(failure, func(t *testing.T) {
			sentinel := errors.New("private upstream token=DO_NOT_REPORT")
			rootRequests, authRequests := 0, 0
			transport := loginDiscoveryTransport(func(req *http.Request) (*http.Response, error) {
				if req.Header.Get("Authorization") != "" {
					authRequests++
					return nil, errors.New("credential attempt forbidden")
				}
				body := "ordinary page"
				if req.URL.Path == "/" {
					rootRequests++
					body = `<html><input type="password"></html>`
					// Initial metadata succeeds; the subsequent login-discovery root fails.
					// A later unauthenticated preflight succeeds without a Basic challenge.
					if rootRequests == 2 {
						if failure == "request" {
							return nil, sentinel
						}
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: loginReadFailure{sentinel}, Request: req}, nil
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			for _, timeout := range []time.Duration{2 * time.Second, 1500 * time.Millisecond, 5 * time.Second} {
				client := httptransport.Client(timeout)
				old := client.Transport
				client.Transport = transport
				defer func() { client.Transport = old }()
			}
			dir := t.TempDir()
			creds := filepath.Join(dir, "empty-credentials.txt")
			if err := os.WriteFile(creds, []byte("# No credential attempts permitted\n"), 0600); err != nil {
				t.Fatal(err)
			}
			p := NewOptimizedProcessor(false, creds, dir)
			p.Options.CheckCredentials = true
			got := p.processHost(context.Background(), "fixture.invalid", []int{80})
			if got.HTTPMeta.Error != nil || got.HTTPMeta.Endpoints[0].State != "responded" {
				t.Fatalf("initial root did not succeed: %+v", got.HTTPMeta)
			}
			if !reflect.DeepEqual(got.LoginPages, []string{"http://fixture.invalid:80/"}) {
				t.Errorf("successful root login evidence discarded: %v", got.LoginPages)
			}
			if got.State != "inconclusive" || got.CredentialState != "inconclusive" {
				t.Errorf("requested discovery failure collapsed: host=%s credentials=%s", got.State, got.CredentialState)
			}
			if !errors.Is(got.Error, sentinel) {
				t.Errorf("discovery cause not propagated: %v", got.Error)
			}
			if len(got.Errors) != 1 || got.Errors[0].Stage != "credentials" || got.Errors[0].Message != "operation failed" {
				t.Errorf("missing/sensitive discovery stage error: %+v", got.Errors)
			}
			if authRequests != 0 {
				t.Fatalf("made %d credential attempts", authRequests)
			}
			if rootRequests < 2 {
				t.Fatal("subsequent discovery fixture not exercised")
			}
		})
	}
}
