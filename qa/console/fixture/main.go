// Owned ordinary-web negative control and synthetic image positive control.
// No camera, credentials or external network connection is involved.
package main

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"log"
	"net/http"
	"time"
)

func main() {
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for y := 0; y < 90; y++ {
		for x := 0; x < 160; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y * 2), 120, 255})
		}
	}
	if err := jpeg.Encode(&data, img, nil); err != nil {
		log.Fatal(err)
	}
	go func() {
		server := &http.Server{Addr: ":18082", ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", "Hikvision-QA-Synthetic")
			switch r.URL.Path {
			case "/snapshot.jpg":
				w.Header().Set("Content-Type", "image/jpeg")
				w.Write(data.Bytes())
			case "/":
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte("<title>IP Camera — synthetic owned fixture, not a device</title>"))
			default:
				http.NotFound(w, r)
			}
		})}
		log.Fatal(server.ListenAndServe())
	}()
	server := &http.Server{Addr: ":18080", ReadHeaderTimeout: 3 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Owned QA ordinary HTTP service. Not a camera.\n"))
	})}
	log.Fatal(server.ListenAndServe())
}
