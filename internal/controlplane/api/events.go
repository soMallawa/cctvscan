package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) eventRoutes() {
	s.mux.HandleFunc("GET /api/jobs/{id}/history", validatedID(s.history))
	s.mux.HandleFunc("GET /api/events", s.events)
	s.mux.HandleFunc("GET /api/jobs/{id}/events", validatedID(s.events))
}
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	cursor := r.URL.Query().Get("after")
	if cursor == "" {
		cursor = r.Header.Get("Last-Event-ID")
	}
	after := int64(0)
	if cursor != "" {
		n, e := strconv.ParseInt(cursor, 10, 64)
		if e != nil || n < 0 {
			fail(w, 400, "event cursor must be a nonnegative decimal sequence", "after")
			return
		}
		after = n
	}
	id := r.PathValue("id")
	if id != "" {
		if _, e := s.options.Store.GetJob(r.Context(), id); e != nil {
			handleError(w, e)
			return
		}
	}
	// Polling is per connection with no background producer or unbounded channel.
	// Each write gets a deadline so a stalled client releases its handler and pool.
	if _, ok := w.(http.Flusher); !ok {
		fail(w, 503, "streaming unavailable", "")
		return
	}
	initial, e := s.options.Store.GlobalEvents(r.Context(), id, after, 200)
	if e != nil {
		handleError(w, e)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	control := http.NewResponseController(w)
	defer control.SetWriteDeadline(time.Time{})
	send := func(text string) bool {
		if e := control.SetWriteDeadline(time.Now().Add(5 * time.Second)); e != nil && e != http.ErrNotSupported {
			return false
		}
		if _, e := fmt.Fprint(w, text); e != nil {
			return false
		}
		return control.Flush() == nil
	}
	if !send(": connected\n\n") {
		return
	}
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	batch := initial
	for {
		for _, event := range batch {
			var data any
			_ = json.Unmarshal(redact(event.Data), &data)
			phase, message := "", ""
			if obj, ok := data.(map[string]any); ok {
				phase, _ = obj["phase"].(string)
				message, _ = obj["message"].(string)
			}
			v := map[string]any{"seq": strconv.FormatInt(event.Seq, 10), "job_id": event.JobID, "kind": event.Kind, "at": event.CreatedAt, "phase": phase, "message": message, "data": data}
			raw, e := json.Marshal(v)
			if e != nil {
				return
			}
			if !send(fmt.Sprintf("id: %d\nevent: %s\ndata: %s\n\n", event.Seq, safeEventKind(event.Kind), raw)) {
				return
			}
			after = event.Seq
		}
		if len(batch) == 200 {
			batch, e = s.options.Store.GlobalEvents(r.Context(), id, after, 200)
			if e != nil {
				return
			}
			continue
		}
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			if !send(": heartbeat\n\n") {
				return
			}
			batch = nil
		case <-poll.C:
			batch, e = s.options.Store.GlobalEvents(r.Context(), id, after, 200)
			if e != nil {
				return
			}
		}
	}
}
func safeEventKind(kind string) string {
	switch kind {
	case "state", "phase", "metrics", "log", "artifact", "runner":
		return kind
	}
	return "log"
}
