package probe

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/postfix/cctvscan/internal/httptransport"
)

type discoveryTransport func(*http.Request) (*http.Response, error)

func (f discoveryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type discoveryReadFailure struct{ err error }

func (r discoveryReadFailure) Read([]byte) (int, error) { return 0, r.err }
func (r discoveryReadFailure) Close() error             { return nil }

func TestLoginDiscoveryDetailedPartialEvidence(t *testing.T) {
	for _, failure := range []string{"request", "read", "cancel"} {
		t.Run(failure, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sentinel := errors.New("fixture discovery failure")
			client := httptransport.Client(1500 * time.Millisecond)
			old := client.Transport
			defer func() { client.Transport = old }()
			client.Transport = discoveryTransport(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/login" {
					switch failure {
					case "request":
						return nil, sentinel
					case "read":
						return &http.Response{StatusCode: 200, Header: make(http.Header), Body: discoveryReadFailure{sentinel}, Request: req}, nil
					case "cancel":
						cancel()
						return nil, context.Canceled
					}
				}
				body := "ordinary page"
				if req.URL.Path == "/" {
					body = `<input type="password">`
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
			})
			pages, err := FindLoginPagesDetailed(ctx, "fixture.invalid", []int{80})
			if !reflect.DeepEqual(pages, []string{"http://fixture.invalid:80/"}) {
				t.Fatalf("partial positive evidence lost: %v", pages)
			}
			cause := sentinel
			if failure == "cancel" {
				cause = context.Canceled
			}
			if !errors.Is(err, cause) {
				t.Fatalf("missing discovery failure: %v", err)
			}
		})
	}
}
