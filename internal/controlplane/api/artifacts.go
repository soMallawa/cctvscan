package api

import (
	"crypto/sha256"
	"encoding/hex"
	"github.com/google/uuid"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func (s *Server) listArtifacts(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := page(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if _, e := s.options.Store.GetJob(r.Context(), id); e != nil {
		handleError(w, e)
		return
	}
	items, n, e := s.options.Store.ArtifactPage(r.Context(), id, limit, offset)
	if e != nil {
		handleError(w, e)
		return
	}
	write(w, 200, map[string]any{"items": items, "total": n})
}
func (s *Server) serveArtifact(w http.ResponseWriter, r *http.Request) {
	if s.options.ArtifactDir == "" {
		fail(w, 404, "stored artifact unavailable", "")
		return
	}
	a, e := s.options.Store.Artifact(r.Context(), r.PathValue("id"))
	if e != nil {
		handleError(w, e)
		return
	}
	if _, e = uuid.Parse(a.JobID); e != nil {
		fail(w, 404, "stored artifact unavailable", "")
		return
	}
	if _, e = uuid.Parse(a.AttemptID); e != nil {
		fail(w, 404, "stored artifact unavailable", "")
		return
	}
	prefix := a.JobID + "/" + a.AttemptID + "/"
	relative := filepath.ToSlash(a.RelativePath)
	if !strings.HasPrefix(relative, prefix) || filepath.ToSlash(filepath.Clean(relative)) != relative || a.Size < 1 || a.Size > 5<<20 || len(a.Digest) != 64 || (a.MediaType != "image/jpeg" && a.MediaType != "image/png") {
		fail(w, 404, "stored artifact unavailable", "")
		return
	}
	root, e := os.OpenRoot(s.options.ArtifactDir)
	if e != nil {
		fail(w, 503, "artifact storage unavailable", "")
		return
	}
	defer root.Close()
	// No path component may be a symlink. OpenRoot additionally rejects escapes
	// if an on-disk component changes concurrently with validation.
	parts := strings.Split(relative, "/")
	for i := range parts {
		info, e := root.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			fail(w, 404, "stored artifact unavailable", "")
			return
		}
	}
	file, e := root.Open(relative)
	if e != nil {
		fail(w, 404, "stored artifact unavailable", "")
		return
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != a.Size {
		fail(w, 409, "artifact integrity check failed", "")
		return
	}
	h := sha256.New()
	if _, e = io.Copy(h, io.LimitReader(file, (5<<20)+1)); e != nil || hex.EncodeToString(h.Sum(nil)) != a.Digest {
		fail(w, 409, "artifact integrity check failed", "")
		return
	}
	if _, e = file.Seek(0, io.SeekStart); e != nil {
		fail(w, 503, "artifact storage unavailable", "")
		return
	}
	cfg, format, e := image.DecodeConfig(file)
	mime := "image/" + format
	if e != nil || mime != a.MediaType || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 50000000 {
		fail(w, 409, "artifact image validation failed", "")
		return
	}
	if _, e = file.Seek(0, io.SeekStart); e != nil {
		fail(w, 503, "artifact storage unavailable", "")
		return
	}
	w.Header().Set("Content-Type", a.MediaType)
	w.Header().Set("Content-Security-Policy", "sandbox; default-src 'none'")
	w.Header().Set("X-Frame-Options", "DENY")
	name := "evidence.png"
	if format == "jpeg" {
		name = "evidence.jpg"
	}
	w.Header().Set("Content-Disposition", `inline; filename="`+name+`"`)
	http.ServeContent(w, r, name, info.ModTime(), file)
}
