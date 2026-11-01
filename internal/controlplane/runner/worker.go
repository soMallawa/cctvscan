package runner

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/postfix/cctvscan/internal/controlplane/secrets"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"github.com/riverqueue/river"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

type Worker struct {
	river.WorkerDefaults[store.ScanJobArgs]
	Store                               *store.Store
	Secrets                             *secrets.Box
	Binary, Root, RunnerID, LibraryPath string
}

func (w *Worker) Timeout(*river.Job[store.ScanJobArgs]) time.Duration { return -1 }
func (w *Worker) Work(ctx context.Context, j *river.Job[store.ScanJobArgs]) error {
	if w.Store == nil || !filepath.IsAbs(w.Root) || !filepath.IsAbs(w.Binary) {
		return river.JobCancel(errors.New("runner configuration unavailable"))
	}
	a, e := w.Store.BeginAttempt(ctx, j.Args.JobID, w.RunnerID)
	if errors.Is(e, store.ErrFenced) || errors.Is(e, store.ErrNotFound) {
		return nil
	}
	if errors.Is(e, store.ErrCapacity) {
		return river.JobSnooze(2 * time.Second)
	}
	if e != nil {
		return river.JobCancel(errors.New("runner admission failed"))
	}
	outcome := "failed"
	finish := func() error {
		final, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		e := w.Store.FinishAttempt(final, a, outcome)
		if errors.Is(e, store.ErrFenced) {
			return nil
		}
		if e != nil {
			return errors.New("runner outcome persistence unavailable")
		}
		return nil
	}
	selectedProxy, selectedCredentials, e := w.Store.AttemptSecrets(ctx, a)
	if e != nil {
		return finish()
	}
	proxyURL := ""
	var credentials []byte
	if len(selectedProxy) > 0 {
		if w.Secrets == nil {
			return finish()
		}
		plain, e := w.Secrets.Open("proxy/"+a.Job.Config.ProxyVersionID, selectedProxy)
		if e != nil {
			return finish()
		}
		proxyURL = string(plain)
		clear(plain)
	}
	if len(selectedCredentials) > 0 {
		if w.Secrets == nil {
			return finish()
		}
		credentials, e = w.Secrets.Open("credentials/"+a.Job.Config.CredentialSetID, selectedCredentials)
		if e != nil {
			return finish()
		}
		defer clear(credentials)
	}
	settings, e := w.Store.GetSettings(ctx)
	if e != nil {
		return finish()
	}
	if e = w.Store.Phase(ctx, a, "scanning"); e != nil {
		return finish()
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	monitorDone := make(chan struct{})
	var lost atomic.Bool
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-monitorDone:
				return
			case <-runCtx.Done():
				return
			case <-ticker.C:
				check, cancel := context.WithTimeout(runCtx, 2*time.Second)
				requested, e := w.Store.ActiveAttempt(check, a)
				cancel()
				if e != nil {
					lost.Store(true)
					stop()
					return
				}
				if requested {
					stop()
					return
				}
			}
		}
	}()
	directory := filepath.Join(w.Root, a.JobID, a.ID)
	processErr := (Process{Binary: w.Binary, Directory: directory, ProxyURL: proxyURL, Credentials: credentials, LibraryPath: w.LibraryPath, Policy: &settings.Policy, OnEvent: w.observe(a, directory)}).Run(runCtx, a.Job.Config)
	close(monitorDone)
	// Reconcile complete atomic reports even after scanner exit 1 or cancellation;
	// the report's state and retained host evidence are independent of exit status.
	importCtx, cancelImport := context.WithTimeout(context.Background(), 30*time.Second)
	_ = w.Store.Phase(importCtx, a, "importing")
	reportState, importErr := w.importReport(importCtx, a, directory)
	cancelImport()
	switch {
	case lost.Load() || ctx.Err() != nil:
		outcome = "orphaned"
	case errors.Is(processErr, context.DeadlineExceeded):
		outcome = "timed_out"
	case errors.Is(processErr, context.Canceled):
		outcome = "canceled"
	case importErr == nil:
		switch reportState {
		case "complete":
			if processErr == nil {
				outcome = "completed"
			}
		case "inconclusive":
			outcome = "inconclusive"
		case "canceled":
			outcome = "canceled"
		}
	}
	return finish()
}

