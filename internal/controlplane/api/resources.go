package api

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	credentialfile "github.com/postfix/cctvscan/internal/credentials"
	"github.com/postfix/cctvscan/internal/httptransport"
	"github.com/robfig/cron/v3"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

func (s *Server) resourceRoutes() {
	for _, kind := range []string{"proxies", "credential-sets", "profiles", "schedules", "runners"} {
		s.mux.HandleFunc("GET /api/"+kind, func(w http.ResponseWriter, r *http.Request) {
			limit, offset, ok := page(w, r)
			if !ok {
				return
			}
			items, total, e := s.options.Store.ResourcePage(r.Context(), kind, r.URL.Query().Get("search"), limit, offset)
			if e != nil {
				handleError(w, e)
				return
			}
			list(w, items, total)
		})
		s.mux.HandleFunc("GET /api/"+kind+"/{id}", validatedID(func(w http.ResponseWriter, r *http.Request) {
			item, e := s.options.Store.Resource(r.Context(), kind, r.PathValue("id"))
			if e != nil {
				handleError(w, e)
				return
			}
			write(w, 200, item)
		}))
		if kind == "runners" {
			continue
		}
		s.mux.HandleFunc("POST /api/"+kind, s.saveResource(kind, false))
		s.mux.HandleFunc("DELETE /api/"+kind+"/{id}", validatedID(func(w http.ResponseWriter, r *http.Request) {
			if !emptyMutation(w, r) {
				return
			}
			if e := s.options.Store.DeleteResource(r.Context(), kind, r.PathValue("id")); e != nil {
				handleError(w, e)
				return
			}
			w.WriteHeader(204)
		}))
		if kind != "credential-sets" {
			s.mux.HandleFunc("PUT /api/"+kind+"/{id}", validatedID(s.saveResource(kind, true)))
		}
	}
	s.mux.HandleFunc("POST /api/proxies/{id}/test", validatedID(s.testProxy))
	s.mux.HandleFunc("GET /api/settings", s.getSettings)
	s.mux.HandleFunc("PUT /api/settings", s.putSettings)
}
func (s *Server) saveResource(kind string, update bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		in := store.ResourceInput{ID: uuid.NewString(), VersionID: uuid.NewString()}
		if update {
			in.ID = r.PathValue("id")
		}
		switch kind {
		case "proxies":
			var body struct {
				Name string `json:"name"`
				URL  string `json:"url"`
			}
			if !decode(w, r, &body) {
				return
			}
			u, port, e := parseProxy(body.URL)
			if e != nil {
				fail(w, 400, "provide an HTTP(S) proxy URL with valid host/port and no path/query", "url")
				return
			}
			if s.options.Secrets == nil {
				fail(w, 503, "secret encryption is not configured", "")
				return
			}
			in.Name = body.Name
			in.Sealed, e = s.options.Secrets.Seal("proxy/"+in.VersionID, []byte(body.URL))
			if e != nil {
				fail(w, 503, "secret encryption unavailable", "")
				return
			}
			in.Metadata, _ = json.Marshal(map[string]any{"scheme": u.Scheme, "host": u.Hostname(), "port": port, "authentication_present": u.User != nil})
		case "credential-sets":
			var body struct {
				Name    string `json:"name"`
				Content string `json:"content"`
			}
			if !decode(w, r, &body) {
				return
			}
			parsed, err := credentialfile.Parse([]byte(body.Content))
			count := len(parsed.Entries)
			if err != nil || count == 0 {
				fail(w, 400, "provide 1-10000 username:password pairs; vendor comment headers are supported", "content")
				return
			}

			if s.options.Secrets == nil {
				fail(w, 503, "secret encryption is not configured", "")
				return
			}
			in.Name = body.Name
			in.EntryCount = count
			var e error
			in.Sealed, e = s.options.Secrets.Seal("credentials/"+in.ID, []byte(body.Content))
			if e != nil {
				fail(w, 503, "secret encryption unavailable", "")
				return
			}
		case "profiles":
			var body struct {
				Name   string            `json:"name"`
				Config config.ScanConfig `json:"config"`
			}
			if !decode(w, r, &body) {
				return
			}
			p, e := s.validate(r.Context(), body.Config)
			if e != nil {
				handleError(w, e)
				return
			}
			in.Name = body.Name
			in.Config = p.Config
		case "schedules":
			var body struct {
				Name          string            `json:"name"`
				Config        config.ScanConfig `json:"config"`
				Cron          string            `json:"cron"`
				Timezone      string            `json:"timezone"`
				Enabled       bool              `json:"enabled"`
				MisfirePolicy string            `json:"misfire_policy"`
				OverlapPolicy string            `json:"overlap_policy"`
			}
			if !decode(w, r, &body) {
				return
			}
			p, e := s.validate(r.Context(), body.Config)
			if e != nil {
				handleError(w, e)
				return
			}
			next, e := scheduleNext(body.Cron, body.Timezone, time.Now())
			if e != nil {
				fail(w, 400, "provide a valid five-field cron expression and IANA timezone", "cron")
				return
			}
			in.Name = body.Name
			in.Config = p.Config
			if body.MisfirePolicy == "" {
				body.MisfirePolicy = "skip"
			}
			if body.OverlapPolicy == "" {
				body.OverlapPolicy = "skip"
			}
			if body.MisfirePolicy != "skip" && body.MisfirePolicy != "catch_up_once" {
				fail(w, 400, "choose skip or catch_up_once", "misfire_policy")
				return
			}
			if body.OverlapPolicy != "skip" && body.OverlapPolicy != "queue" {
				fail(w, 400, "choose skip or queue", "overlap_policy")
				return
			}
			in.MisfirePolicy = body.MisfirePolicy
			in.OverlapPolicy = body.OverlapPolicy
			in.Cron = body.Cron
			in.Timezone = body.Timezone
			in.Enabled = body.Enabled
			if in.Enabled {
				in.NextRunAt = &next
			}
		}
		obj, e := s.options.Store.SaveResource(r.Context(), kind, in, update)
		if e != nil {
			handleError(w, e)
			return
		}
		status := 201
		if update {
			status = 200
		}
		write(w, status, obj)
	}
}
func parseProxy(raw string) (*url.URL, int, error) {
	if len(raw) > 8192 {
		return nil, 0, errors.New("invalid proxy")
	}
	u, e := url.Parse(raw)
	if e != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.HasSuffix(u.Host, ":") {
		return nil, 0, errors.New("invalid proxy")
	}
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if u.Port() != "" {
		port, e = strconv.Atoi(u.Port())
		if e != nil || port < 1 || port > 65535 {
			return nil, 0, errors.New("invalid port")
		}
	}
	return u, port, nil
}
func scheduleNext(expr, zone string, now time.Time) (time.Time, error) {
	if len(expr) > 200 || strings.Contains(expr, "TZ=") {
		return time.Time{}, errors.New("invalid cron")
	}
	location, e := time.LoadLocation(zone)
	if e != nil || zone == "" || zone == "Local" {
		return time.Time{}, errors.New("invalid timezone")
	}
	schedule, e := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow).Parse(expr)
	if e != nil {
		return time.Time{}, e
	}
	next := schedule.Next(now.In(location))
	if next.IsZero() {
		return next, errors.New("cron has no future occurrence")
	}
	return next.UTC(), nil
}

