package probe

import (
	"encoding/json"
	"testing"
)

func TestCameraTriageRequiresCorroboratedEvidence(t *testing.T) {
	cases := []struct {
		name     string
		e        HTTPEndpoint
		state    string
		positive bool
	}{
		{"request failure is unknown", HTTPEndpoint{State: "failed", Error: "HTTP request failed"}, "unknown", false},
		{"ordinary HTML stays unknown", HTTPEndpoint{State: "responded", StatusCode: 200, BodySnippet: "<title>Business home</title>"}, "unknown", false},
		{"generic client is review candidate", HTTPEndpoint{State: "responded", StatusCode: 200, BodySnippet: "<title>Web Client</title>"}, "possible", false},
		{"embedded server is not device proof", HTTPEndpoint{State: "responded", StatusCode: 200, Server: "Webs"}, "possible", false},
		{"bare DVR mention is not camera proof", HTTPEndpoint{State: "responded", StatusCode: 200, BodySnippet: "<p>We sell DVR accessories</p>"}, "possible", false},
		{"vendor header alone is insufficient", HTTPEndpoint{State: "responded", StatusCode: 200, Server: "Hikvision"}, "possible", false},
		{"vendor and camera context support label", HTTPEndpoint{State: "responded", StatusCode: 200, Server: "Hikvision", BodySnippet: "<title>Hikvision IP Camera</title>"}, "supported", true},
		{"error page cannot become supported", HTTPEndpoint{State: "responded", StatusCode: 404, Server: "Hikvision", BodySnippet: "Hikvision IP Camera not found"}, "possible", false},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got := CameraFromEndpoint(tt.e)
			raw, _ := json.Marshal(got)
			var saved map[string]any
			_ = json.Unmarshal(raw, &saved)
			if saved["State"] != tt.state || got.IsCamera != tt.positive {
				t.Fatalf("need triage=%s positive=%t, got %s", tt.state, tt.positive, raw)
			}
			if saved["Reason"] == "" || saved["Reason"] == nil || saved["EvidenceVersion"] != float64(1) {
				t.Fatalf("missing auditable decision: %s", raw)
			}
		})
	}
}
