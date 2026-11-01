package credbrute

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAutomaticCredentialSelectionUsesVendorSection(t *testing.T) {
	var unrelated atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if ok && u == "other-vendor" {
			unrelated.Add(1)
		}
		if ok && u == "fixture-user" && p == "fixture-pass" {
			w.Write([]byte("fixture accepted"))
			return
		}
		w.Header().Set("WWW-Authenticate", `Basic realm="owned fixture"`)
		w.WriteHeader(401)
	}))
	defer s.Close()
	file := filepath.Join(t.TempDir(), "creds.txt")
	os.WriteFile(file, []byte("# Dahua\nother-vendor:fixture-other\n# Hikvision\nfixture-user:fixture-pass\n# Generic\nfixture-generic:fixture-value\n"), 0600)
	result, e := CheckCredentialsForVendor(context.Background(), []string{s.URL}, file, "Hikvision", time.Second)
	if e != nil || result.State != "found" || unrelated.Load() != 0 {
		t.Fatal("automatic vendor selection failed or attempted another vendor")
	}
}

func TestAutomaticDigestCredentialsUseChallengeNotBasic(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a := r.Header.Get("Authorization")
		fields := map[string]string{}
		for _, m := range regexp.MustCompile(`([a-z]+)=(?:"([^"]*)"|([^, ]+))`).FindAllStringSubmatch(a, -1) {
			fields[m[1]] = m[2] + m[3]
		}
		h := func(v string) string { return fmt.Sprintf("%x", md5.Sum([]byte(v))) }
		expected := h(h("fixture-user:fixture:fixture-pass") + ":fixture-nonce:" + fields["nc"] + ":" + fields["cnonce"] + ":auth:" + h("GET:/"))
		if strings.HasPrefix(a, "Digest ") && fields["response"] == expected && fields["username"] == "fixture-user" {
			w.Write([]byte("fixture accepted"))
			return
		}
		w.Header().Set("WWW-Authenticate", `Digest realm="fixture", nonce="fixture-nonce", algorithm=MD5, qop="auth"`)
		w.WriteHeader(401)
	}))
	defer s.Close()
	file := filepath.Join(t.TempDir(), "creds.txt")
	os.WriteFile(file, []byte("# Hikvision\nfixture-user:fixture-pass\n"), 0600)
	result, e := CheckCredentialsForVendor(context.Background(), []string{s.URL}, file, "Hikvision", time.Second)
	if e != nil || result.State != "found" {
		t.Fatal("automatic digest credentials were not actually accepted")
	}
}