// skipValue traverses large unneeded report maps without materializing them.
func skipValue(d *json.Decoder, depth int) error {
	if depth > 64 {
		return errors.New("report nesting exceeds limit")
	}
	token, e := d.Token()
	if e != nil {
		return e
	}
	if delim, ok := token.(json.Delim); ok && (delim == '{' || delim == '[') {
		for d.More() {
			if delim == '{' {
				if _, e = d.Token(); e != nil {
					return e
				}
			}
			if e = skipValue(d, depth+1); e != nil {
				return e
			}
		}
		_, e = d.Token()
	}
	return e
}
func allowedHost(raw string, targets []string) bool {
	a, e := netip.ParseAddr(raw)
	if e != nil {
		return false
	}
	for _, target := range targets {
		if p, e := netip.ParsePrefix(target); e == nil && p.Contains(a) {
			return true
		}
		if b, e := netip.ParseAddr(target); e == nil && b == a {
			return true
		}
	}
	return false
}
func (w *Worker) importReport(ctx context.Context, a store.Attempt, directory string) (string, error) {
	entries, e := os.ReadDir(directory)
	if e != nil {
		return "", e
	}
	name := ""
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "scan-") && strings.HasSuffix(entry.Name(), ".json") {
			if name != "" {
				return "", errors.New("ambiguous scanner reports")
			}
			name = entry.Name()
		}
	}
	if name == "" {
		return "", errors.New("scanner report missing")
	}
	root, e := os.OpenRoot(directory)
	if e != nil {
		return "", e
	}
	defer root.Close()
	file, e := root.Open(name)
	if e != nil {
		return "", e
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > 512<<20 {
		return "", errors.New("scanner report exceeds import bounds")
	}
	d := json.NewDecoder(io.LimitReader(file, 512<<20))
	token, e := d.Token()
	if e != nil || token != json.Delim('{') {
		return "", errors.New("invalid scanner report")
	}
	schema := 0
	state := ""
	seenResults := false
	partial := false
	batch := make([]store.ResultInput, 0, 50)
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		e := w.Store.SaveResults(ctx, a, batch)
		batch = batch[:0]
		return e
	}
	for d.More() {
		if e = ctx.Err(); e != nil {
			return "", e
		}
		key, e := d.Token()
		if e != nil {
			return "", e
		}
		switch key {
		case "SchemaVersion":
			if e = d.Decode(&schema); e != nil {
				return "", e
			}
		case "State":
			if e = d.Decode(&state); e != nil {
				return "", e
			}
		case "Results":
			if seenResults {
				return "", errors.New("duplicate report results")
			}
			seenResults = true
			token, e = d.Token()
			if e != nil || token != json.Delim('[') {
				return "", errors.New("invalid result list")
			}
			for d.More() {
				var raw json.RawMessage
				if e = d.Decode(&raw); e != nil {
					return "", e
				}
				if len(raw) > 1048576 {
					return "", errors.New("result exceeds import bound")
				}
				row, files, incomplete, e := decodeHost(a, raw)
				if e != nil {
					return "", e
				}
				partial = partial || incomplete
				for _, path := range files {
					artifact, e := inspectCapture(directory, a.JobID, a.ID, path)
					if e != nil {
						return "", e
					}
					if e = w.Store.SaveArtifacts(ctx, a, []store.ArtifactInput{artifact}); e != nil {
						return "", e
					}
				}
				batch = append(batch, row)
				if len(batch) == 50 {
					if e = flush(); e != nil {
						return "", e
					}
				}
			}
			if _, e = d.Token(); e != nil {
				return "", e
			}
		default:
			if e = skipValue(d, 0); e != nil {
				return "", e
			}
		}
	}
	if _, e = d.Token(); e != nil {
		return "", e
	}
	var extra any
	if e = d.Decode(&extra); e != io.EOF {
		return "", errors.New("trailing report content")
	}
	if schema != 1 || !seenResults {
		return "", errors.New("unsupported report schema")
	}
	if state != "complete" && state != "inconclusive" && state != "failed" && state != "canceled" {
		return "", errors.New("invalid report outcome")
	}
	if e = flush(); e != nil {
		return "", e
	}
	if state == "complete" && partial {
		state = "inconclusive"
	}
	return state, nil
}
