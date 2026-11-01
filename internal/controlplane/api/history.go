package api

import (
	"net/http"
	"strconv"
)

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	limit, _, ok := page(w, r)
	if !ok {
		return
	}
	before := int64(0)
	if text := r.URL.Query().Get("before"); text != "" {
		var e error
		before, e = strconv.ParseInt(text, 10, 64)
		if e != nil || before < 0 {
			fail(w, 400, "before must be a nonnegative decimal sequence", "before")
			return
		}
	}
	id := r.PathValue("id")
	if _, e := s.options.Store.GetJob(r.Context(), id); e != nil {
		handleError(w, e)
		return
	}
	items, more, e := s.options.Store.EventHistory(r.Context(), id, before, limit)
	if e != nil {
		handleError(w, e)
		return
	}
	for i := range items {
		items[i].Data = redact(items[i].Data)
	}
	write(w, 200, map[string]any{"items": items, "has_more": more})
}
