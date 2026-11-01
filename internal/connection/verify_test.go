package connection

import (
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKnownCredentialsRequireARealChallengeAndDecodedFrame(t *testing.T) {
	for _, method := range []string{"Basic", "Digest-MD5", "Digest-SHA-256"} {
		t.Run(method, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				a := r.Header.Get("Authorization")
				valid := false
				if method == "Basic" {
					u, p, ok := r.BasicAuth()
					valid = ok && u == "fixture-user" && p == "fixture-pass"
				} else if strings.HasPrefix(a, "Digest ") {
					fields := map[string]string{}
					for _, m := range regexp.MustCompile(`([a-z]+)=(?:"([^"]*)"|([^, ]+))`).FindAllStringSubmatch(a, -1) {
						fields[m[1]] = m[2] + m[3]
					}
					h := func(s string) string {
						if method == "Digest-SHA-256" {
							v := sha256.Sum256([]byte(s))
							return hex.EncodeToString(v[:])
						}
						v := md5.Sum([]byte(s))
						return hex.EncodeToString(v[:])
					}
					expected := h(h("fixture-user:fixture-realm:fixture-pass") + ":fixture-nonce:" + fields["nc"] + ":" + fields["cnonce"] + ":auth:" + h("GET:/snapshot"))
					valid = fields["response"] == expected && fields["username"] == "fixture-user" && fields["uri"] == "/snapshot"
				}
				if !valid {
					challenge := `Basic realm="fixture-realm"`
					if method != "Basic" {
						challenge = fmt.Sprintf(`Digest realm="fixture-realm", nonce="fixture-nonce", algorithm=%s, qop="auth"`, strings.TrimPrefix(method, "Digest-"))
					}
					w.Header().Set("WWW-Authenticate", challenge)
					w.WriteHeader(401)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				png.Encode(w, image.NewRGBA(image.Rect(0, 0, 8, 8)))
			}))
			defer s.Close()
			host, port, _ := net.SplitHostPort(s.Listener.Addr().String())
			pn, _ := strconv.Atoi(port)
			out := Verify(context.Background(), Input{Host: host, Port: pn, Scheme: "http", Path: "/snapshot", Username: "fixture-user", Password: "fixture-pass"})
			if out.State != "frame_verified" || out.Authentication != "accepted" || out.Media != "decoded_frame" || out.Width != 8 || out.FrameBase64 == "" {
				t.Fatalf("challenge/frame verification failed: %#v", out)
			}
		})
	}
}
func TestHTMLLoginIsUnsupportedNotFalseAuthenticationSuccess(t *testing.T) {
	var hits atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<form><input type="password"></form>`))
	}))
	defer s.Close()
	host, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	out := Verify(context.Background(), Input{Host: host, Port: pn, Scheme: "http", Path: "/", Username: "fixture-user", Password: "fixture-pass"})
	if out.Authentication != "unsupported_form" || out.Media == "decoded_frame" || hits.Load() != 1 {
		t.Fatalf("form success fabricated: %#v", out)
	}
}
func TestProxyFailureNeverFallsBackAndRTSPIsExplicit(t *testing.T) {
	var direct atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { direct.Add(1) }))
	defer target.Close()
	host, port, _ := net.SplitHostPort(target.Listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer proxy.Close()
	p, _ := url.Parse(proxy.URL)
	out := Verify(context.Background(), Input{Host: host, Port: pn, Scheme: "http", Path: "/", Proxy: p})
	if direct.Load() != 0 || out.State != "inconclusive" {
		t.Fatal("proxy failure caused fallback or success")
	}
	out = Verify(context.Background(), Input{Host: host, Port: pn, Scheme: "rtsp", Path: "/", AllowDirectRTSP: true, Proxy: p})
	if out.State != "blocked" || direct.Load() != 0 {
		t.Fatal("RTSP bypassed HTTP proxy boundary")
	}
}

func TestLiveMJPEGDecodesOnePartWithoutWaitingForStreamEnd(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=AaB03x")
		io.WriteString(w, "--AaB03x\r\nContent-Type: image/png\r\n\r\n")
		png.Encode(w, image.NewRGBA(image.Rect(0, 0, 8, 8)))
		io.WriteString(w, "\r\n--AaB03x\r\nContent-Type: image/png\r\n\r\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer s.Close()
	host, port, _ := net.SplitHostPort(s.Listener.Addr().String())
	pn, _ := strconv.Atoi(port)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	out := Verify(ctx, Input{Host: host, Port: pn, Scheme: "http", Path: "/video"})
	if out.State != "frame_verified" {
		t.Fatal("live multipart frame was not decoded before the stream ended")
	}
}
