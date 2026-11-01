//go:build integration

package api

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"os"
	"strings"
	"testing"
)

func TestExportAllStoredResults(t *testing.T) {
	s := apiFixture(t)
	ctx := context.Background()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	j, e := s.options.Store.CreateJob(ctx, store.CreateJobInput{Name: "export-fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	conn, e := pgx.Connect(ctx, os.Getenv("CCTVSCAN_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal("fixture connection unavailable")
	}
	defer conn.Close(ctx)
	attempt := uuid.NewString()
	if _, e = conn.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,'completed')`, attempt, j.ID, uuid.NewString()); e != nil {
		t.Fatal(e)
	}
	if _, e = conn.Exec(ctx, `INSERT INTO results(job_id,attempt_id,target,brand,state,data) SELECT $1,$2,'fixture-'||n,'Fixture','inconclusive','{"CredentialState":"inconclusive","password":"never-export-marker"}'::jsonb FROM generate_series(1,10001) n`, j.ID, attempt); e != nil {
		t.Fatal(e)
	}
	w := request(t, s, "GET", "/api/jobs/"+j.ID+"/export", nil)
	if w.Code != 200 {
		t.Fatalf("full export rejected: %d", w.Code)
	}
	var report struct {
		Job     store.JobDTO      `json:"job"`
		Results []json.RawMessage `json:"results"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &report); e != nil {
		t.Fatal("invalid export JSON", e)
	}
	if len(report.Results) != 10001 || report.Job.ID != j.ID {
		t.Fatalf("truncated export: %d", len(report.Results))
	}
	if strings.Contains(w.Body.String(), "never-export-marker") {
		t.Fatal("export leaked secret")
	}
	if !strings.Contains(w.Body.String(), `"CredentialState":"inconclusive"`) {
		t.Fatal("lost public evidence state")
	}
	if !strings.Contains(w.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("not a download")
	}
}
