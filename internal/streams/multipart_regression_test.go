package streams

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"testing"
)

func TestMixedCaseMultipartBoundaryDiscoveryAndCapture(t *testing.T) {
	var body, frame bytes.Buffer
	if err := jpeg.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 2, 2)), nil); err != nil {
		t.Fatal(err)
	}
	mw := multipart.NewWriter(&body)
	if err := mw.SetBoundary("CameraFrameAbC123"); err != nil {
		t.Fatal(err)
	}
	part, err := mw.CreatePart(textproto.MIMEHeader{"Content-Type": {"image/jpeg"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(frame.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = mw.Close(); err != nil {
		t.Fatal(err)
	}
	const ct = "Multipart/X-Mixed-Replace; boundary=CameraFrameAbC123"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", ct)
		w.Write(body.Bytes())
	}))
	defer s.Close()
	// Exercise capture independently too, so discovery cannot mask a parsing regression.
	path, err := SaveSnapshot(context.Background(), s.Client(), StreamResult{URL: s.URL}, t.TempDir())
	if err != nil {
		t.Fatalf("capture mixed-case boundary: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err = jpeg.Decode(f); err != nil {
		t.Fatalf("saved JPEG: %v", err)
	}
	result, err := checkStreamURLDetailed(context.Background(), s.Client(), s.URL, 80, "/")
	if err != nil {
		t.Fatal(err)
	}
	if result == nil || !result.Verified || result.Type != StreamMJPEG {
		t.Fatalf("mixed-case multipart JPEG not discovered: %+v", result)
	}
	if result.ContentType != ct {
		t.Fatalf("Content-Type parameters changed: %q", result.ContentType)
	}
}
