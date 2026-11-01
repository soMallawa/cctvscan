//go:build integration

package connection

import (
	"bufio"
	"context"
	"encoding/json"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestRealRTSPKnownAuthenticationAndDecodedVideoFrame(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixture := exec.CommandContext(ctx, "python3", "../../qa/console/rtsp_fixture.py")
	stdout, e := fixture.StdoutPipe()
	if e != nil {
		t.Fatal(e)
	}
	if e = fixture.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { fixture.Process.Signal(syscall.SIGTERM); fixture.Wait() }()
	scan := bufio.NewScanner(stdout)
	if !scan.Scan() {
		t.Fatal("owned RTSP fixture did not become ready")
	}
	var addr struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if e = json.Unmarshal(scan.Bytes(), &addr); e != nil {
		t.Fatal(e)
	}
	out := Verify(ctx, Input{Host: addr.Host, Port: addr.Port, Scheme: "rtsp", Path: "/live", Username: "fixture-user", Password: "fixture-pass", AllowDirectRTSP: true})
	if out.State != "frame_verified" || out.Authentication != "accepted" || out.Width != 64 || out.Height != 48 || out.FrameBase64 == "" {
		t.Fatalf("owned RTSP journey did not decode: state=%s auth=%s media=%s reason=%s", out.State, out.Authentication, out.Media, out.Reason)
	}
}
