//go:build integration

package api

import (
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"testing"
)

func TestSchedulePolicyControls(t *testing.T) {
	h := apiFixture(t)
	c := config.DefaultConfig()
	c.Targets = []string{"127.0.0.1"}
	body := map[string]any{"name": "owned disabled schedule policy fixture", "config": c, "cron": "* * * * *", "timezone": "UTC", "enabled": false, "misfire_policy": "catch_up_once", "overlap_policy": "queue"}
	created := request(t, h, "POST", "/api/schedules", body)
	if created.Code != 201 {
		t.Fatal("valid schedule policies rejected", created.Code, created.Body.String())
	}
	var obj map[string]any
	json.Unmarshal(created.Body.Bytes(), &obj)
	if obj["misfire_policy"] != "catch_up_once" || obj["overlap_policy"] != "queue" {
		t.Fatal("schedule policies were not persisted")
	}
	body["misfire_policy"] = "replay_everything"
	if r := request(t, h, "POST", "/api/schedules", body); r.Code != 400 {
		t.Fatal("unbounded misfire replay accepted", r.Code)
	}
}
