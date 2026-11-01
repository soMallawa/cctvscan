// Disposable camera used only on the private acceptance Docker network.
package main

import (
	"encoding/json"
	"image"
	"image/png"
	"net/http"
	"sync/atomic"
)

func main() {
	var other atomic.Int64
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/qa-counters" {
			json.NewEncoder(w).Encode(map[string]int64{"unrelated_vendor_attempts": other.Load()})
			return
		}
		w.Header().Set("Server", "Hikvision")
		u, p, ok := r.BasicAuth()
		if ok && u == "other-vendor" {
			other.Add(1)
		}
		if r.URL.Path != "/" && r.URL.Path != "/ISAPI/Streaming/channels/101/picture" {
			http.NotFound(w, r)
			return
		}
		if !ok || u != "fixture-user" || p != "fixture-pass" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Hikvision owned fixture"`)
			w.WriteHeader(401)
			w.Write([]byte("<title>Hikvision DS-2CD2087G2-L — owned acceptance fixture</title>"))
			return
		}
		if r.URL.Path == "/" {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<title>Hikvision DS-2CD2087G2-L — owned acceptance fixture</title>"))
			return
		}
		w.Header().Set("Content-Type", "image/png")
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 64, 48)))
	})
	if e := http.ListenAndServe(":8080", nil); e != nil {
		panic("owned fixture listener failed")
	}
}
