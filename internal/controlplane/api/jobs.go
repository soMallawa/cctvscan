package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) jobRoutes() {
	for pattern, h := range map[string]http.HandlerFunc{"GET /api/jobs": s.jobs, "POST /api/jobs": s.createJob, "GET /api/jobs/{id}": s.getJob, "POST /api/jobs/{id}/cancel": s.cancelJob, "POST /api/jobs/{id}/retry": s.retryJob, "DELETE /api/jobs/{id}": s.deleteJob, "GET /api/jobs/{id}/results": s.results, "GET /api/results": s.results, "GET /api/jobs/{id}/export": s.export, "GET /api/overview": s.overview} {
		s.mux.HandleFunc(pattern, validatedID(h))
	}
}
func validatedID(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if id := r.PathValue("id"); id != "" {
			if _, e := uuid.Parse(id); e != nil {
				fail(w, 400, "use a valid resource ID", "id")
				return
			}
		}
		h(w, r)
	}
}
func page(w http.ResponseWriter, r *http.Request) (int, int, bool) {
	limit, offset := 50, 0
	var e error
	if v := r.URL.Query().Get("limit"); v != "" {
		limit, e = strconv.Atoi(v)
		if e != nil || limit < 1 || limit > 200 {
			fail(w, 400, "limit must be 1-200", "limit")
			return 0, 0, false
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		offset, e = strconv.Atoi(v)
		if e != nil || offset < 0 || offset > 100000 {
			fail(w, 400, "offset must be 0-100000", "offset")
			return 0, 0, false
		}
	}
	return limit, offset, true
}
func list(w http.ResponseWriter, items any, total int64) {
	write(w, 200, map[string]any{"items": items, "total": total})
}
func (s *Server) jobs(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := page(w, r)
	if !ok {
		return
	}
	in := store.ListJobsInput{Search: r.URL.Query().Get("search"), State: r.URL.Query().Get("state"), Limit: limit, Offset: offset}
	items, e := s.options.Store.ListJobs(r.Context(), in)
	if e != nil {
		handleError(w, e)
		return
	}
	n, e := s.options.Store.CountJobs(r.Context(), in)
	if e != nil {
		handleError(w, e)
		return
	}
	list(w, items, n)
}

type createInput struct {
	Name           string            `json:"name"`
	Config         config.ScanConfig `json:"config"`
	Priority       int               `json:"priority"`
	ScheduledAt    *time.Time        `json:"scheduled_at"`
	IdempotencyKey string            `json:"idempotency_key"`
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	var in createInput
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		fail(w, 400, "provide an idempotency key", "idempotency_key")
		return
	}
	settings, e := s.settings(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	if _, e := config.Validate(r.Context(), in.Config, settings.Policy); e != nil {
		configError(w, e)
		return
	}
	j, e := s.options.Store.CreateJob(r.Context(), store.CreateJobInput{Name: in.Name, Config: in.Config, Policy: settings.Policy, Priority: in.Priority, IdempotencyKey: in.IdempotencyKey, QueueScheduledAt: in.ScheduledAt})
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 201, j)
}
func configError(w http.ResponseWriter, e error) {
	parts := strings.SplitN(e.Error(), ": ", 2)
	if len(parts) == 2 {
		fail(w, 400, parts[1], parts[0])
	} else {
		fail(w, 400, "invalid scan configuration", "config")
	}
}
func (s *Server) getJob(w http.ResponseWriter, r *http.Request) {
	j, e := s.options.Store.GetJob(r.Context(), r.PathValue("id"))
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, j)
}
func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request) {
	if !emptyMutation(w, r) {
		return
	}
	j, e := s.options.Store.CancelQueued(r.Context(), r.PathValue("id"))
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, j)
}
func emptyMutation(w http.ResponseWriter, r *http.Request) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	var in struct{}
	return decode(w, r, &in)
}
func (s *Server) retryJob(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IdempotencyKey string `json:"idempotency_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	if strings.TrimSpace(in.IdempotencyKey) == "" {
		fail(w, 400, "provide an idempotency key", "idempotency_key")
		return
	}
	j, e := s.options.Store.GetJob(r.Context(), r.PathValue("id"))
	if e != nil {
		handleError(w, e)
		return
	}
	if !store.Terminal(j.State) {
		handleError(w, store.ErrNotTerminal)
		return
	}
	settings, e := s.settings(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	if _, e = config.Validate(r.Context(), j.Config, settings.Policy); e != nil {
		configError(w, e)
		return
	}
	next, e := s.options.Store.CreateJob(r.Context(), store.CreateJobInput{Name: j.Name, Config: j.Config, Policy: settings.Policy, Priority: j.Priority, IdempotencyKey: in.IdempotencyKey, RetryOf: &j.ID})
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 201, next)
}
func (s *Server) deleteJob(w http.ResponseWriter, r *http.Request) {
	if !emptyMutation(w, r) {
		return
	}
	if e := s.options.Store.DeleteJob(r.Context(), r.PathValue("id")); e != nil {
		handleError(w, e)
		return
	}
	w.WriteHeader(204)
}
func (s *Server) results(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := page(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if id == "" {
		id = r.URL.Query().Get("job_id")
	}
	if id != "" {
		if _, e := uuid.Parse(id); e != nil {
			fail(w, 400, "use a valid job ID", "job_id")
			return
		}
		if _, e := s.options.Store.GetJob(r.Context(), id); e != nil {
			handleError(w, e)
			return
		}
	}
	state, search := r.URL.Query().Get("state"), r.URL.Query().Get("search")
	if len(state) > 100 || len(search) > 200 {
		fail(w, 400, "search or state is too long", "")
		return
	}
	items, n, e := s.options.Store.Results(r.Context(), id, state, search, limit, offset)
	if e != nil {
		handleError(w, e)
		return
	}
	for i, v := range items {
		items[i] = redact(v)
	}
	list(w, items, n)
}
func (s *Server) export(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	control := http.NewResponseController(w)
	defer control.SetWriteDeadline(time.Time{})
	started, first := false, true
	send := func(data []byte) error {
		if e := control.SetWriteDeadline(time.Now().Add(10 * time.Second)); e != nil && !errors.Is(e, http.ErrNotSupported) {
			return e
		}
		_, e := w.Write(data)
		return e
	}
	e := s.options.Store.ExportResults(ctx, r.PathValue("id"), func(j store.JobDTO) error {
		raw, e := json.Marshal(j)
		if e != nil {
			return e
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="scan-`+j.ID+`.json"`)
		started = true
		return send(append(append([]byte(`{"job":`), raw...), []byte(`,"results":[`)...))
	}, func(row json.RawMessage) error {
		if !first {
			if e := send([]byte(",")); e != nil {
				return e
			}
		}
		first = false
		return send(redact(row))
	})
	if e != nil {
		if !started {
			handleError(w, e)
		}
		return
	} // An interrupted export stays incomplete, never a valid truncated report.
	_ = send([]byte("]}\n"))
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 1 || n > 365 {
			fail(w, 400, "days must be 1-365", "days")
			return
		}
		days = n
	}
	v, e := s.options.Store.Overview(r.Context(), days)
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, v)
}
