package credbrute

import (
	"context"
	"errors"
	"github.com/postfix/cctvscan/internal/connection"
	credentialfile "github.com/postfix/cctvscan/internal/credentials"
	"github.com/postfix/cctvscan/internal/httptransport"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type checkJob struct{ url, credential string }
type CredentialResult struct {
	State     string
	Found     string `json:"-"`
	Attempted int64
}

// firstSuccessful owns, cancels and joins a fixed worker set before returning.
func firstSuccessful(parent context.Context, jobs <-chan checkJob, workers int, check func(context.Context, checkJob) (bool, error)) (string, error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var wg sync.WaitGroup
	var mu sync.Mutex
	found := ""
	var errs []error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case j, ok := <-jobs:
					if !ok {
						return
					}
					if ctx.Err() != nil {
						return
					}
					yes, e := check(ctx, j)
					mu.Lock()
					if yes && found == "" {
						found = j.credential
						cancel()
					} else if e != nil && !errors.Is(e, context.Canceled) {
						errs = append(errs, e)
					}
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	if found != "" {
		return found, nil
	}
	return "", errors.Join(append(errs, parent.Err())...)
}
func CheckCredentials(ctx context.Context, urls []string, file string, timeout time.Duration) (CredentialResult, error) {
	return CheckCredentialsForVendor(ctx, urls, file, "", timeout)
}
func CheckCredentialsForVendor(ctx context.Context, urls []string, file, brand string, timeout time.Duration) (CredentialResult, error) {
	r := CredentialResult{State: "not_tested"}
	creds, e := LoadVendorCredentials(file, brand)
	if e != nil {
		return r, e
	}
	if len(creds) == 0 || len(urls) == 0 {
		return r, nil
	}
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	clientCopy := *httptransport.Client(timeout)
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	client := &clientCopy
	jobs := make(chan checkJob)
	producerDone := make(chan struct{})
	var producerErr error
	var attempts atomic.Int64
	var challenges sync.Map
	go func() {
		defer close(producerDone)
		defer close(jobs)
		for _, u := range urls {
			if work.Err() != nil {
				return
			}
			challenge, err := authChallenge(work, client, u)
			if err != nil {
				producerErr = errors.Join(producerErr, err)
				continue
			}
			if challenge == "" {
				continue
			}
			challenges.Store(u, challenge)
			for _, c := range creds {
				select {
				case jobs <- checkJob{u, c}:
				case <-work.Done():
					return
				}
			}
		}
	}()
	found, err := firstSuccessful(work, jobs, 2, func(ctx context.Context, j checkJob) (bool, error) {
		attempts.Add(1)
		challenge, _ := challenges.Load(j.url)
		return checkCredentialChallenge(ctx, client, j.url, j.credential, challenge.(string))
	})
	cancel()
	<-producerDone
	r.Attempted = attempts.Load()
	r.Found = found
	if found != "" {
		r.State = "found"
		return r, nil
	}
	err = errors.Join(err, producerErr, ctx.Err())
	switch {
	case ctx.Err() != nil:
		r.State = "canceled"
	case err != nil:
		r.State = "inconclusive"
	case r.Attempted > 0:
		r.State = "no_match"
	}
	return r, err
}
func OptimizedBruteForce(ctx context.Context, host string, urls []string, file string, timeout time.Duration) string {
	r, _ := CheckCredentials(ctx, urls, file, timeout)
	return r.Found
}
func loadCredentials(path string) ([]string, error) { return LoadVendorCredentials(path, "") }
func LoadVendorCredentials(path, brand string) ([]string, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if e != nil {
		return nil, e
	}
	set, e := credentialfile.Parse(b)
	if e != nil {
		return nil, e
	}
	out := []string{}
	for _, entry := range set.Select(brand) {
		out = append(out, entry.Pair)
	}
	return out, nil
}
func requiresBasicAuth(ctx context.Context, client *http.Client, u string) (bool, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return false, e
	}
	r, e := httptransport.Do(client, req)
	if e != nil {
		return false, e
	}
	defer r.Body.Close()
	return r.StatusCode == 401 && strings.HasPrefix(strings.ToLower(strings.TrimSpace(r.Header.Get("WWW-Authenticate"))), "basic"), nil
}

func checkCredential(ctx context.Context, client *http.Client, u, c string) (bool, error) {
	return checkCredentialChallenge(ctx, client, u, c, `Basic realm="camera"`)
}
func checkCredentialChallenge(ctx context.Context, client *http.Client, u, c, challenge string) (bool, error) {
	a, b, ok := strings.Cut(c, ":")
	if !ok {
		return false, errors.New("invalid credential format")
	}
	parsed, e := url.Parse(u)
	if e != nil {
		return false, errors.New("invalid credential endpoint")
	}
	header, _, ok := connection.ChallengeAuthorization(challenge, "GET", parsed.RequestURI(), a, b)
	if !ok {
		return false, errors.New("unsupported authentication method")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return false, e
	}
	req.Header.Set("Authorization", header)
	r, e := httptransport.Do(client, req)
	if e != nil {
		return false, e
	}
	defer r.Body.Close()
	return r.StatusCode >= 200 && r.StatusCode < 300, nil
}
func authChallenge(ctx context.Context, client *http.Client, u string) (string, error) {
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return "", e
	}
	r, e := httptransport.Do(client, req)
	if e != nil {
		return "", e
	}
	defer r.Body.Close()
	if r.StatusCode != 401 {
		return "", nil
	}
	challenge := connection.ChooseChallenge(r.Header.Values("WWW-Authenticate"))
	if _, _, ok := connection.ChallengeAuthorization(challenge, "GET", req.URL.RequestURI(), "", ""); !ok {
		return "", errors.New("unsupported authentication method")
	}
	return challenge, nil
}
