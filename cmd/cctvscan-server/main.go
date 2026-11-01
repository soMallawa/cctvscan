// cctvscan-server serves only the private control plane. It never starts workers.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/postfix/cctvscan/internal/controlplane/api"
	"github.com/postfix/cctvscan/internal/controlplane/auth"
	"github.com/postfix/cctvscan/internal/controlplane/privatefile"
	"github.com/postfix/cctvscan/internal/controlplane/secrets"
	"github.com/postfix/cctvscan/internal/controlplane/store"
)

type runtimeConfig struct {
	listen, dsn string
	key         []byte
	options     api.Options
}

// privateFile accepts only bounded regular non-symlink files with no group/other
// permissions. The encryption key format is exactly 32 raw bytes (no newline).
func privateFile(path string, max int64) ([]byte, error) { return privatefile.Read(path, max) }

func loadConfig(get func(string) string) (runtimeConfig, error) {
	var c runtimeConfig
	c.listen = get("CCTVSCAN_LISTEN_ADDR")
	if c.listen == "" {
		c.listen = "127.0.0.1:14904"
	}
	mode := get("CCTVSCAN_AUTH_MODE")
	if mode == "" {
		mode = "access"
	}
	host, port, err := net.SplitHostPort(c.listen)
	if err != nil || port == "" {
		return c, errors.New("invalid listen address")
	}
	// Local mode is intentionally narrower than the reusable auth boundary: no
	// RFC1918 trust exception, hostname resolution or wildcard bind is allowed.
	if mode == "local" {
		ip, err := netip.ParseAddr(host)
		if err != nil || !ip.IsLoopback() || get("CCTVSCAN_LOCAL_TRUST_CIDR") != "" {
			return c, errors.New("local authentication requires a literal loopback listen address and no trust CIDR")
		}
	}
	c.options = api.Options{Auth: auth.Config{Mode: mode, Issuer: get("CCTVSCAN_ACCESS_ISSUER"), Audience: get("CCTVSCAN_ACCESS_AUDIENCE")}, PublicOrigin: get("CCTVSCAN_PUBLIC_ORIGIN"), ArtifactDir: get("CCTVSCAN_ARTIFACT_DIR"), ProxyTestURL: get("CCTVSCAN_PROXY_TEST_URL"), Version: "development"}
	// Validate authentication, origin and diagnostic URL before touching the DB.
	if _, err := api.New(c.options); err != nil {
		return c, errors.New("invalid API authentication, public origin or proxy test configuration")
	}
	c.key, err = privateFile(get("CCTVSCAN_SECRET_KEY_FILE"), 32)
	if err != nil || len(c.key) != 32 {
		return c, errors.New("secret key file requires exactly 32 raw bytes with owner-only permissions")
	}
	path := get("CCTVSCAN_DATABASE_URL_FILE")
	if path == "" {
		path = get("DATABASE_URL_FILE")
	}
	c.dsn = get("CCTVSCAN_DATABASE_URL")
	if path != "" {
		if c.dsn != "" {
			return c, errors.New("choose database URL environment or file, not both")
		}
		b, e := privateFile(path, 16384)
		if e != nil {
			return c, errors.New("database URL file unavailable or insecure")
		}
		c.dsn = strings.TrimSpace(string(b))
	}
	if c.dsn == "" {
		return c, errors.New("database URL configuration required")
	}
	return c, nil
}

func newHTTPServer(ctx context.Context, addr string, h http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		// SSE owns per-write deadlines; ordinary responses have a bounded write.
		isEvents := r.Method == http.MethodGet && (r.URL.Path == "/api/events" || (strings.HasPrefix(r.URL.Path, "/api/jobs/") && strings.HasSuffix(r.URL.Path, "/events")))
		if !isEvents {
			control := http.NewResponseController(w)
			_ = control.SetWriteDeadline(time.Now().Add(30 * time.Second))
			defer control.SetWriteDeadline(time.Time{})
		}
		h.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 0, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 32 << 10, BaseContext: func(net.Listener) context.Context { return ctx }, ErrorLog: log.New(io.Discard, "", 0)}
}

func run(ctx context.Context) error {
	c, err := loadConfig(os.Getenv)
	if err != nil {
		return err
	}
	box, err := secrets.New(c.key)
	clear(c.key)
	if err != nil {
		return errors.New("secret initialization failed")
	}
	startup, cancel := context.WithTimeout(ctx, 60*time.Second)
	db, err := store.New(startup, c.dsn)
	c.dsn = ""
	if err != nil {
		cancel()
		return errors.New("database startup unavailable")
	}
	defer db.Close()
	err = db.Migrate(startup)
	cancel()
	if err != nil {
		return errors.New("database migration unavailable")
	}
	c.options.Store = db
	c.options.Secrets = box
	app, err := api.New(c.options)
	if err != nil {
		return errors.New("API initialization failed")
	}
	requests, cancelRequests := context.WithCancel(ctx)
	defer cancelRequests()
	srv := newHTTPServer(requests, c.listen, app.Handler())
	listener, err := net.Listen("tcp", c.listen)
	if err != nil {
		return errors.New("HTTP listener unavailable")
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(listener) }()
	fmt.Fprintln(os.Stderr, "cctvscan-server ready (API only; no worker)")
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return errors.New("HTTP server unavailable")
		}
		return nil
	case <-ctx.Done():
		cancelRequests() // Shutdown alone does not cancel active SSE handlers.
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := srv.Shutdown(shutdown); err != nil {
			_ = srv.Close()
			return errors.New("HTTP shutdown deadline exceeded")
		}
		<-done
		return nil
	}
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
