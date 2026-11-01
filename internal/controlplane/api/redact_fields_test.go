package api

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactComposedSensitiveFields(t *testing.T) {
	raw := json.RawMessage(`{"db_password":"private-password","login_username":"private-user","bearerToken":"private-token","http_headers":"private-header","request_payload":"private-payload","capture_path":"/srv/private-file","proxyPlaintext":"private-proxy","credential_state":"inconclusive","proxy_test_status":"unavailable","credential_set_id":"opaque-credential","artifact_id":"opaque-artifact","StreamResults":[{"Path":"/video?token=private-query","Evidence":"media_header","State":"inconclusive"}]}`)
	got := string(redact(raw))
	if strings.Contains(got, "private-") {
		t.Fatalf("composed sensitive field leaked: %s", got)
	}
	for _, want := range []string{`"credential_state":"inconclusive"`, `"proxy_test_status":"unavailable"`, `"credential_set_id":"opaque-credential"`, `"artifact_id":"opaque-artifact"`, `"Path":"/video"`, `"Evidence":"media_header"`, `"State":"inconclusive"`} {
		if !strings.Contains(got, want) {
			t.Errorf("safe field lost %s in %s", want, got)
		}
	}
}
