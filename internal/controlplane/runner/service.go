//go:build linux

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"os"
	"strconv"
	"strings"
	"time"
)

func (w *Worker) Run(ctx context.Context) error {
	if w.Store == nil {
		return errors.New("worker database unavailable")
	}
	if w.RunnerID == "" {
		w.RunnerID = uuid.NewString()
	}
	if e := w.Store.RegisterRunner(ctx, w.RunnerID); e != nil {
		return errors.New("runner registration unavailable")
	}
	hostname, _ := os.Hostname()
	beat := func() error {
		stats := map[string]any{"name": "CONNECT runner", "hostname": hostname, "rss_bytes": nil, "cpu_percent": nil}
		if b, e := os.ReadFile("/proc/self/statm"); e == nil {
			parts := strings.Fields(string(b))
			if len(parts) > 1 {
				if n, e := strconv.ParseUint(parts[1], 10, 64); e == nil {
					stats["rss_bytes"] = n * uint64(os.Getpagesize())
				}
			}
		}
		raw, _ := json.Marshal(stats)
		check, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if e := w.Store.RunnerHeartbeat(check, w.RunnerID, raw); e != nil {
			return e
		}
		if e := w.Store.RecoverOrphans(check); e != nil {
			return e
		}
		_, e := w.Store.DispatchSchedules(check, time.Now())
		return e
	}
	if e := beat(); e != nil {
		return errors.New("runner heartbeat unavailable")
	}
	client, e := w.Store.WorkerClient(w)
	if e != nil {
		return errors.New("queue initialization unavailable")
	}
	if e = client.Start(ctx); e != nil {
		return errors.New("queue start unavailable")
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	shutdown := func() error {
		stop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e := client.StopAndCancel(stop); e != nil {
			return errors.New("worker shutdown incomplete")
		}
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return shutdown()
		case <-ticker.C:
			if e = beat(); e != nil {
				_ = shutdown()
				return errors.New("runner lost database heartbeat")
			}
		}
	}
}
