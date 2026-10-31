package processor

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
)

func TestCaptureOptInAndJoined(t *testing.T) {
	var b bytes.Buffer
	if e := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); e != nil {
		t.Fatal(e)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Header().Set("Server", "Hikvision")
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("Hikvision camera"))
			return
		}
		if r.URL.Path == "/snapshot.jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(b.Bytes())
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer s.Close()
	h, p, _ := net.SplitHostPort(s.Listener.Addr().String())
	port, _ := strconv.Atoi(p)
	for _, capture := range []bool{false, true} {
		dir := t.TempDir()
		e := NewOptimizedProcessor(false, "", dir)
		e.Options.Capture = capture
		rs := e.ProcessHosts(context.Background(), map[string][]int{h: {port}})
		if len(rs) != 1 || rs[0].Error != nil {
			t.Fatalf("capture=%t: %+v", capture, rs)
		}
		saved := 0
		for _, r := range rs[0].StreamResults {
			if r.SavedPath != "" {
				saved++
				data, err := os.ReadFile(r.SavedPath)
				if err != nil {
					t.Fatal(err)
				}
				if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if capture && saved != 1 {
			t.Fatalf("capture not complete at return: %d", saved)
		}
		if !capture && saved != 0 {
			t.Fatal("capture occurred without opt-in")
		}
	}
}
