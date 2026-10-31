package processor

import (
	"context"
	"errors"
	"fmt"
	"github.com/postfix/cctvscan/internal/credbrute"
	"github.com/postfix/cctvscan/internal/fingerprint"
	"github.com/postfix/cctvscan/internal/httptransport"
	"github.com/postfix/cctvscan/internal/probe"
	"github.com/postfix/cctvscan/internal/streams"
	"github.com/postfix/cctvscan/internal/util"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type StageError struct{ Stage, Message string }
type HostResult struct {
	Host                        string
	Ports, HTTPPorts, RTSPPorts []int
	HTTPMeta                    probe.HTTPMeta
	LoginPages                  []string
	RTSPInfo                    probe.RTSPInfo
	ONVIFResult                 string
	MJPEGPaths                  []string
	Brand, BrandNote            string
	CVEs                        []string
	Credentials                 string `json:"-"`
	CredentialState             string
	CameraDetection             probe.CameraDetectionResult
	CameraDetections            []probe.CameraDetectionResult
	StreamResults               []streams.StreamResult
	State                       string
	Errors                      []StageError
	Error                       error `json:"-"`
}
type Options struct {
	Workers                          int
	HostTimeout                      time.Duration
	CheckCredentials, Capture, ONVIF bool
}
type OptimizedProcessor struct {
	debug                bool
	credsFile, outputDir string
	Options              Options
}

func NewOptimizedProcessor(debug bool, file, dir string) *OptimizedProcessor {
	return &OptimizedProcessor{debug: debug, credsFile: file, outputDir: dir, Options: Options{Workers: 5, HostTimeout: 30 * time.Second}}
}

// One bounded worker pool processes hosts; cancellation marks queued work without
// launching network activity, retaining a truthful outcome for every discovered host.
func (p *OptimizedProcessor) ProcessHosts(ctx context.Context, hosts map[string][]int) []HostResult {
	keys := make([]string, 0, len(hosts))
	for h := range hosts {
		keys = append(keys, h)
	}
	sort.Strings(keys)
	out := make([]HostResult, len(keys))
	jobs := make(chan int)
	var wg sync.WaitGroup
	workers := min(max(1, p.Options.Workers), len(keys))
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := range jobs {
				out[n] = p.processHost(ctx, keys[n], hosts[keys[n]])
			}
		}()
	}
	for n := range keys {
		jobs <- n
	}
	close(jobs)
	wg.Wait()
	return out
}
func (r *HostResult) fail(stage string, e error) {
	if e == nil {
		return
	}
	r.Error = errors.Join(r.Error, e)
	msg := "operation failed"
	switch {
	case errors.Is(e, context.Canceled):
		msg = "canceled"
		r.State = "canceled"
	case errors.Is(e, context.DeadlineExceeded):
		msg = "deadline exceeded"
		r.State = "canceled"
	case errors.Is(e, httptransport.ErrProxyAuth):
		msg = "proxy authentication required"
	case errors.Is(e, httptransport.ErrProxyConnect):
		msg = "proxy connection failed"
	case errors.Is(e, httptransport.ErrProxiedRequest):
		msg = "request through proxy failed"
	}
	r.Errors = append(r.Errors, StageError{stage, msg})
	if r.State != "canceled" {
		r.State = "inconclusive"
	}
}
func (p *OptimizedProcessor) processHost(parent context.Context, host string, ports []int) HostResult {
	ctx := parent
	cancel := func() {}
	if p.Options.HostTimeout > 0 {
		ctx, cancel = context.WithTimeout(parent, p.Options.HostTimeout)
	}
	defer cancel()
	r := HostResult{Host: host, Ports: append([]int(nil), ports...), State: "complete", CredentialState: "not_tested"}
	sort.Ints(r.Ports)
	r.HTTPPorts = probe.FilterHTTPish(r.Ports)
	r.RTSPPorts = probe.FilterRTSP(r.Ports)
	if ctx.Err() != nil {
		r.fail("host", ctx.Err())
		return r
	}
	pr := probe.ProbeWithOptions(ctx, host, r.Ports, p.Options.ONVIF)
	r.HTTPMeta = pr.HTTPMeta
	r.RTSPInfo = pr.RTSPInfo
	if pr.RTSPInfo.Error != nil {
		r.fail("rtsp_probe", pr.RTSPInfo.Error)
	}
	r.ONVIFResult = pr.ONVIFResult
	r.LoginPages = pr.LoginPages
	r.fail("http_metadata", pr.HTTPMeta.Error)
	r.Brand, r.BrandNote = fingerprint.OptimizedDetect(pr.HTTPMeta.Server, pr.HTTPMeta.BodySnippet, pr.RTSPInfo.Server)
	best := -1
	for _, e := range pr.HTTPMeta.Endpoints {
		if e.State == "failed" {
			continue
		}
		d := probe.CameraFromEndpoint(e)
		r.CameraDetections = append(r.CameraDetections, d)
		b, n := fingerprint.OptimizedDetect(e.Server, e.BodySnippet, "")
		if b != "" && b != "Unknown cam" && (r.Brand == "" || r.Brand == "Unknown cam") {
			r.Brand = b
			r.BrandNote = n
		}
		score := 0
		if d.IsCamera {
			score = 1
		}
		if d.Brand != "" && d.Brand != "Unknown cam" {
			score += 2
		}
		if score > best {
			r.CameraDetection = d
			best = score
		}
	}
	r.CVEs = fingerprint.OptimizedCVEsForBrand(r.Brand)
	if ctx.Err() != nil {
		r.fail("host", ctx.Err())
		return r
	}
	if p.Options.CheckCredentials {
		pages, discoveryErr := probe.FindLoginPagesDetailed(ctx, host, r.HTTPPorts)
		r.LoginPages = util.Uniq(append(r.LoginPages, pages...))
		cr, e := credbrute.CheckCredentials(ctx, r.LoginPages, p.credsFile, 5*time.Second)
		r.Credentials = cr.Found
		r.CredentialState = cr.State
		if discoveryErr != nil && cr.State != "found" && cr.State != "canceled" {
			r.CredentialState = "inconclusive"
		}
		r.fail("credentials", errors.Join(discoveryErr, e))
	}
	// Deep path discovery is reserved for likely camera hosts. Non-HTTP protocol
	// paths remain labeled candidates, never fabricated success or playback proof.
	selected := append([]int(nil), r.RTSPPorts...)
	if r.CameraDetection.IsCamera || r.RTSPInfo.Any || r.ONVIFResult != "" {
		selected = append(selected, r.HTTPPorts...)
	}
	for _, port := range r.Ports {
		if (port >= 1935 && port <= 1939) || (port >= 1755 && port <= 1760) {
			selected = append(selected, port)
		}
	}
	sr, e := streams.DetectStreamsDetailed(ctx, host, selected)
	r.StreamResults = sr
	r.fail("media_discovery", e)
	if p.Options.Capture {
		client := httptransport.Client(3 * time.Second)
		for i := range r.StreamResults {
			s := &r.StreamResults[i]
			if s.Verified && (s.Type == streams.StreamMJPEG || s.Type == streams.StreamSnapshot) {
				path, e := streams.SaveSnapshot(ctx, client, *s, filepath.Join(p.outputDir, "snapshots"))
				s.SavedPath = path
				r.fail("capture", e)
			}
		}
	}
	if ctx.Err() != nil {
		r.fail("host", ctx.Err())
	}
	return r
}
func (p *OptimizedProcessor) PrintResults(results []HostResult) {
	for _, r := range results {
		fmt.Printf("\n=== %s [%s] ===\nOpen ports: %v\n", r.Host, r.State, r.Ports)
		for _, e := range r.HTTPMeta.Endpoints {
			fmt.Printf("HTTP %s: %s status=%d server=%q\n", e.URL, e.State, e.StatusCode, e.Server)
		}
		if r.RTSPInfo.Any {
			fmt.Printf("RTSP response: status=%d auth_required=%t server=%q (not playback proof)\n", r.RTSPInfo.Status, r.RTSPInfo.AuthRequired, r.RTSPInfo.Server)
		}
		if r.Brand != "" {
			fmt.Printf("Vendor hint: %s %s\n", r.Brand, r.BrandNote)
		}
		if len(r.CVEs) > 0 {
			fmt.Printf("Vendor-associated advisories (model/firmware applicability NOT verified): %s\n", strings.Join(fingerprint.OptimizedCVELinks(r.CVEs), ", "))
		}
		if r.CameraDetection.IsCamera {
			fmt.Printf("Likely camera: %s; evidence=%v\n", r.CameraDetection.Brand, r.CameraDetection.Indicators)
		}
		fmt.Printf("Credentials: %s", r.CredentialState)
		if r.Credentials != "" {
			fmt.Print(" (value redacted)")
		}
		fmt.Println()
		for _, s := range r.StreamResults {
			fmt.Printf("Media %s [%s]: %s; %s\n", s.Type, s.State, s.URL, s.Evidence)
			if s.SavedPath != "" {
				fmt.Printf("Saved image: %s\n", s.SavedPath)
			}
		}
		if r.ONVIFResult != "" {
			fmt.Printf("ONVIF: %s\n", r.ONVIFResult)
		}
		for _, e := range r.Errors {
			fmt.Printf("Incomplete %s: %s\n", e.Stage, e.Message)
		}
	}
}
func GetPerformanceStats() map[string]interface{} {
	n, c := fingerprint.GetCacheStats()
	return map[string]interface{}{"brand_cache_entries": n, "cve_cache_entries": c}
}
