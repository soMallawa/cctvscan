//go:build integration

package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"github.com/riverqueue/river"
	"image"
	"image/jpeg"
	"testing"
)

func TestWorkerRegistersValidatedCapture(t *testing.T) {
	st := workerFixture(t)
	ctx := context.Background()
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	c.Capture = true
	j, e := st.CreateJob(ctx, store.CreateJobInput{Name: "owned synthetic image fixture", Config: c, Policy: config.DefaultPolicy(), Priority: 2})
	if e != nil {
		t.Fatal(e)
	}
	rid := uuid.NewString()
	if e = st.RegisterRunner(ctx, rid); e != nil {
		t.Fatal(e)
	}
	var imageBytes bytes.Buffer
	if e = jpeg.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); e != nil {
		t.Fatal(e)
	}
	binary := script(t, fmt.Sprintf(`mkdir -p snapshots
printf '%%s' '%s' | base64 -d > snapshots/fixture.jpg
printf '%%s' '{"SchemaVersion":1,"State":"complete","Results":[{"Host":"127.0.0.1","State":"complete","StreamResults":[{"State":"media_header","Verified":true,"SavedPath":"'"$PWD"'/snapshots/fixture.jpg"}]}]}' > scan-fixture.json
`, base64.StdEncoding.EncodeToString(imageBytes.Bytes())))
	w := &Worker{Store: st, Binary: binary, Root: t.TempDir(), RunnerID: rid}
	if e = w.Work(ctx, &river.Job[store.ScanJobArgs]{Args: store.ScanJobArgs{JobID: j.ID}}); e != nil {
		t.Fatal(e)
	}

	// The worker fixture uses its own database; identify it without disclosing a DSN.
	// Artifact state is read via the public store API added for protected content.
	results, n, e := st.Results(ctx, j.ID, "", "", 10, 0)
	if e != nil || n != 1 || len(results) != 1 {
		t.Fatal("fixture result missing", e)
	}
	// A captured file must have a durable manifest, not only a file on disk.

	var rowID string
	// Query the worker database through its job-specific artifact page.
	page, total, e := st.ArtifactPage(ctx, j.ID, 10, 0)
	if e != nil || total != 1 || len(page) != 1 {
		t.Fatal("captured image has no manifest", total, e)
	}
	rowID = page[0].ID
	a, e := st.Artifact(ctx, rowID)
	if e != nil || a.Digest == "" || a.MediaType != "image/jpeg" {
		t.Fatal("capture lacks integrity metadata", e)
	}
}
