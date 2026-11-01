package processor

import (
	"context"
	"encoding/json"
	"image"
	"image/png"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalScanAutomaticallyUsesVendorCredentialsAndDecodesFrame(t *testing.T) {
	var unrelated atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "Hikvision")
		u, pass, ok := r.BasicAuth()
		if ok && u == "other-vendor" {
			unrelated.Add(1)
		}
		if r.URL.Path != "/" && r.URL.Path != "/ISAPI/Streaming/channels/101/picture" {
			http.NotFound(w, r)
			return
		}
		if !ok || u != "fixture-user" || pass != "fixture-pass" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Hikvision camera"`)
			w.WriteHeader(401)
			w.Write([]byte("<title>Hikvision Network Camera</title>"))
			return
		}
		if r.URL.Path == "/" {
			w.Write([]byte("<title>Hikvision Network Camera</title>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 16, 12)))
	})
	var listener net.Listener
	for _, port := range []int{8080, 8000, 8001, 8888} {
		listener, _ = net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if listener != nil {
			break
		}
	}
	if listener == nil {
		t.Fatal("no owned HTTP fixture port available")
	}
	s := httptest.NewUnstartedServer(handler)
	s.Listener = listener
	s.Start()
	defer s.Close()
	_, rawport, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(rawport)
	dir := t.TempDir()
	file := filepath.Join(dir, "creds.txt")
	os.WriteFile(file, []byte("# Dahua\nother-vendor:fixture-other\n# Hikvision\nfixture-user:fixture-pass\n# Generic\nfixture-generic:fixture-other\n"), 0600)
	p := NewOptimizedProcessor(false, file, dir)
	p.Options.CheckCredentials = true
	p.Options.Capture = true
	p.Options.HostTimeout = 15 * time.Second
	results := p.ProcessHosts(context.Background(), map[string][]int{"127.0.0.1": {port}})
	r := results[0]
	verified := false
	for _, c := range r.AutomaticVerification {
		verified = verified || c.Outcome.State == "frame_verified" && c.Outcome.Authentication == "accepted" && c.Outcome.FrameBase64 != ""
	}
	if !verified || unrelated.Load() != 0 {
		t.Fatal("normal scan did not automatically select vendor credentials and decode a frame")
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "fixture-pass") || strings.Contains(string(raw), "other-vendor") {
		t.Fatal("automatic evidence leaked credential material")
	}
}
