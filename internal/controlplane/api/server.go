// Package api exposes the private single-operator control plane using net/http.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/auth"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/secrets"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Options struct {
	Store                                            *store.Store
	Secrets                                          *secrets.Box
	Auth                                             auth.Config
	PublicOrigin, Version, ArtifactDir, ProxyTestURL string
}
type Server struct {
 verifyGate chan struct{}
	options  Options
	boundary *auth.Boundary
	mux      *http.ServeMux
}

func New(o Options) (*Server, error) {
	b, e := auth.New(o.Auth)
	if e != nil {
		return nil, e
	}
	if o.Auth.Mode == "" {
		o.Auth.Mode = "access"
	}
	u, e := url.Parse(o.PublicOrigin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, errors.New("public origin must be an explicit HTTP(S) origin")
	}
	if o.Auth.Mode != "local" && u.Scheme != "https" {
		return nil, errors.New("access public origin requires HTTPS")
	}
	o.PublicOrigin = strings.TrimSuffix(o.PublicOrigin, "/")
	if o.ProxyTestURL != "" {
		p, e := url.Parse(o.ProxyTestURL)
		if e != nil || p.User != nil || p.Host == "" || (p.Scheme != "http" && p.Scheme != "https") {
			return nil, errors.New("invalid server-owned proxy test URL")
		}
	}
	s := &Server{options: o, boundary: b, mux: http.NewServeMux(), verifyGate:make(chan struct{},1)}
	s.routes()
	return s, nil
}
func (s *Server) Handler() http.Handler {
	return s.boundary.Protect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if r.Method != "GET" && r.Method != "HEAD" && r.Method != "OPTIONS" && r.Header.Get("Origin") != s.options.PublicOrigin {
			fail(w, 403, "Origin must equal the configured public origin", "origin")
			return
		}
		if id := r.PathValue("id"); id != "" {
			if _, e := uuid.Parse(id); e != nil {
				fail(w, 400, "use a valid resource ID", "id")
				return
			}
		}
		s.mux.ServeHTTP(w, r)
	}))
}
func (s *Server) routes() {
 s.connectionRoutes()
 s.catalogueRoutes()
	s.mux.HandleFunc("GET /api/artifacts/{id}", s.serveArtifact)
	s.mux.HandleFunc("GET /api/jobs/{id}/artifacts", s.listArtifacts)
	s.jobRoutes()
	s.eventRoutes()
	s.resourceRoutes()
	s.mux.HandleFunc("GET /api/health", s.health)
	s.mux.HandleFunc("GET /api/bootstrap", s.bootstrap)
	s.mux.HandleFunc("POST /api/jobs/preview", s.preview)
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, message, field string) {
	v := map[string]string{"error": message}
	if field != "" {
		v["field"] = field
	}
	write(w, status, v)
}
func handleError(w http.ResponseWriter, e error) {
	var field *store.FieldError
	var conflict *store.ConflictError
	switch {
	case errors.As(e, &field):
		fail(w, 400, field.Message, field.Field)
	case errors.Is(e, store.ErrNotFound):
		fail(w, 404, "resource not found", "")
	case errors.As(e, &conflict):
		fail(w, 409, conflict.Error(), "idempotency_key")
	case errors.Is(e, store.ErrReferenced):
		fail(w, 409, "resource is referenced by live jobs or saved configurations; remove those references first", "")
	case errors.Is(e, store.ErrNotTerminal):
		fail(w, 409, "only terminal jobs can be retried or deleted", "")
	case errors.Is(e, store.ErrInvalidInput):
		fail(w, 400, "invalid request; check names, pagination and priority", "")
	default:
		fail(w, 503, "database operation unavailable; retry later", "")
	}
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	media, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if e != nil || media != "application/json" {
		fail(w, 400, "Content-Type must be application/json", "")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	e = d.Decode(v)
	if e == nil {
		var extra any
		if d.Decode(&extra) != io.EOF {
			e = errors.New("trailing JSON")
		}
	}
	if e != nil {
		var big *http.MaxBytesError
		if errors.As(e, &big) {
			fail(w, 413, "JSON body exceeds 1 MiB", "")
		} else {
			fail(w, 400, "provide one valid JSON object with only documented fields", "")
		}
		return false
	}
	return true
}
func (s *Server) healthy(ctx context.Context) bool {
	if s.options.Store == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return s.options.Store.Ping(ctx) == nil
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	ok := s.healthy(r.Context())
	status := 503
	if ok {
		status = 200
	}
	db := "unavailable"
	if ok {
		db = "ready"
	}
	write(w, status, map[string]any{"ok": ok, "database": db, "queue": db, "version": s.options.Version, "auth_mode": s.options.Auth.Mode})
}
func (s *Server) settings(ctx context.Context) (store.Settings, error) {
	if s.options.Store == nil {
		return store.Settings{}, errors.New("database unavailable")
	}
	return s.options.Store.GetSettings(ctx)
}
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	settings, e := s.settings(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	cursor, e := s.options.Store.EventCursor(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, map[string]any{"event_cursor": cursor, "defaults": config.DefaultConfig(), "policy": settings.Policy, "auth_mode": s.options.Auth.Mode, "version": s.options.Version, "proxy_test_configured": s.options.ProxyTestURL != ""})
}
func (s *Server) validate(ctx context.Context, c config.ScanConfig) (config.Preview, error) {
	settings, e := s.settings(ctx)
	if e != nil {
		return config.Preview{}, e
	}
	p, e := config.Validate(ctx, c, settings.Policy)
	if e != nil {
		parts := strings.SplitN(e.Error(), ": ", 2)
		if len(parts) == 2 {
			return p, &store.FieldError{Field: parts[0], Message: parts[1]}
		}
		return p, store.ErrInvalidInput
	}
	if e = s.options.Store.ValidateReferences(ctx, p.Config); e != nil {
		return p, e
	}
	return p, nil
}
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Config config.ScanConfig `json:"config"`
	}
	if !decode(w, r, &in) {
		return
	}
	p, e := s.validate(r.Context(), in.Config)
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, p)
}
