package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func inspectCapture(directory, jobID, attemptID, path string) (store.ArtifactInput, error) {
	var a store.ArtifactInput
	if !filepath.IsAbs(path) {
		path = filepath.Join(directory, path)
	}
	relative, e := filepath.Rel(directory, path)
	if e != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return a, errors.New("capture path outside attempt")
	}
	root, e := os.OpenRoot(directory)
	if e != nil {
		return a, e
	}
	defer root.Close()
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for i := range parts {
		info, e := root.Lstat(strings.Join(parts[:i+1], "/"))
		if e != nil || info.Mode()&os.ModeSymlink != 0 {
			return a, errors.New("capture path is not a regular owned file")
		}
	}
	file, e := root.Open(relative)
	if e != nil {
		return a, e
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > 5<<20 {
		return a, errors.New("capture exceeds storage bounds")
	}
	cfg, format, e := image.DecodeConfig(file)
	if e != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 50000000 {
		return a, errors.New("capture is not a supported bounded image")
	}
	if _, e = file.Seek(0, io.SeekStart); e != nil {
		return a, e
	}
	h := sha256.New()
	if _, e = io.Copy(h, io.LimitReader(file, (5<<20)+1)); e != nil {
		return a, e
	}
	return store.ArtifactInput{RelativePath: filepath.ToSlash(filepath.Join(jobID, attemptID, relative)), MediaType: "image/" + format, Size: info.Size(), Digest: hex.EncodeToString(h.Sum(nil))}, nil
}