// The profile client is deliberately independent of http.DefaultTransport and
// scanner global proxy policy. Both proxy and destination TLS are strict.
func profileClient(proxy *url.URL) *http.Client {
	return httptransport.NewProxyDiagnosticClient(proxy)
}
func (s *Server) testProxy(w http.ResponseWriter, r *http.Request) {
	if !emptyMutation(w, r) {
		return
	}
	if s.options.ProxyTestURL == "" {
		fail(w, 400, "configure the server-owned proxy diagnostic destination first", "proxy_test_url")
		return
	}
	id, sealed, e := s.options.Store.ProxySecret(r.Context(), r.PathValue("id"))
	if e != nil {
		handleError(w, e)
		return
	}
	if s.options.Secrets == nil {
		fail(w, 503, "secret encryption unavailable", "")
		return
	}
	plain, e := s.options.Secrets.Open("proxy/"+id, sealed)
	if e != nil {
		fail(w, 503, "proxy secret unavailable; verify the server key", "")
		return
	}
	proxy, _, e := parseProxy(string(plain))
	clear(plain)
	if e != nil {
		fail(w, 503, "proxy secret is invalid", "")
		return
	}
	client := profileClient(proxy)
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(ctx, "GET", s.options.ProxyTestURL, nil)
	if e != nil {
		fail(w, 503, "proxy diagnostic destination is invalid", "")
		return
	}
	start := time.Now()
	res, e := client.Do(req)
	ok := false
	if e == nil {
		ok = res.StatusCode >= 200 && res.StatusCode < 400
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		res.Body.Close()
	}
	if e = s.options.Store.RecordProxyTest(r.Context(), r.PathValue("id"), ok); e != nil {
		handleError(w, e)
		return
	}
	message := "proxy diagnostic failed; verify proxy TLS, authentication and destination"
	if ok {
		message = "proxy diagnostic succeeded"
	}
	write(w, 200, map[string]any{"ok": ok, "message": message, "elapsed_ms": time.Since(start).Milliseconds()})
}
func (s *Server) settingsDTO(in store.Settings) map[string]any {
	return map[string]any{"policy": in.Policy, "retention_days": in.RetentionDays, "max_concurrent_jobs": in.MaxConcurrentJobs, "auth_mode": s.options.Auth.Mode, "public_origin": s.options.PublicOrigin, "version": s.options.Version}
}
func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	in, e := s.settings(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, s.settingsDTO(in))
}
func validateSettings(in store.Settings) error {
	if in.RetentionDays < 1 || in.RetentionDays > 3650 {
		return &store.FieldError{Field: "retention_days", Message: "must be 1-3650"}
	}
	if in.MaxConcurrentJobs != 1 {
		return &store.FieldError{Field: "max_concurrent_jobs", Message: "this worker version supports exactly one slot"}
	}
	caps := reflect.ValueOf(config.DefaultPolicy())
	p := reflect.ValueOf(in.Policy)
	typ := p.Type()
	for i := 0; i < p.NumField(); i++ {
		v, c := p.Field(i), caps.Field(i)
		valid := true
		switch v.Kind() {
		case reflect.Bool:
			valid = !v.Bool() || c.Bool()
		case reflect.Int:
			valid = v.Int() >= 0 && v.Int() <= c.Int()
		case reflect.Uint64:
			valid = v.Uint() <= c.Uint()
		case reflect.Float64:
			valid = v.Float() >= 0 && v.Float() <= c.Float()
		}
		if !valid {
			return &store.FieldError{Field: "policy", Message: typ.Field(i).Tag.Get("json") + " exceeds the server capability ceiling"}
		}
	}
	return nil
}
func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) {
	var in store.Settings
	if !decode(w, r, &in) {
		return
	}
	if e := validateSettings(in); e != nil {
		handleError(w, e)
		return
	}
	if e := s.options.Store.PutSettings(r.Context(), in); e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, s.settingsDTO(in))
}
