package streams

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/postfix/cctvscan/internal/httptransport"
	"github.com/postfix/cctvscan/internal/probe"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// StreamType represents different types of camera streams
type StreamType string

const (
	StreamMJPEG    StreamType = "MJPEG"
	StreamSnapshot StreamType = "Snapshot"
	StreamRTSP     StreamType = "RTSP"
	StreamRTMP     StreamType = "RTMP"
	StreamMMS      StreamType = "MMS"
	StreamHLS      StreamType = "HLS"
	StreamGeneric  StreamType = "Generic"
)

// StreamResult contains information about detected streams
type StreamResult struct {
	SavedPath   string
	State       string
	Evidence    string
	Verified    bool
	URL         string
	Type        StreamType
	ContentType string
	Status      int
	Size        int64
	Path        string
}

// Comprehensive stream paths for different camera brands and protocols
var StreamPaths = map[string][]string{
	"rtsp": {
		// Generic RTSP paths
		"/live.sdp",
		"/h264.sdp",
		"/stream1",
		"/stream2",
		"/main",
		"/sub",
		"/video",
		"/cam/realmonitor",
		"/Streaming/Channels/1",
		"/Streaming/Channels/101",
		// Brand-specific RTSP paths
		"/onvif/streaming/channels/1", // ONVIF
		"/live/0/onvif.sdp",           // ONVIF
		"/live/0/h264.sdp",            // Generic
		"/live/0/mpeg4.sdp",           // Generic
		"/live/0/audio.sdp",           // Generic
		"/live/1/onvif.sdp",           // ONVIF
		"/live/1/h264.sdp",            // Generic
		"/live/1/mpeg4.sdp",           // Generic
		"/live/1/audio.sdp",           // Generic
	},
	"rtmp": {
		"/live",
		"/stream",
		"/hls",
		"/flv",
		"/rtmp",
		"/live/stream",
		"/live/stream1",
		"/live/stream2",
		"/live/main",
		"/live/sub",
		"/live/video",
		"/live/audio",
		"/live/av",
		"/live/rtmp",
		"/live/rtmps",
	},
	"http": {
		// Generic HTTP stream paths
		"/video",
		"/stream",
		"/mjpg/video.mjpg",
		"/cgi-bin/mjpg/video.cgi",
		"/axis-cgi/mjpg/video.cgi",
		"/cgi-bin/viewer/video.jpg",
		"/snapshot.jpg",
		"/img/snapshot.cgi",
		// Brand-specific HTTP paths
		"/onvif/device_service",     // ONVIF
		"/onvif/streaming",          // ONVIF
		"/axis-cgi/com/ptz.cgi",     // Axis
		"/axis-cgi/param.cgi",       // Axis
		"/cgi-bin/snapshot.cgi",     // Generic
		"/cgi-bin/hi3510/snap.cgi",  // Hikvision
		"/cgi-bin/viewer/video.jpg", // Generic
		"/img/snapshot.cgi",         // Generic
		"/snapshot.jpg",             // Generic
		"/video/mjpg.cgi",           // Generic
		"/video.cgi",                // Generic
		"/videostream.cgi",          // Generic
		"/mjpg/video.mjpg",          // Generic
		"/mjpg.cgi",                 // Generic
		"/stream.cgi",               // Generic
		"/live.cgi",                 // Generic
		// Additional API endpoints
		"/api/video", // API endpoints
		"/api/stream",
		"/api/live",
		"/api/video/live",
		"/api/stream/live",
		"/api/camera/live",
		"/api/camera/stream",
		"/api/camera/video",
		"/api/camera/snapshot",
		"/api/camera/image",
		"/api/camera/feed",
		"/api/camera/feed/live",
		"/api/camera/feed/stream",
		"/api/camera/feed/video",
		// CP Plus specific paths
		"/cgi-bin/snapshot.cgi",
		"/cgi-bin/video.cgi",
		"/cgi-bin/stream.cgi",
		"/cgi-bin/live.cgi",
	},
}

