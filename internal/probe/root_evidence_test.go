package probe

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestRootResponseEvidenceIsBoundedAndReplayable(t *testing.T) {
	body := strings.Repeat("x", 9000)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("root request must be unauthenticated")
		}
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Set-Cookie", "private-cookie")
		w.Header().Set("WWW-Authenticate", `Basic realm="fixture"`)
		_, _ = w.Write([]byte(body))
	}))
	defer s.Close()
	_, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	p, _ := strconv.Atoi(port)
	meta := ProbeHTTPMeta(context.Background(), "127.0.0.1", []int{p})
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Endpoints []struct {
			RootEvidence *struct {
				Version                                          int
				Method                                           string
				Authenticated                                    bool
				BytesBase64, SHA256, CapturedAt, WWWAuthenticate string
				CapturedBytes, Limit                             int
				Truncated                                        bool
			}
		}
	}
	if err = json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Endpoints) != 1 || got.Endpoints[0].RootEvidence == nil {
		t.Fatal("unauthenticated root response body was discarded from saved evidence")
	}
	e := got.Endpoints[0].RootEvidence
	captured, err := base64.StdEncoding.DecodeString(e.BytesBase64)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(captured)
	if string(captured) != body[:8192] || e.CapturedBytes != 8192 || e.Limit != 8192 || !e.Truncated || e.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("invalid bounded evidence: bytes=%d record=%+v", len(captured), e)
	}
	if e.Version != 1 || e.Method != "GET" || e.Authenticated || e.CapturedAt == "" || e.WWWAuthenticate != `Basic realm="fixture"` {
		t.Fatalf("missing evidence provenance: %+v", e)
	}
	if strings.Contains(string(raw), "private-cookie") {
		t.Fatal("session cookie leaked into evidence")
	}
}
