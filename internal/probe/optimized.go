package probe

import (
	"context"
	"github.com/postfix/cctvscan/internal/httptransport"
	"github.com/postfix/cctvscan/internal/util"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type OptimizedProbeResult struct {
	HTTPMeta    HTTPMeta
	LoginPages  []string
	RTSPInfo    RTSPInfo
	ONVIFResult string
	MJPEGPaths  []string
}

func OptimizedProbe(ctx context.Context, host string, ports []int) OptimizedProbeResult {
	return ProbeWithOptions(ctx, host, ports, false)
}
func ProbeWithOptions(ctx context.Context, host string, ports []int, onvif bool) OptimizedProbeResult {
	var r OptimizedProbeResult
	if ctx.Err() != nil {
		r.HTTPMeta.Error = ctx.Err()
		r.HTTPMeta.State = "canceled"
		return r
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		r.HTTPMeta = ProbeHTTPMeta(ctx, host, FilterHTTPish(ports))
		for _, e := range r.HTTPMeta.Endpoints {
			if loginEvidence(e.StatusCode, e.Auth, e.BodySnippet) {
				r.LoginPages = append(r.LoginPages, e.URL)
			}
		}
	}()
	go func() {
		defer wg.Done()
		if ps := FilterRTSP(ports); len(ps) > 0 {
			r.RTSPInfo = ProbeRTSP(ctx, host, ps)
		}
	}()
	if onvif {
		wg.Add(1)
		go func() { defer wg.Done(); r.ONVIFResult = ProbeONVIF(ctx, host) }()
	}
	wg.Wait()
	return r
}

// These optional discovery helpers use a fixed worker count, never one goroutine
// per port/path. The request budget is shared with all other first-party helpers.
func FindMJPEGPaths(ctx context.Context, host string, ports []int) []string {
	jobs := make(chan string)
	var out []string
	var mu sync.Mutex
	var wg sync.WaitGroup
	client := httptransport.Client(2 * time.Second)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				if ctx.Err() != nil {
					continue
				}
				req, e := http.NewRequestWithContext(ctx, "HEAD", u, nil)
				if e != nil {
					continue
				}
				resp, e := httptransport.Do(client, req)
				if e != nil {
					continue
				}
				ok := resp.StatusCode == 200 && isMJPEGContentType(resp.Header.Get("Content-Type"))
				resp.Body.Close()
				if ok {
					mu.Lock()
					out = append(out, u)
					mu.Unlock()
				}
			}
		}()
	}
send:
	for _, p := range ports {
		scheme := "http"
		if isHTTPS(p) {
			scheme = "https"
		}
		for _, path := range util.Uniq(MJPEGPaths) {
			u := scheme + "://" + net.JoinHostPort(host, util.Itoa(p)) + path
			select {
			case jobs <- u:
			case <-ctx.Done():
				break send
			}
		}
	}
	close(jobs)
	wg.Wait()
	return util.Uniq(out)
}
func isMJPEGContentType(ct string) bool {
	ct = strings.ToLower(ct)
	for _, t := range []string{"image/jpeg", "multipart/x-mixed-replace", "video/mjpeg", "video/x-motion-jpeg"} {
		if strings.Contains(ct, t) {
			return true
		}
	}
	return false
}

// Metadata is reused within each host result, never globally retained or served stale.
func GetCachedHTTPMeta(ctx context.Context, host string, ports []int) HTTPMeta {
	return ProbeHTTPMeta(ctx, host, ports)
}
func OptimizedLoginPageFinder(ctx context.Context, host string, ports []int) []string {
	return FindLoginPages(ctx, host, ports)
}
