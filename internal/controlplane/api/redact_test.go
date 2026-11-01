package api

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/postfix/cctvscan/internal/probe"
	"github.com/postfix/cctvscan/internal/processor"
	"github.com/postfix/cctvscan/internal/streams"
)

// Exercise the real producer schema: Credentials is already json:"-", but
// CredentialState and MJPEGPaths are public evidence, not secrets or files.
func TestRedactHostResultEvidence(t *testing.T) {
	host := processor.HostResult{
		Host: "192.0.2.1", State: "completed", CredentialState: "inconclusive", Credentials: "private-credentials",
		MJPEGPaths:    []string{"http://private-user:private-password@192.0.2.1/video?token=private-query#private-fragment", "/video.mjpg?password=private-query#private-fragment"},
		HTTPMeta:      probe.HTTPMeta{State: "inconclusive", Endpoints: []probe.HTTPEndpoint{{URL: "http://192.0.2.1/", StatusCode: 401, Auth: "Basic private-auth", BodySnippet: "private-body"}}},
		RTSPInfo:      probe.RTSPInfo{Status: 401, AuthRequired: true, Any: true},
		StreamResults: []streams.StreamResult{{URL: "rtsp://private-user:private-password@192.0.2.1/live?token=private-query#private-fragment", Path: "/live?token=private-query", SavedPath: "/srv/private-file.jpg", State: "inconclusive", Evidence: "protocol_only", Type: streams.StreamRTSP, Status: 200}},
	}
	raw, err := json.Marshal(map[string]any{"result": host, "artifact_id": "artifact-opaque", "proxy_version_id": "proxy-opaque", "credential_set_id": "credential-opaque"})
	if err != nil {
		t.Fatal(err)
	}
	got := redact(raw)
	if strings.Contains(string(got), "private-") {
		t.Fatalf("sensitive producer values escaped: %s", got)
	}
	var out map[string]any
	if err = json.Unmarshal(got, &out); err != nil {
		t.Fatal(err)
	}
	result := out["result"].(map[string]any)
	if result["CredentialState"] != "inconclusive" || result["State"] != "completed" {
		t.Fatalf("credential uncertainty lost: %s", got)
	}
	if !reflect.DeepEqual(result["MJPEGPaths"], []any{"http://192.0.2.1/video", "/video.mjpg"}) {
		t.Fatalf("candidate evidence lost: %s", got)
	}
	stream := result["StreamResults"].([]any)[0].(map[string]any)
	for k, v := range map[string]any{"URL": "rtsp://192.0.2.1/live", "Path": "/live", "State": "inconclusive", "Evidence": "protocol_only", "Type": "RTSP", "Status": float64(200)} {
		if stream[k] != v {
			t.Errorf("stream %s=%v want %v", k, stream[k], v)
		}
	}
	for k, v := range map[string]any{"artifact_id": "artifact-opaque", "proxy_version_id": "proxy-opaque", "credential_set_id": "credential-opaque"} {
		if out[k] != v {
			t.Errorf("reference %s lost", k)
		}
	}
}

func TestRedactSensitiveKeysAndURLValues(t *testing.T) {
	raw := json.RawMessage(`{"CredentialState":"inconclusive","credential_state":"not_tested","proxy_status":"unavailable","artifact_ids":["opaque"],"headers":{"X":"private-header"},"RequestHeaders":"private-header","Cookies":"private-cookie","access_token":"private-token","password":"private-password","username":"private-user","Credentials":"private-credentials","BodySnippet":"private-body","response_body":"private-body","raw_payload":"private-raw","server_path":"/srv/private-file","proxy":"private-proxy","proxy_url":"http://private-proxy","url":"http://private-user:private-password@192.0.2.1/video?secret=private-query#private-fragment","MJPEGPaths":["//private-user:private-password@192.0.2.1/video?secret=private-query#private-fragment","file:///srv/private-file","http://bad%zz/private-url"],"message":"probe rtsp://private-user:private-password@192.0.2.1/live?token=private-query#private-fragment"}`)
	got := redact(raw)
	if strings.Contains(string(got), "private-") {
		t.Fatalf("sensitive value escaped: %s", got)
	}
	for _, want := range []string{`"CredentialState":"inconclusive"`, `"credential_state":"not_tested"`, `"proxy_status":"unavailable"`, `"artifact_ids":["opaque"]`, `http://192.0.2.1/video`, `//192.0.2.1/video`, `rtsp://192.0.2.1/live`} {
		if !strings.Contains(string(got), want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}
