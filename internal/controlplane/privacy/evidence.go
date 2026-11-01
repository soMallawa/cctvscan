package privacy

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var embeddedURL = regexp.MustCompile(`(?i)(?:https?|rtsp|rtmp|mms)://[^\s"<>]+`)

// Keys are matched as fields, not substrings: credential outcomes and resource
// references are public, while credentials and server-owned file locations are not.
func redactionKey(key string) string {
	return strings.ToLower(strings.NewReplacer("_", "", "-", "").Replace(key))
}
func sensitiveField(key, parent string) bool {
	switch key {
	case "credentialstate", "credentialsetid", "proxyversionid", "proxyid", "proxystatus", "artifactid", "artifactids":
		return false
	case "credentials", "credential", "credentialvalue", "credentialvalues", "username", "user", "password", "passwd", "pass", "secret", "secrets", "token", "accesstoken", "refreshtoken", "idtoken", "apikey", "authorization", "auth", "proxyauthorization", "cookie", "cookies", "setcookie", "headers", "requestheaders", "responseheaders", "body", "requestbody", "responsebody", "bodysnippet", "raw", "rawdata", "rawpayload", "payload", "proxy", "proxies", "proxyurl", "proxyplaintext", "savedpath", "serverpath", "serverfilepath", "filesystempath", "filepath", "artifactpath", "outputpath", "outputdir":
		return true
	case "path", "paths":
		// StreamResult.Path is a remote candidate, SavedPath is a server file.
		return parent != "streamresults"
	}
	// Composed sensitive fields (db_password, bearerToken, request_payload)
	// remain sensitive. Match terminal field semantics, never arbitrary embedded
	// words: credential_state, proxy_test_status and opaque *_id survive.
	for _, suffix := range []string{"password", "passwd", "username", "credential", "credentials", "secret", "token", "authorization", "headers", "cookie", "cookies", "body", "bodysnippet", "payload", "path", "paths"} {
		if strings.HasSuffix(key, suffix) && key != "mjpegpaths" {
			return true
		}
	}
	return false
}

func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "[redacted URL]"
	}
	switch strings.ToLower(u.Scheme) {
	case "", "http", "https", "rtsp", "rtmp", "mms":
	default:
		return "[redacted URL]"
	}
	if u.Opaque != "" {
		return "[redacted URL]"
	}
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""
	u.RawFragment = ""
	return u.String()
}

func Sanitize(raw json.RawMessage) json.RawMessage {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return json.RawMessage(`null`)
	}
	var clean func(any, string, string) any
	clean = func(v any, key, parent string) any {
		switch x := v.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, value := range x {
				field := redactionKey(k)
				if key == "rootevidence" && !rootEvidenceField(field) {
					continue
				}
				if field == "rootevidence" && !validRootEvidence(value) {
					continue
				}
				if sensitiveField(field, key) {
					continue
				}
				out[k] = clean(value, field, key)
			}
			return out
		case []any:
			out := make([]any, len(x))
			for i, value := range x {
				out[i] = clean(value, key, parent)
			}
			return out
		case string:
			// URL/path collections have remote-URL semantics, never filesystem semantics.
			switch key {
			case "url", "streamurl", "mjpegpaths", "loginpages", "endpoints":
				return sanitizeURL(x)
			case "path", "paths":
				if parent == "streamresults" {
					return sanitizeURL(x)
				}
			}
			return embeddedURL.ReplaceAllStringFunc(x, sanitizeURL)
		default:
			return v
		}
	}
	out, err := json.Marshal(clean(v, "", ""))
	if err != nil {
		return json.RawMessage(`null`)
	}
	return out
}
