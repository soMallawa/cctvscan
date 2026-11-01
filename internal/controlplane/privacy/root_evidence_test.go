package privacy

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestInvalidRootEvidenceIsNotExposed(t *testing.T) {
	for _, raw := range []string{
		`{"RootEvidence":{"Version":1,"Method":"POST","Authenticated":true,"BytesBase64":"c2VjcmV0"}}`,
		`{"RootEvidence":{"Version":1,"Method":"GET","Authenticated":false,"BytesBase64":"!invalid"}}`,
		`{"RootEvidence":{"Version":1,"Method":"GET","Authenticated":false,"CapturedAt":"2026-11-01T00:00:00Z","Limit":8192,"CapturedBytes":1,"SHA256":"bad","BytesBase64":"eA==","Truncated":false}}`,
	} {
		got := Sanitize(json.RawMessage(raw))
		if strings.Contains(string(got), "RootEvidence") {
			t.Fatalf("malformed/untrusted evidence was exposed: %s", got)
		}
	}
}

func TestValidRootEvidenceSurvivesWithoutCredentialFields(t *testing.T) {
	data := []byte("<title>Owned fixture</title>")
	sum := sha256.Sum256(data)
	archive := map[string]any{"Version": 1, "Method": "GET", "Authenticated": false, "CapturedAt": "2026-11-01T00:00:00Z", "Limit": 8192, "CapturedBytes": len(data), "SHA256": hex.EncodeToString(sum[:]), "BytesBase64": base64.StdEncoding.EncodeToString(data), "Truncated": false, "WWWAuthenticate": "Basic fixture", "UnexpectedSecret": "must-not-survive"}
	raw, _ := json.Marshal(map[string]any{"RootEvidence": archive, "password": "must-not-survive", "BodySnippet": "must-not-survive"})
	clean := Sanitize(raw)
	var got map[string]any
	if err := json.Unmarshal(clean, &got); err != nil {
		t.Fatal(err)
	}
	e, ok := got["RootEvidence"].(map[string]any)
	if !ok {
		t.Fatalf("valid root evidence discarded: %s", clean)
	}
	if e["BytesBase64"] != archive["BytesBase64"] || e["SHA256"] != archive["SHA256"] {
		t.Fatal("byte-exact replay archive changed")
	}
	if strings.Contains(string(clean), "must-not-survive") {
		t.Fatal("credential/unexpected fields leaked")
	}
}
