//go:build integration

package api

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestStoredCameraConnectionJourneyAndSecretFreeHistory(t *testing.T) {
	s := apiFixture(t)
	ctx := context.Background()
	db, e := pgxpool.New(ctx, os.Getenv("CCTVSCAN_TEST_DATABASE_URL"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(db.Close)
	camera := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != "fixture-user" || p != "fixture-pass" {
			w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 16, 12)))
	}))
	defer camera.Close()
	_, port, _ := net.SplitHostPort(camera.Listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	cfg := config.DefaultConfig()
	cfg.Targets = []string{"127.0.0.1"}
	cfg.PortMode = "custom"
	cfg.Ports = port
	raw, _ := json.Marshal(cfg)
	jid, aid := uuid.NewString(), uuid.NewString()
	_, e = db.Exec(ctx, `INSERT INTO jobs(id,name,state,config,priority,request_hash) VALUES($1,'owned connection fixture','completed',$2,2,$3)`, jid, raw, []byte{1})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Exec(ctx, `DELETE FROM results WHERE job_id=$1`, jid)
		db.Exec(ctx, `DELETE FROM attempts WHERE job_id=$1`, jid)
		db.Exec(ctx, `DELETE FROM jobs WHERE id=$1`, jid)
	})
	_, e = db.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,'completed')`, aid, jid, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(map[string]any{"Host": "127.0.0.1", "Ports": []int{pn}, "HTTPPorts": []int{pn}, "Brand": "Hikvision", "CameraDetection": map[string]any{"Brand": "Hikvision", "PageTitle": "IP Camera", "State": "possible"}})
	var rid int64
	if e = db.QueryRow(ctx, `INSERT INTO results(job_id,attempt_id,target,brand,state,data) VALUES($1,$2,'127.0.0.1','Hikvision','complete',$3) RETURNING id`, jid, aid, data).Scan(&rid); e != nil {
		t.Fatal(e)
	}
	policy := config.DefaultPolicy()
	policy.AllowCredentials = true
	policy.AllowCapture = true
	pr, _ := json.Marshal(policy)
	db.Exec(ctx, `UPDATE settings SET policy=$1`, pr)
	base := "/api/results/" + strconv.FormatInt(rid, 10)
	w := request(t, s, "GET", base+"/catalogue", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "brand_only") {
		t.Fatalf("stored match %d", w.Code)
	}
	body := map[string]any{"scheme": "http", "port": pn, "path": "/snapshot", "username": "fixture-user", "password": "fixture-pass"}
	w = request(t, s, "POST", base+"/connections", body)
	if w.Code != 201 || !strings.Contains(w.Body.String(), "frame_verified") {
		t.Fatalf("connection %d %s", w.Code, w.Body.String())
	}
	w = request(t, s, "GET", base+"/connections", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "frame_verified") || strings.Contains(w.Body.String(), "fixture-pass") {
		t.Fatal("verification history missing or leaked credential")
	}
	body["port"] = pn + 1
	w = request(t, s, "POST", base+"/connections", body)
	if w.Code != 400 {
		t.Fatalf("unretained port accepted: %d", w.Code)
	}
}
