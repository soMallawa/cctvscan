package processor

import (
	"context"
	"encoding/base64"
	"github.com/postfix/cctvscan/internal/catalogue"
	"github.com/postfix/cctvscan/internal/connection"
	"github.com/postfix/cctvscan/internal/credbrute"
	"github.com/postfix/cctvscan/internal/httptransport"
	"github.com/postfix/cctvscan/internal/streams"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type AutomaticCheck struct {
	ProfileID        string             `json:"profile_id"`
	CredentialVendor string             `json:"credential_vendor"`
	AttemptedPairs   int                `json:"attempted_pairs"`
	Outcome          connection.Outcome `json:"outcome"`
}
type automaticCandidate struct {
	id, scheme, path string
	port             int
}

func cameraEligible(r *HostResult) bool {
	return r.CameraDetection.IsCamera || r.CameraDetection.State == "possible" || r.RTSPInfo.Any || (r.Catalogue != nil && r.Catalogue.State == "model_candidate")
}

// automaticVerify follows the existing scan's target/ports, credential opt-in,
// capture policy, deadline and HTTP route. It does not widen network scope.
func (p *OptimizedProcessor) automaticVerify(ctx context.Context, r *HostResult) bool {
	if !cameraEligible(r) || ctx.Err() != nil {
		return false
	}
	candidates := []automaticCandidate{}
	seen := map[string]bool{}
	add := func(c automaticCandidate) {
		key := c.scheme + ":" + strconv.Itoa(c.port) + c.path
		if !seen[key] && len(candidates) < 6 {
			seen[key] = true
			candidates = append(candidates, c)
		}
	}
	allowed := func(port int) bool {
		for _, n := range r.Ports {
			if port == n {
				return true
			}
		}
		return false
	}
	for _, s := range r.StreamResults {
		u, e := url.Parse(s.URL)
		if e != nil || u.Hostname() != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		port := 80
		if u.Scheme == "https" {
			port = 443
		}
		if u.Port() != "" {
			port, _ = strconv.Atoi(u.Port())
		}
		if allowed(port) {
			add(automaticCandidate{"observed-media", u.Scheme, u.RequestURI(), port})
		}
	}
	for _, port := range r.HTTPPorts {
		scheme := "http"
		if port == 443 || port == 8443 {
			scheme = "https"
		}
		switch strings.ToLower(r.Brand) {
		case "hikvision":
			add(automaticCandidate{"builtin-hikvision-snapshot", scheme, "/ISAPI/Streaming/channels/101/picture", port})
		case "dahua", "cp plus":
			add(automaticCandidate{"builtin-dahua-snapshot", scheme, "/cgi-bin/snapshot.cgi", port})
		case "axis":
			add(automaticCandidate{"builtin-axis-snapshot", scheme, "/axis-cgi/jpg/image.cgi", port})
		}
		add(automaticCandidate{"builtin-snapshot", scheme, "/snapshot.jpg", port})
		add(automaticCandidate{"builtin-mjpeg", scheme, "/video.mjpg", port})
	}
	if c, e := catalogue.Default(); e == nil {
		for _, port := range r.RTSPPorts {
			profiles := c.ProfilesFor(r.Brand)
			if r.Catalogue != nil && r.Catalogue.Brand != "" {
				profiles = c.ProfilesFor(r.Catalogue.Brand)
			}
			for _, profile := range profiles {
				if profile.Scheme == "rtsp" {
					add(automaticCandidate{profile.ID, "rtsp", profile.Path, port})
				}
			}
			if len(r.RTSPPorts) > 0 && len(candidates) == 0 {
				add(automaticCandidate{"builtin-rtsp-live", "rtsp", "/live", r.RTSPPorts[0]})
			}
		}
	}
	pairs := []string{}
	if p.Options.CheckCredentials {
		pairs, _ = credbrute.LoadVendorCredentials(p.credsFile, r.Brand)
	}
	for _, c := range candidates {
		if ctx.Err() != nil {
			break
		}
		input := connection.Input{Host: r.Host, Port: c.port, Scheme: c.scheme, Path: c.path, Proxy: httptransport.ConfiguredProxy(), AllowDirectRTSP: c.scheme == "rtsp", Parameters: map[string]string{"channel": "1", "NN": "1", "N": "1", "codec": "h264"}}
		if r.Credentials != "" {
			input.Username, input.Password, _ = strings.Cut(r.Credentials, ":")
		}
		check := AutomaticCheck{ProfileID: c.id, CredentialVendor: r.Brand}
		check.Outcome = connection.Verify(ctx, input)
		if check.Outcome.Authentication == "required" && r.Credentials == "" {
			for _, pair := range pairs {
				if ctx.Err() != nil {
					break
				}
				input.Username, input.Password, _ = strings.Cut(pair, ":")
				check.AttemptedPairs++
				check.Outcome = connection.Verify(ctx, input)
				if check.Outcome.Authentication == "accepted" {
					r.Credentials = pair
					r.CredentialState = "found"
					break
				}
				if check.Outcome.Authentication != "rejected" {
					break
				}
			}
		}
		if check.Outcome.Authentication == "accepted" {
			r.CredentialState = "found"
		}
		verified := check.Outcome.State == "frame_verified"
		if verified && p.Options.Capture && check.Outcome.FrameBase64 != "" {
			dir := filepath.Join(p.outputDir, "snapshots")
			if e := os.MkdirAll(dir, 0700); e == nil {
				if f, e := os.CreateTemp(dir, "verified-*.jpg"); e == nil {
					data, _ := base64.StdEncoding.DecodeString(check.Outcome.FrameBase64)
					_, e = f.Write(data)
					closeErr := f.Close()
					if e == nil {
						e = closeErr
					}
					if e == nil {
						kind := streams.StreamSnapshot
						if c.scheme == "rtsp" {
							kind = streams.StreamRTSP
						}
						r.StreamResults = append(r.StreamResults, streams.StreamResult{URL: c.scheme + "://" + net.JoinHostPort(r.Host, strconv.Itoa(c.port)), Type: kind, State: "frame_decoded", Verified: true, Evidence: "Decoded image frame", SavedPath: f.Name()})
					} else {
						os.Remove(f.Name())
						r.fail("capture", e)
					}
				}
			}
		}
		if !p.Options.Capture {
			check.Outcome.FrameBase64 = ""
		}
		r.AutomaticVerification = append(r.AutomaticVerification, check)
		if verified {
			return true
		}
	}
	return false
}
