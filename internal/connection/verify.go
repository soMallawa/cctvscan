// Package connection verifies one explicitly selected, scoped connection using
// known credentials. It never iterates password lists or candidate path lists.
package connection

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/postfix/cctvscan/internal/httptransport"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Input struct {
	Host                             string
	Port                             int
	Scheme, Path, Username, Password string
	Parameters                       map[string]string
	Proxy                            *url.URL
	AllowDirectRTSP                  bool
}
type Outcome struct {
	State          string `json:"state"`
	Authentication string `json:"authentication"`
	AuthMethod     string `json:"auth_method,omitempty"`
	Media          string `json:"media"`
	Reason         string `json:"reason"`
	Route          string `json:"route"`
	Target         string `json:"target"`
	Status         int    `json:"status,omitempty"`
	Width          int    `json:"width,omitempty"`
	Height         int    `json:"height,omitempty"`
	FrameBase64    string `json:"frame_base64,omitempty"`
	DurationMS     int64  `json:"duration_ms"`
}

func Verify(parent context.Context, in Input) (out Outcome) {
	start := time.Now()
	out = Outcome{State: "inconclusive", Authentication: "not_tested", Media: "not_verified", Route: "direct", Reason: "Connection could not be verified."}
	defer func() { out.DurationMS = time.Since(start).Milliseconds() }()
	if in.Proxy != nil {
		out.Route = "http_proxy"
	}
	if net.ParseIP(in.Host) == nil || in.Port < 1 || in.Port > 65535 || len(in.Username) > 200 || len(in.Password) > 1024 {
		out.State = "blocked"
		out.Reason = "Use a retained numeric target and an allowed port."
		return
	}
	if in.Scheme != "http" && in.Scheme != "https" && in.Scheme != "rtsp" {
		out.State = "unsupported"
		out.Reason = "Only HTTP, HTTPS and explicitly enabled direct RTSP are supported."
		return
	}
	out.Target = in.Scheme + "://" + net.JoinHostPort(in.Host, strconv.Itoa(in.Port))
	if in.Scheme == "rtsp" && (!in.AllowDirectRTSP || in.Proxy != nil) {
		out.State = "blocked"
		out.Reason = "Direct RTSP requires explicit opt-in and cannot use or bypass an HTTP proxy job."
		return
	}
	path, ok := expandPath(in.Path, in.Parameters, in.Username, in.Password)
	if !ok {
		out.State = "needs_parameters"
		out.Reason = "Fill all profile placeholders, or provide a valid relative path."
		return
	}
	u, e := url.Parse(out.Target + path)
	if e != nil {
		out.State = "blocked"
		out.Reason = "Invalid connection path."
		return
	}
	ctx, cancel := context.WithTimeout(parent, 18*time.Second)
	defer cancel()
	if in.Scheme == "rtsp" {
		return verifyRTSP(ctx, in, u, out)
	}
	client := httptransport.NewScopedCameraClient(in.Proxy)
	defer client.CloseIdleConnections()
	do := func(authorization string) (*http.Response, error) {
		req, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
		if e != nil {
			return nil, e
		}
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		return httptransport.Do(client, req)
	}
	resp, e := do("")
	if e != nil {
		out.Reason = "Request failed or timed out on the selected route; no direct fallback was attempted."
		return
	}
	if resp.StatusCode == 401 {
		challenge := chooseChallenge(resp.Header.Values("WWW-Authenticate"))
		resp.Body.Close()
		out.Authentication = "required"
		if in.Username == "" && in.Password == "" {
			out.State = "authentication_required"
			out.Reason = "This endpoint requires known credentials."
			return
		}
		authorization, method, ok := authorization(challenge, "GET", u.RequestURI(), in.Username, in.Password)
		out.AuthMethod = method
		if !ok {
			out.State = "unsupported"
			out.Authentication = "unsupported_method"
			out.Reason = "The advertised authentication method is not supported; no credentials were sent."
			return
		}
		resp, e = do(authorization)
		if e != nil {
			out.Authentication = "inconclusive"
			out.Reason = "Authenticated request failed or timed out."
			return
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			out.Authentication = "accepted"
		} else if resp.StatusCode == 401 || resp.StatusCode == 403 {
			out.Authentication = "rejected"
		} else {
			out.Authentication = "inconclusive"
		}
	} else {
		out.Authentication = "not_required"
	}
	defer resp.Body.Close()
	out.Status = resp.StatusCode
	if out.Authentication == "rejected" {
		out.State = "authentication_rejected"
		out.Reason = "The endpoint rejected the supplied credential pair."
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		out.Reason = "Endpoint returned a non-success response; redirects are not followed."
		return
	}
	mediaType, params, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	var reader io.Reader = resp.Body
	if strings.HasPrefix(mediaType, "multipart/") && params["boundary"] != "" {
		part, err := multipart.NewReader(resp.Body, params["boundary"]).NextPart()
		if err != nil {
			out.Reason = "No readable multipart frame was received."
			return
		}
		defer part.Close()
		reader = part
	}
	data, e := io.ReadAll(io.LimitReader(reader, (2<<20)+1))
	if e != nil || len(data) > 2<<20 {
		out.Reason = "Response was incomplete or exceeded the 2 MiB verification budget."
		return
	}
	if mediaType == "text/html" {
		out.State = "responded"
		out.Media = "not_media"
		out.Reason = "HTML response received; it is not camera or media proof."
		lower := strings.ToLower(string(data))
		if strings.Contains(lower, "password") && out.Authentication != "accepted" {
			out.Authentication = "unsupported_form"
			out.Reason = "HTML login forms require a vendor-specific adapter; no form submission was attempted."
		}
		return
	}
	if setFrame(&out, data) {
		return
	}
	out.State = "responded"
	out.Media = "not_decoded"
	out.Reason = "Endpoint responded, but no image frame decoded within the verification budget."
	return
}

