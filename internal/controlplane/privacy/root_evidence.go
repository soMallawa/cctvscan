package privacy

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"
)

func rootEvidenceField(key string) bool {
	switch key {
	case "version", "method", "authenticated", "capturedat", "sha256", "bytesbase64", "capturedbytes", "limit", "truncated", "wwwauthenticate":
		return true
	}
	return false
}

// The one explicit raw-evidence exception is a bounded unauthenticated root GET
// archive with byte-exact integrity metadata. Generic bodies remain redacted.
func validRootEvidence(value any) bool {
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	if m["Version"] != float64(1) || m["Method"] != "GET" || m["Authenticated"] != false || m["Limit"] != float64(8192) {
		return false
	}
	encoded, ok := m["BytesBase64"].(string)
	if !ok || len(encoded) > 10924 {
		return false
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) > 8192 || m["CapturedBytes"] != float64(len(data)) {
		return false
	}
	digest := sha256.Sum256(data)
	hash, ok := m["SHA256"].(string)
	if !ok || hash != hex.EncodeToString(digest[:]) {
		return false
	}
	stamp, ok := m["CapturedAt"].(string)
	if !ok {
		return false
	}
	if _, err = time.Parse(time.RFC3339Nano, stamp); err != nil {
		return false
	}
	_, ok = m["Truncated"].(bool)
	if !ok {
		return false
	}
	challenge, ok := m["WWWAuthenticate"].(string)
	if !ok || len(challenge) > 8192 || strings.ContainsAny(challenge, "\r\n") {
		return false
	}
	return true
}
