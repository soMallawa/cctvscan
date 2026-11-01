// cctvscan-worker has no listener and never accepts a shell command from a job.
package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/postfix/cctvscan/internal/controlplane/privatefile"
	"github.com/postfix/cctvscan/internal/controlplane/runner"
	"github.com/postfix/cctvscan/internal/controlplane/secrets"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type runtimeConfig struct {
	dsn, binary, root, libraryPath string
	key                            []byte
}

func loadConfig(get func(string) string) (runtimeConfig, error) {
	var c runtimeConfig
	var e error
	c.key, e = privatefile.Read(get("CCTVSCAN_SECRET_KEY_FILE"), 32)
	if e != nil || len(c.key) != 32 {
		return c, errors.New("secret key requires exactly 32 raw bytes in an owner-only file")
	}
	c.dsn = get("CCTVSCAN_DATABASE_URL")
	path := get("CCTVSCAN_DATABASE_URL_FILE")
	if path == "" {
		path = get("DATABASE_URL_FILE")
	}
	if path != "" {
		if c.dsn != "" {
			return c, errors.New("choose database environment or file, not both")
		}
		b, e := privatefile.Read(path, 16384)
		if e != nil {
			return c, errors.New("database URL file unavailable or insecure")
		}
		c.dsn = strings.TrimSpace(string(b))
	}
	if c.dsn == "" {
		return c, errors.New("database configuration required")
	}
	c.binary = get("CCTVSCAN_SCANNER_BIN")
	if c.binary == "" {
		c.binary = "/usr/local/bin/cctvscan"
	}
	if !filepath.IsAbs(c.binary) {
		return c, errors.New("scanner binary must be an absolute path")
	}
	st, e := os.Stat(c.binary)
	if e != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0111 == 0 || st.Mode().Perm()&0022 != 0 {
		return c, errors.New("scanner binary must be a regular, non-writable executable")
	}
	c.root = get("CCTVSCAN_WORKER_ROOT")
	if c.root == "" {
		c.root = "/var/lib/cctvscan/jobs"
	}
	c.root = filepath.Clean(c.root)
	if !filepath.IsAbs(c.root) || c.root == "/" {
		return c, errors.New("worker root must be a dedicated absolute directory")
	}
	c.libraryPath = get("CCTVSCAN_SCANNER_LIBRARY_PATH")
	return c, nil
}
func run(ctx context.Context) error {
	c, e := loadConfig(os.Getenv)
	if e != nil {
		return e
	}
	box, e := secrets.New(c.key)
	clear(c.key)
	if e != nil {
		return errors.New("secret initialization unavailable")
	}
	if e = os.MkdirAll(c.root, 0700); e != nil {
		return errors.New("worker directory unavailable")
	}
	st, e := os.Lstat(c.root)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("worker directory must be private and not a symlink")
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	db, e := store.New(startup, c.dsn)
	c.dsn = ""
	if e != nil {
		cancel()
		return errors.New("worker database unavailable")
	}
	defer db.Close()
	_, e = db.GetSettings(startup)
	cancel()
	if e != nil {
		return errors.New("initialize the database with the API migration before starting the worker")
	}
	w := &runner.Worker{Store: db, Secrets: box, Binary: c.binary, Root: c.root, LibraryPath: c.libraryPath}
	return w.Run(ctx)
}
func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		fmt.Println("cctvscan-worker: private River queue consumer\nRequired: CCTVSCAN_SECRET_KEY_FILE, CCTVSCAN_DATABASE_URL_FILE\nOptional: CCTVSCAN_SCANNER_BIN, CCTVSCAN_WORKER_ROOT, CCTVSCAN_SCANNER_LIBRARY_PATH\nNo listener; scanner subprocesses receive an explicit environment and private workspace.")
		return
	}
	if len(os.Args) > 1 {
		fmt.Fprintln(os.Stderr, "worker accepts environment configuration only")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if e := run(ctx); e != nil {
		fmt.Fprintln(os.Stderr, e.Error())
		os.Exit(1)
	}
}
