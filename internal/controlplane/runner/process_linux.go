//go:build linux

// Package runner launches one isolated scanner child per immutable job.
package runner

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Process struct {
	OnEvent                                  func(context.Context, json.RawMessage) error
	Binary, Directory, ProxyURL, LibraryPath string
	Credentials                              []byte
	Policy                                   *config.Policy
}

func (p Process) Run(ctx context.Context, c config.ScanConfig) error {
	policy := config.DefaultPolicy()
	if p.Policy != nil {
		policy = *p.Policy
	}
	preview, e := config.Validate(ctx, c, policy)
	if e != nil {
		return errors.New("scanner configuration is invalid")
	}
	c = preview.Config
	if !filepath.IsAbs(p.Binary) || !filepath.IsAbs(p.Directory) {
		return errors.New("scanner paths must be absolute")
	}
	if c.ProxyVersionID != "" {
		u, e := url.Parse(p.ProxyURL)
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("selected proxy is unavailable")
		}
	}
	if c.CheckCredentials && len(p.Credentials) == 0 {
		return errors.New("selected credential set is unavailable")
	}
	if e = os.MkdirAll(p.Directory, 0700); e != nil {
		return errors.New("scanner workspace unavailable")
	}
	if e = os.Chmod(p.Directory, 0700); e != nil {
		return errors.New("scanner workspace permissions unavailable")
	}
	targets := filepath.Join(p.Directory, "targets.txt")
	if e = os.WriteFile(targets, []byte(strings.Join(c.Targets, "\n")+"\n"), 0600); e != nil {
		return errors.New("scanner scope unavailable")
	}
	defer os.Remove(targets)
	number := func(v int) string { return strconv.Itoa(v) }
	duration := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) + "s" }
	args := []string{"--output", p.Directory, "--max-targets", strconv.FormatUint(c.MaxTargets, 10), "--rate", number(c.Rate), "--retry", number(c.Retry), "--wait", number(c.Wait), "--threads", number(c.Threads), "--timeout", duration(c.TimeoutSeconds), "--host-timeout", duration(c.HostTimeoutSeconds), "--host-workers", number(c.HostWorkers), "--http-requests", number(c.HTTPRequests), "--http-per-host", number(c.HTTPPerHost), "--http-rps", strconv.FormatFloat(c.HTTPRPS, 'f', -1, 64)}
	if c.ScanMode == "connect" {
		args = append(args, "--unprivileged")
	}
	if c.ScanMode == "syn" {
		args = append(args, "--privileged")
	}
	if c.PortMode == "custom" {
		args = append(args, "--ports", c.Ports)
	}
	if c.PortMode == "comprehensive" {
		args = append(args, "--comprehensive")
	}
	for _, v := range [][2]string{{"--adapter", c.Adapter}, {"--adapter-ip", c.AdapterIP}, {"--router-ip", c.RouterIP}, {"--router-mac", c.RouterMAC}, {"--masscan-route-target", c.RouteTarget}} {
		if v[1] != "" {
			args = append(args, v[0], v[1])
		}
	}
	for _, v := range []struct {
		name    string
		enabled bool
	}{{"--capture", c.Capture}, {"--onvif", c.ONVIF}, {"--debug", c.Debug}} {
		if v.enabled {
			args = append(args, v.name)
		}
	}
	environment := []string{"PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C.UTF-8", "HOME=" + p.Directory, "TMPDIR=" + p.Directory}
	if p.LibraryPath != "" {
		environment = append(environment, "LD_LIBRARY_PATH="+p.LibraryPath)
	}
	if c.ProxyVersionID != "" {
		args = append(args, "--http-proxy-env", "CCTVSCAN_JOB_HTTP_PROXY")
		environment = append(environment, "CCTVSCAN_JOB_HTTP_PROXY="+p.ProxyURL)
	}
	if c.CheckCredentials {
		path := filepath.Join(p.Directory, "credentials.txt")
		if e = os.WriteFile(path, p.Credentials, 0600); e != nil {
			return errors.New("private credential materialization failed")
		}
		defer os.Remove(path)
		args = append(args, "--check-credentials", "--creds", path)
	}
	if p.OnEvent != nil {
		args = append(args, "--event-fd", "3")
	}
	args = append(args, targets)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds*float64(time.Second)))
	defer cancel()
	if e = runCtx.Err(); e != nil {
		return e
	}
	cmd := exec.Command(p.Binary, args...)
	cmd.Dir = p.Directory
	cmd.Env = environment
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	cmd.WaitDelay = 500 * time.Millisecond
	var eventRead, eventWrite *os.File
	var eventDone chan error
	if p.OnEvent != nil {
		eventRead, eventWrite, e = os.Pipe()
		if e != nil {
			return errors.New("private event channel unavailable")
		}
		defer eventRead.Close()
		defer eventWrite.Close()
		cmd.ExtraFiles = []*os.File{eventWrite}
		eventDone = make(chan error, 1)
	}
	if e = cmd.Start(); e != nil {
		return errors.New("scanner process could not start")
	}
	if eventWrite != nil {
		_ = eventWrite.Close()
		go func() {
			e := readObservations(runCtx, eventRead, p.OnEvent)
			eventDone <- e
			if e != nil {
				cancel()
			}
		}()
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case e = <-done:
		// The scanner leader may exit while helper processes remain in its group.
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if eventDone != nil {
			select {
			case observationError := <-eventDone:
				if observationError != nil {
					return errors.New("scanner observation stream failed")
				}
			case <-time.After(3 * time.Second):
				_ = eventRead.Close()
				return errors.New("scanner observation drain timed out")
			}
		}
		if runCtx.Err() != nil {
			return runCtx.Err()
		}
		if e != nil {
			return errors.New("scanner process failed")
		}
		return nil
	case <-runCtx.Done():
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		timer := time.NewTimer(1500 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-done:
		case <-timer.C:
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			<-done
		}
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		return runCtx.Err()
	}
}