// Streaming ports by protocol
var StreamingPorts = map[string][]int{
	"rtsp":  {554, 8554, 10554},
	"rtmp":  {1935, 1936},
	"http":  {80, 8080, 8000, 8001},
	"https": {443, 8443, 8444},
	"mms":   {1755},
	"onvif": {3702, 80, 443},
	"vlc":   {8080, 8090},
}

// Camera content types for stream detection
var StreamContentTypes = []string{
	"image/jpeg", "image/jpg", "image/pjpeg", "image/png", "image/gif",
	"multipart/x-mixed-replace",
	"video/mpeg", "video/mp4", "video/h264", "video/h265", "video/hevc",
	"video/3gpp", "video/webm", "video/ogg", "video/MP2T",
	"application/mp4", "application/sdp", "application/vnd.apple.mpegurl",
	"application/x-mpegURL", "application/octet-stream", "application/x-rtsp",
	"text/html", "application/json", "application/xml", "text/xml",
}

// Video file extensions
var VideoExtensions = []string{
	".mp4", ".m3u8", ".ts", ".flv", ".webm", ".avi", ".mov",
}

// Streaming protocol prefixes
var StreamingProtocols = []string{
	"rtsp://", "rtmp://", "mms://", "rtp://",
}

// DetectStreams returns media-evidenced HTTP endpoints and explicitly unverified
// protocol candidates. It does not acquire snapshots implicitly.
func DetectStreams(ctx context.Context, host string, ports []int, outDir string) []StreamResult {
	r, _ := DetectStreamsDetailed(ctx, host, ports)
	return r
}
func DetectStreamsDetailed(ctx context.Context, host string, ports []int) ([]StreamResult, error) {
	client := &http.Client{Timeout: 3 * time.Second, Transport: httptransport.NewTransport(&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: (&net.Dialer{Timeout: 1500 * time.Millisecond}).DialContext})}
	var results []StreamResult
	var errs []error
	for _, port := range ports {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		switch {
		case probe.IsRTSPPort(port):
			results = append(results, checkRTSPStreams(ctx, host, port)...)
		case isRTMPPort(port):
			results = append(results, checkRTMPStreams(ctx, host, port)...)
		case isMMSPort(port):
			results = append(results, checkMMSStreams(ctx, host, port)...)
		case port == 3702 || port == 37777:
			continue
		default:
			scheme := "http"
			if port == 443 || port == 8443 || port == 8444 {
				scheme = "https"
			}
			base := scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port))
			seen := map[string]bool{}
			for _, path := range StreamPaths["http"] {
				if seen[path] {
					continue
				}
				seen[path] = true
				r, err := checkStreamURLDetailed(ctx, client, base+path, port, path)
				if err != nil {
					errs = append(errs, err)
					break
				}
				if r != nil {
					results = append(results, *r)
				}
			}
		}
	}
	return results, errors.Join(errs...)
}

