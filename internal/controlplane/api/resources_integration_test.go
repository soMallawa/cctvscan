//go:build integration

package api

import (
	"encoding/json"
	"github.com/google/uuid"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"strings"
	"testing"
)

func TestResourceLifecycleHTTP(t *testing.T) {
	s := apiFixture(t)
	name := "resource-" + uuid.NewString()
	c := config.DefaultConfig()
	c.Targets = []string{"192.0.2.2"}
	for _, v := range []struct {
		kind string
		body map[string]any
	}{{"proxies", map[string]any{"name": name, "url": "http://private-user:private-password@127.0.0.1:18080"}}, {"credential-sets", map[string]any{"name": name, "content": "admin:private-password\n"}}, {"profiles", map[string]any{"name": name, "config": c}}, {"schedules", map[string]any{"name": name, "config": c, "cron": "0 12 * * *", "timezone": "UTC", "enabled": true}}} {
		t.Run(v.kind, func(t *testing.T) {
			w := request(t, s, "POST", "/api/"+v.kind, v.body)
			if w.Code != 201 {
				t.Fatalf("create %d %s", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "private-") {
				t.Fatal("secret disclosed")
			}
			var obj map[string]any
			json.Unmarshal(w.Body.Bytes(), &obj)
			id := obj["id"].(string)
			if w = request(t, s, "GET", "/api/"+v.kind, nil); w.Code != 200 || !strings.Contains(w.Body.String(), id) || strings.Contains(w.Body.String(), "private-") {
				t.Fatal("metadata read", w.Code, w.Body.String())
			}
			if v.kind != "credential-sets" {
				v.body["name"] = name + " updated"
				if w = request(t, s, "PUT", "/api/"+v.kind+"/"+id, v.body); w.Code != 200 {
					t.Fatal("update", w.Code, w.Body.String())
				}
			}
			if w = request(t, s, "DELETE", "/api/"+v.kind+"/"+id, nil); w.Code != 204 {
				t.Fatal("delete", w.Code, w.Body.String())
			}
		})
	}
}
func TestSettingsCapsHTTP(t *testing.T) {
	s := apiFixture(t)
	p := config.DefaultPolicy()
	for _, body := range []map[string]any{{"policy": p, "retention_days": 30, "max_concurrent_jobs": 2}, {"policy": func() config.Policy { v := p; v.AllowRaw = true; return v }(), "retention_days": 30, "max_concurrent_jobs": 1}} {
		w := request(t, s, "PUT", "/api/settings", body)
		if w.Code != 400 {
			t.Fatal("unsupported settings accepted", w.Code, w.Body.String())
		}
	}
	w := request(t, s, "PUT", "/api/settings", map[string]any{"policy": p, "retention_days": 30, "max_concurrent_jobs": 1})
	if w.Code != 200 {
		t.Fatal("valid settings", w.Code, w.Body.String())
	}
}
