package credbrute

import (
	"bufio"
	"context"
	"errors"
	"github.com/postfix/cctvscan/internal/httptransport"
	"net/http"
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
	r := CredentialResult{State: "not_tested"}
	creds, e := loadCredentials(file)
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
	go func() {
		defer close(producerDone)
		defer close(jobs)
		for _, u := range urls {
			if work.Err() != nil {
				return
			}
			ok, err := requiresBasicAuth(work, client, u)
			if err != nil {
				producerErr = errors.Join(producerErr, err)
				continue
			}
			if !ok {
				continue
			}
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
		return checkCredential(ctx, client, j.url, j.credential)
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
func loadCredentials(path string) ([]string, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if stat.Size() > 1024*1024 {
		return nil, errors.New("credential file exceeds 1 MiB limit")
	}
	var out []string
	seen := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		s := strings.TrimSpace(sc.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		if !strings.Contains(s, ":") {
			return nil, errors.New("invalid credential file format")
		}
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out, sc.Err()
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
	a, b, ok := strings.Cut(c, ":")
	if !ok {
		return false, errors.New("invalid credential format")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return false, e
	}
	req.SetBasicAuth(a, b)
	r, e := httptransport.Do(client, req)
	if e != nil {
		return false, e
	}
	defer r.Body.Close()
	return r.StatusCode == 200, nil
}
