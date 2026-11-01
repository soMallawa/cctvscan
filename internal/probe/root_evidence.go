package probe

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"
)

const RootEvidenceLimit = 8192
const HostEvidenceLimit = 32768

// RootEvidence archives only the bounded body of an unauthenticated root GET.
// It never includes request authorization, cookies, or credential submissions.
type RootEvidence struct {
	Version         int
	Method          string
	Authenticated   bool
	CapturedAt      string
	SHA256          string
	BytesBase64     string
	CapturedBytes   int
	Limit           int
	Truncated       bool
	WWWAuthenticate string
}

func captureRootEvidence(data []byte, truncated bool, challenge string) *RootEvidence {
	digest := sha256.Sum256(data)
	return &RootEvidence{Version: 1, Method: "GET", CapturedAt: time.Now().UTC().Format(time.RFC3339Nano), SHA256: hex.EncodeToString(digest[:]), BytesBase64: base64.StdEncoding.EncodeToString(data), CapturedBytes: len(data), Limit: RootEvidenceLimit, Truncated: truncated, WWWAuthenticate: challenge}
}
