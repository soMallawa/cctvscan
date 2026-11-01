//go:build integration

package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

func TestProtectedArtifactContentAndIntegrity(t *testing.T) {
	s := apiFixture(t)
	ctx := context.Background()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	j, e := s.options.Store.CreateJob(ctx, store.CreateJobInput{Name: "owned artifact fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	root := t.TempDir()
	s.options.ArtifactDir = root
	attempt, id := uuid.NewString(), uuid.NewString()
	relative := filepath.Join(j.ID, attempt, "snapshots", "fixture.jpg")
	full := filepath.Join(root, relative)
	if e = os.MkdirAll(filepath.Dir(full), 0700); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	if e = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); e != nil {
		t.Fatal(e)
	}
	data := b.Bytes()
	digest := fmt.Sprintf("%x", sha256.Sum256(data))
	if e = os.WriteFile(full, data, 0600); e != nil {
		t.Fatal(e)
	}
	conn, e := pgx.Connect(ctx, os.Getenv("CCTVSCAN_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal("fixture connection unavailable")
	}
	defer conn.Close(ctx)
	if _, e = conn.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,'completed')`, attempt, j.ID, uuid.NewString()); e != nil {
		t.Fatal(e)
	}
	if _, e = conn.Exec(ctx, `INSERT INTO artifacts(id,job_id,attempt_id,relative_path,media_type,size_bytes,sha256) VALUES($1,$2,$3,$4,'image/jpeg',$5,$6)`, id, j.ID, attempt, relative, len(data), digest); e != nil {
		t.Fatal(e)
	}
	got := request(t, s, "GET", "/api/artifacts/"+id, nil)
	if got.Code != 200 || !bytes.Equal(got.Body.Bytes(), data) {
		t.Fatal("protected image unavailable", got.Code)
	}
	if got.Header().Get("Content-Type") != "image/jpeg" || got.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("image response not hardened")
	}
	if e = os.WriteFile(full, []byte("modified bytes"), 0600); e != nil {
		t.Fatal(e)
	}
	if r := request(t, s, "GET", "/api/artifacts/"+id, nil); r.Code == 200 {
		t.Fatal("modified evidence served as original")
	}
	os.Remove(full)
	if e = os.Symlink("/etc/passwd", full); e != nil {
		t.Fatal(e)
	}
	if r := request(t, s, "GET", "/api/artifacts/"+id, nil); r.Code == 200 {
		t.Fatal("artifact symlink escaped root")
	}
}
