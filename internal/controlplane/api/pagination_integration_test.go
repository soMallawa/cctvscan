//go:build integration

package api

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"testing"
)

func TestResourcePageAndDirectRead(t *testing.T) {
	s := apiFixture(t)
	name := "page-" + uuid.NewString()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	ids := []string{}
	for i := 0; i < 3; i++ {
		w := request(t, s, "POST", "/api/profiles", map[string]any{"name": name, "config": c})
		if w.Code != 201 {
			t.Fatalf("create: %d", w.Code)
		}
		var row struct {
			ID string `json:"id"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &row); e != nil {
			t.Fatal(e)
		}
		ids = append(ids, row.ID)
	}
	defer func() {
		for _, id := range ids {
			request(t, s, "DELETE", "/api/profiles/"+id, nil)
		}
	}()
	var first struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	w := request(t, s, "GET", "/api/profiles?limit=2&search="+name, nil)
	if w.Code != 200 {
		t.Fatalf("page: %d", w.Code)
	}
	if e := json.Unmarshal(w.Body.Bytes(), &first); e != nil {
		t.Fatal(e)
	}
	if len(first.Items) != 2 || first.Total != 3 {
		t.Fatalf("unbounded/incorrect page: items=%d total=%d", len(first.Items), first.Total)
	}
	var second struct {
		Items []map[string]any `json:"items"`
		Total int64            `json:"total"`
	}
	w = request(t, s, "GET", "/api/profiles?limit=2&offset=2&search="+name, nil)
	if e := json.Unmarshal(w.Body.Bytes(), &second); e != nil {
		t.Fatal(e)
	}
	if len(second.Items) != 1 || second.Total != 3 {
		t.Fatalf("second page: items=%d total=%d", len(second.Items), second.Total)
	}
	for _, row := range first.Items {
		if row["id"] == second.Items[0]["id"] {
			t.Fatal("page overlaps")
		}
	}
	w = request(t, s, "GET", "/api/profiles/"+ids[0], nil)
	if w.Code != 200 {
		t.Fatalf("direct read: %d", w.Code)
	}
	if w = request(t, s, "GET", "/api/profiles?limit=201", nil); w.Code != 400 {
		t.Fatalf("invalid limit: %d", w.Code)
	}
	if w = request(t, s, "GET", "/api/profiles/"+uuid.NewString(), nil); w.Code != 404 {
		t.Fatalf("missing metadata: %d", w.Code)
	}
}
