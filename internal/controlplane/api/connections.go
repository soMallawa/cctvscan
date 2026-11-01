package api

import (
	"context"
	"encoding/json"
	"github.com/postfix/cctvscan/internal/catalogue"
	"github.com/postfix/cctvscan/internal/connection"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

func (s *Server) connectionRoutes() {
	s.mux.HandleFunc("GET /api/results/{result}/catalogue", s.resultCatalogue)
	s.mux.HandleFunc("GET /api/results/{result}/connections", s.resultConnections)
	s.mux.HandleFunc("POST /api/results/{result}/connections", s.verifyConnection)
}
func (s *Server) resultEvidence(w http.ResponseWriter, r *http.Request) (store.ResultEvidence, bool) {
	id, e := strconv.ParseInt(r.PathValue("result"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "use a valid stored result ID", "")
		return store.ResultEvidence{}, false
	}
	if s.options.Store == nil {
		fail(w, 503, "stored results unavailable", "")
		return store.ResultEvidence{}, false
	}
	v, e := s.options.Store.ResultEvidence(r.Context(), id)
	if e != nil {
		handleError(w, e)
		return v, false
	}
	return v, true
}
func (s *Server) resultCatalogue(w http.ResponseWriter, r *http.Request) {
	v, ok := s.resultEvidence(w, r)
	if !ok {
		return
	}
	c, e := catalogue.Default()
	if e != nil {
		fail(w, 503, "local catalogue unavailable", "")
		return
	}
	a := c.AnalyzeRecord(v.Data)
	var record struct{ Ports []int }
	json.Unmarshal(v.Data, &record)
	settings, e := s.settings(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, map[string]any{"assessment": a, "profiles": c.ProfilesFor(a.Brand), "scope": map[string]any{"host": v.Target, "ports": record.Ports, "has_http_proxy": v.Config.ProxyVersionID != "", "credentials_allowed": settings.Policy.AllowCredentials, "capture_allowed": settings.Policy.AllowCapture}, "capabilities": map[string]any{"http_auth": []string{"Basic", "Digest MD5", "Digest SHA-256"}, "rtsp": "Explicit direct TCP with described video and one-frame decode", "vendor_forms": "unsupported", "continuous_playback": false}})
}
func (s *Server) resultConnections(w http.ResponseWriter, r *http.Request) {
	v, ok := s.resultEvidence(w, r)
	if !ok {
		return
	}
	rows, e := s.options.Store.ConnectionChecks(r.Context(), v.ID)
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, map[string]any{"items": rows})
}
func (s *Server) verifyConnection(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ProfileID  string            `json:"profile_id"`
		Scheme     string            `json:"scheme"`
		Port       int               `json:"port"`
		Path       string            `json:"path"`
		Username   string            `json:"username"`
		Password   string            `json:"password"`
		Parameters map[string]string `json:"parameters"`
		DirectRTSP bool              `json:"direct_rtsp"`
	}
	if !decode(w, r, &body) {
		return
	}
	v, ok := s.resultEvidence(w, r)
	if !ok {
		return
	}
	var record struct{ Ports []int }
	if json.Unmarshal(v.Data, &record) != nil {
		fail(w, 400, "retained target scope unavailable", "")
		return
	}
	allowed := false
	for _, p := range record.Ports {
		allowed = allowed || p == body.Port
	}
	if !allowed {
		fail(w, 400, "select a port retained in this scan result", "port")
		return
	}
	if len(body.Parameters) > 16 || len(body.Username) > 200 || len(body.Password) > 1024 {
		fail(w, 400, "credential or parameter input exceeds the connection budget", "")
		return
	}
	for k, val := range body.Parameters {
		if len(k) > 64 || len(val) > 1024 {
			fail(w, 400, "connection parameter exceeds the input budget", "parameters")
			return
		}
	}
	settings, e := s.settings(r.Context())
	if e != nil {
		handleError(w, e)
		return
	}
	authInput := body.Username != "" || body.Password != ""
	for k := range body.Parameters {
		lower := strings.ToLower(k)
		authInput = authInput || strings.Contains(lower, "token") || strings.Contains(lower, "pass") || lower == "user"
	}
	if authInput && !settings.Policy.AllowCredentials {
		fail(w, 400, "credential use is disabled by the server policy", "")
		return
	}
	path := body.Path
	if body.ProfileID != "" {
		c, e := catalogue.Default()
		if e != nil {
			fail(w, 503, "local catalogue unavailable", "")
			return
		}
		p, exists := c.Profile(body.ProfileID)
		if !exists || p.Scheme != body.Scheme {
			fail(w, 400, "select a documented profile with a matching protocol", "profile_id")
			return
		}
		path = p.Path
	}
	var proxy *url.URL
	if v.Config.ProxyVersionID != "" {
		if body.Scheme == "rtsp" {
			fail(w, 400, "RTSP cannot bypass this job's selected HTTP proxy", "scheme")
			return
		}
		if s.options.Secrets == nil {
			fail(w, 503, "saved proxy decryption unavailable", "")
			return
		}
		sealed, e := s.options.Store.ConnectionProxy(r.Context(), v.Config.ProxyVersionID)
		if e != nil {
			handleError(w, e)
			return
		}
		clear, e := s.options.Secrets.Open("proxy/"+v.Config.ProxyVersionID, sealed)
		if e != nil {
			fail(w, 503, "saved proxy unavailable", "")
			return
		}
		proxy, _, e = parseProxy(string(clear))
		for i := range clear {
			clear[i] = 0
		}
		if e != nil {
			fail(w, 503, "saved proxy configuration invalid", "")
			return
		}
	}
	select {
	case s.verifyGate <- struct{}{}:
		defer func() { <-s.verifyGate }()
	default:
		fail(w, 409, "another connection verification is running; wait for its result", "")
		return
	}
	out := connection.Verify(r.Context(), connection.Input{Host: v.Target, Port: body.Port, Scheme: body.Scheme, Path: path, Username: body.Username, Password: body.Password, Parameters: body.Parameters, Proxy: proxy, AllowDirectRTSP: body.DirectRTSP})
	// Keep policy-controlled previews private; credentials and resolved secret paths are never stored.
	if !settings.Policy.AllowCapture {
		out.FrameBase64 = ""
	}
	cleanup, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	saved, e := s.options.Store.SaveConnectionCheck(cleanup, v.ID, body.ProfileID, out)
	if e != nil {
		fail(w, 503, "connection check finished but its result could not be saved; do not assume a saved result", "")
		return
	}
	write(w, 201, saved)
}
