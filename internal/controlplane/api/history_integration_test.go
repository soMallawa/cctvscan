//go:build integration

package api

import (
	"context"
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"strconv"
	"testing"
)

func TestBoundedHistoryAndBootstrapCursor(t *testing.T) {
	s := apiFixture(t)
	ctx := context.Background()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	j, e := s.options.Store.CreateJob(ctx, store.CreateJobInput{Name: "history fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	var latest int64
	for i := 0; i < 3; i++ {
		evt, e := s.options.Store.AddEvent(ctx, j.ID, "phase", json.RawMessage(`{"phase":"fixture","password":"history-secret"}`))
		if e != nil {
			t.Fatal(e)
		}
		latest = evt.Seq
	}
	w := request(t, s, "GET", "/api/jobs/"+j.ID+"/history?limit=2", nil)
	if w.Code != 200 {
		t.Fatalf("history status: %d", w.Code)
	}
	var page struct {
		Items   []store.Event `json:"items"`
		HasMore bool          `json:"has_more"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &page); e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 2 || !page.HasMore || page.Items[0].Seq != latest {
		t.Fatal("history is not a bounded latest window")
	}
	var data map[string]any
	json.Unmarshal(page.Items[0].Data, &data)
	if _, ok := data["password"]; ok {
		t.Fatal("history secret leaked")
	}
	w = request(t, s, "GET", "/api/jobs/"+j.ID+"/history?limit=2&before="+strconv.FormatInt(page.Items[1].Seq, 10), nil)
	if e = json.Unmarshal(w.Body.Bytes(), &page); e != nil {
		t.Fatal(e)
	}
	if len(page.Items) != 2 || page.HasMore {
		t.Fatal("earlier history not accessible")
	}
	w = request(t, s, "GET", "/api/bootstrap", nil)
	var b struct {
		Cursor string `json:"event_cursor"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &b); e != nil {
		t.Fatal(e)
	}
	n, e := strconv.ParseInt(b.Cursor, 10, 64)
	if e != nil || n < latest {
		t.Fatal("bootstrap lacks committed string cursor")
	}
}