func protocolCandidates(ctx context.Context, host string, port int, scheme string, kind StreamType, paths []string) []StreamResult {
	if ctx.Err() != nil {
		return nil
	}
	out := make([]StreamResult, 0, len(paths))
	for _, path := range paths {
		out = append(out, StreamResult{URL: scheme + "://" + net.JoinHostPort(host, strconv.Itoa(port)) + path, Type: kind, Path: path, State: "candidate"})
	}
	return out
}
func checkRTSPStreams(ctx context.Context, host string, port int) []StreamResult {
	return protocolCandidates(ctx, host, port, "rtsp", StreamRTSP, StreamPaths["rtsp"])
}
func checkRTMPStreams(ctx context.Context, host string, port int) []StreamResult {
	return protocolCandidates(ctx, host, port, "rtmp", StreamRTMP, StreamPaths["rtmp"])
}
func checkMMSStreams(ctx context.Context, host string, port int) []StreamResult {
	return protocolCandidates(ctx, host, port, "mms", StreamMMS, []string{"/"})
}
func checkStreamURL(ctx context.Context, client *http.Client, url string, port int, path string) *StreamResult {
	r, _ := checkStreamURLDetailed(ctx, client, url, port, path)
	return r
}
func checkStreamURLDetailed(ctx context.Context, client *http.Client, url string, port int, path string) (*StreamResult, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httptransport.Do(client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return nil, err
	}
	ct := resp.Header.Get("Content-Type")
	kind, evidence := mediaEvidence(data, ct)
	if evidence == "" {
		return nil, nil
	}
	return &StreamResult{URL: url, Type: kind, ContentType: ct, Status: resp.StatusCode, Path: path, State: "media_header", Evidence: evidence, Verified: true}, nil
}
func mediaEvidence(data []byte, ct string) (StreamType, string) {
	if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err == nil && (format == "jpeg" || format == "png") {
		return StreamSnapshot, "Decoded image header"
	}
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("#EXTM3U")) && bytes.Contains(data, []byte("#EXT-X-")) {
		return StreamHLS, "HLS playlist header; playback not verified"
	}
	if len(data) > 12 && string(data[4:8]) == "ftyp" {
		return StreamGeneric, "MP4 container header; playback not verified"
	}
	mediaType, params, err := mime.ParseMediaType(ct)
	if err == nil && mediaType == "multipart/x-mixed-replace" {
		if params["boundary"] != "" {
			part, err := multipart.NewReader(bytes.NewReader(data), params["boundary"]).NextPart()
			if err == nil {
				defer part.Close()
				if _, format, e := image.DecodeConfig(part); e == nil && format == "jpeg" {
					return StreamMJPEG, "Multipart JPEG framing/header; playback not verified"
				}
			}
		}
	}
	return "", ""
}

// Helper functions

func isRTMPPort(port int) bool {
	for _, p := range StreamingPorts["rtmp"] {
		if port == p {
			return true
		}
	}
	return false
}

func isMMSPort(port int) bool {
	for _, p := range StreamingPorts["mms"] {
		if port == p {
			return true
		}
	}
	return false
}

// SaveSnapshot validates and atomically saves one bounded image. It is only
// called by explicit capture operations; ordinary discovery never writes media.
func SaveSnapshot(ctx context.Context, client *http.Client, result StreamResult, outDir string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", result.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := httptransport.Do(client, req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("snapshot HTTP status %d", resp.StatusCode)
	}
	reader := io.Reader(resp.Body)
	ct, params, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if ct == "multipart/x-mixed-replace" {
		if params["boundary"] == "" {
			return "", errors.New("missing multipart boundary")
		}
		part, e := multipart.NewReader(reader, params["boundary"]).NextPart()
		if e != nil {
			return "", e
		}
		defer part.Close()
		reader = part
	}
	data, err := io.ReadAll(io.LimitReader(reader, 256*1024+1))
	if err != nil {
		return "", err
	}
	if len(data) > 256*1024 {
		return "", errors.New("snapshot exceeds 256 KiB limit")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("snapshot is not a valid image")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 4096 || cfg.Height > 4096 || int64(cfg.Width)*int64(cfg.Height) > 8*1024*1024 {
		return "", errors.New("snapshot dimensions exceed safety budget")
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", errors.New("incomplete snapshot image")
	}
	if err = os.MkdirAll(outDir, 0700); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(result.URL))
	name := filepath.Join(outDir, fmt.Sprintf("snapshot_%x.jpg", sum[:16]))
	f, err := os.CreateTemp(outDir, ".snapshot-*")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = jpeg.Encode(f, img, nil); err != nil {
		f.Close()
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = os.Rename(tmp, name); err != nil {
		return "", err
	}
	return name, nil
}
func saveStreamSnapshot(ctx context.Context, client *http.Client, result StreamResult, outDir string) error {
	_, err := SaveSnapshot(ctx, client, result, outDir)
	return err
}
