package api

import (
	"github.com/postfix/cctvscan/internal/catalogue"
	"net/http"
)

func (s *Server) catalogueRoutes() {
	s.mux.HandleFunc("GET /api/catalogue/meta", func(w http.ResponseWriter, r *http.Request) {
		c, e := catalogue.Default()
		if e != nil {
			fail(w, 503, "local camera catalogue unavailable", "")
			return
		}
		write(w, 200, map[string]any{"manifest": c.Manifest, "brands": c.Brands, "ispy": map[string]string{"status": "reference_only", "url": "https://www.ispyconnect.com/cameras", "reason": "Database redistribution requires permission; no iSpy records imported."}})
	})
	s.mux.HandleFunc("GET /api/catalogue/models", func(w http.ResponseWriter, r *http.Request) {
		limit, offset, ok := page(w, r)
		if !ok {
			return
		}
		if len(r.URL.Query().Get("search")) > 200 {
			fail(w, 400, "search must be at most 200 characters", "search")
			return
		}
		c, e := catalogue.Default()
		if e != nil {
			fail(w, 503, "local camera catalogue unavailable", "")
			return
		}
		write(w, 200, c.Search(r.URL.Query().Get("search"), r.URL.Query().Get("brand"), r.URL.Query().Get("protocol"), limit, offset))
	})
	s.mux.HandleFunc("GET /api/catalogue/models/{model}", func(w http.ResponseWriter, r *http.Request) {
		c, e := catalogue.Default()
		if e != nil {
			fail(w, 503, "local camera catalogue unavailable", "")
			return
		}
		raw, ok := c.Detail(r.PathValue("model"))
		if !ok {
			fail(w, 404, "camera model not found", "")
			return
		}
		write(w, 200, raw)
	})
	s.mux.HandleFunc("GET /api/catalogue/profiles", func(w http.ResponseWriter, r *http.Request) {
		c, e := catalogue.Default()
		if e != nil {
			fail(w, 503, "local camera catalogue unavailable", "")
			return
		}
		write(w, 200, map[string]any{"items": c.ProfilesFor(r.URL.Query().Get("brand"))})
	})
}