var placeholder = regexp.MustCompile(`\{([A-Za-z0-9_]+)\}|<N>`)

func expandPath(path string, params map[string]string, user, pass string) (string, bool) {
	if len(path) == 0 || len(path) > 2048 || !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "\r\n#\\") {
		return "", false
	}
	valid := true
	path = placeholder.ReplaceAllStringFunc(path, func(p string) string {
		k := strings.Trim(p, "{}")
		if p == "<N>" {
			k = "N"
		}
		v, ok := params[k]
		if k == "user" {
			v = user
			ok = user != ""
		}
		if k == "pass" {
			v = pass
			ok = user != "" || pass != ""
		}
		if !ok || len(v) > 1024 {
			valid = false
		}
		return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
	})
	if strings.ContainsAny(path, "{}<>") {
		valid = false
	}
	u, e := url.ParseRequestURI(path)
	return path, valid && e == nil && u.Host == "" && u.User == nil && u.Fragment == ""
}
func setFrame(out *Outcome, data []byte) bool {
	config, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || config.Width < 1 || config.Height < 1 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16<<20 {
		return false
	}
	img, _, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return false
	}
	out.Width = config.Width
	out.Height = config.Height
	width, height := config.Width, config.Height
	if width > 960 {
		height = height * 960 / width
		width = 960
	}
	if height > 960 {
		width = width * 960 / height
		height = 960
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	thumb := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			thumb.Set(x, y, img.At(x*config.Width/width, y*config.Height/height))
		}
	}
	var encoded bytes.Buffer
	if jpeg.Encode(&encoded, thumb, &jpeg.Options{Quality: 65}) == nil && encoded.Len() <= 256<<10 {
		out.FrameBase64 = base64.StdEncoding.EncodeToString(encoded.Bytes())
	}
	out.State = "frame_verified"
	out.Media = "decoded_frame"
	out.Reason = "An image frame decoded. This verifies media access, not a specific physical model."
	return true
}

var authFields = regexp.MustCompile(`([A-Za-z0-9_-]+)\s*=\s*("(?:[^"\\]|\\.)*"|[^,\s]+)`)

func chooseChallenge(values []string) string {
	for _, v := range values {
		if strings.HasPrefix(strings.ToLower(v), "digest ") {
			return v
		}
	}
	for _, v := range values {
		if strings.HasPrefix(strings.ToLower(v), "basic ") {
			return v
		}
	}
	return ""
}
func authorization(challenge, method, uri, user, pass string) (string, string, bool) {
	if strings.HasPrefix(strings.ToLower(challenge), "basic ") {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass)), "Basic", true
	}
	if !strings.HasPrefix(strings.ToLower(challenge), "digest ") {
		return "", "", false
	}
	fields := map[string]string{}
	for _, m := range authFields.FindAllStringSubmatch(challenge, -1) {
		v := m[2]
		if strings.HasPrefix(v, "\"") {
			decoded, e := strconv.Unquote(v)
			if e != nil {
				return "", "Digest", false
			}
			v = decoded
		}
		fields[strings.ToLower(m[1])] = v
	}
	algo := strings.ToUpper(fields["algorithm"])
	if algo == "" {
		algo = "MD5"
	}
	base := strings.TrimSuffix(algo, "-SESS")
	if base != "MD5" && base != "SHA-256" || fields["nonce"] == "" {
		return "", "Digest", false
	}
	qop := ""
	if fields["qop"] != "" {
		for _, q := range strings.Split(fields["qop"], ",") {
			if strings.TrimSpace(q) == "auth" {
				qop = "auth"
			}
		}
		if qop == "" {
			return "", "Digest", false
		}
	}
	h := func(s string) string {
		if base == "MD5" {
			v := md5.Sum([]byte(s))
			return hex.EncodeToString(v[:])
		}
		v := sha256.Sum256([]byte(s))
		return hex.EncodeToString(v[:])
	}
	var random [16]byte
	if _, e := rand.Read(random[:]); e != nil {
		return "", "Digest", false
	}
	cnonce := hex.EncodeToString(random[:])
	nc := "00000001"
	ha1 := h(user + ":" + fields["realm"] + ":" + pass)
	if strings.HasSuffix(algo, "-SESS") {
		ha1 = h(ha1 + ":" + fields["nonce"] + ":" + cnonce)
	}
	ha2 := h(method + ":" + uri)
	response := h(ha1 + ":" + fields["nonce"] + ":" + ha2)
	if qop != "" {
		response = h(ha1 + ":" + fields["nonce"] + ":" + nc + ":" + cnonce + ":" + qop + ":" + ha2)
	}
	value := fmt.Sprintf("Digest username=%s, realm=%s, nonce=%s, uri=%s, response=%s, algorithm=%s", strconv.Quote(user), strconv.Quote(fields["realm"]), strconv.Quote(fields["nonce"]), strconv.Quote(uri), strconv.Quote(response), algo)
	if qop != "" {
		value += ", qop=auth, nc=" + nc + ", cnonce=" + strconv.Quote(cnonce)
	} else if strings.HasSuffix(algo, "-SESS") {
		value += ", cnonce=" + strconv.Quote(cnonce)
	}
	if fields["opaque"] != "" {
		value += ", opaque=" + strconv.Quote(fields["opaque"])
	}
	return value, "Digest " + algo, true
}

// ChallengeAuthorization implements the same bounded, challenge-led methods for
// the automatic scanner and explicit verifier. Its output is private.
func ChallengeAuthorization(challenge, method, uri, user, pass string) (string, string, bool) {
	return authorization(challenge, method, uri, user, pass)
}
func ChooseChallenge(values []string) string { return chooseChallenge(values) }
