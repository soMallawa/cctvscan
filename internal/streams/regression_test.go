package streams

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestSnapshotSafeFilenameAndImage(t *testing.T) {
	var buf bytes.Buffer
	jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(buf.Bytes())
	}))
	defer s.Close()
	dir := t.TempDir()
	saveStreamSnapshot(context.Background(), s.Client(), StreamResult{URL: s.URL + "/snapshot.jpg", Status: 200, Path: "/snapshot.jpg"}, dir)
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Fatalf("want one image file, got %d", len(files))
	}
}
func TestNeutralHTMLNotStream(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "ordinary website")
	}))
	defer s.Close()
	if r := checkStreamURL(context.Background(), s.Client(), s.URL+"/video", 80, "/video"); r != nil {
		t.Fatal("HTML/URL shape is not media evidence")
	}
}
func TestCandidatesAreNotSuccessfulStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, r := range checkRTSPStreams(ctx, "127.0.0.1", 554) {
		if r.Status == 200 {
			t.Fatal("invented successful status")
		}
	}
}
