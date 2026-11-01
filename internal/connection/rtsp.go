package connection

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func verifyRTSP(ctx context.Context, in Input, u *url.URL, out Outcome) Outcome {
	out.Route = "direct_rtsp_tcp"
	status, challenge, sdp, e := describe(ctx, u, "")
	if e != nil {
		out.Reason = "RTSP description failed or timed out."
		return out
	}
	out.Authentication = "not_required"
	if status == 401 {
		out.Authentication = "required"
		out.State = "authentication_required"
		if in.Username == "" && in.Password == "" {
			out.Reason = "RTSP requires known credentials."
			return out
		}
		value, method, ok := authorization(challenge, "DESCRIBE", u.String(), in.Username, in.Password)
		out.AuthMethod = method
		if !ok {
			out.State = "unsupported"
			out.Authentication = "unsupported_method"
			out.Reason = "RTSP advertised an unsupported authentication method."
			return out
		}
		status, _, sdp, e = describe(ctx, u, value)
		if e != nil {
			out.State = "inconclusive"
			out.Authentication = "inconclusive"
			out.Reason = "Authenticated RTSP description failed or timed out."
			return out
		}
		if status == 200 {
			out.Authentication = "accepted"
		} else if status == 401 || status == 403 {
			out.Authentication = "rejected"
		} else {
			out.Authentication = "inconclusive"
		}
	}
	out.Status = status
	if out.Authentication == "rejected" {
		out.State = "authentication_rejected"
		out.Reason = "RTSP rejected the supplied credential pair."
		return out
	}
	if status != 200 || !strings.Contains("\n"+sdp, "\nm=video ") {
		out.State = "responded"
		out.Reason = "RTSP responded without a usable video description."
		return out
	}
	out.State = "stream_described"
	out.Media = "sdp_video"
	out.Reason = "RTSP advertised video; a frame has not decoded."
	ffmpeg, e := exec.LookPath("ffmpeg")
	if e != nil {
		out.Reason = "RTSP video described; the packaged frame decoder is unavailable."
		return out
	}
	private := *u
	if in.Username != "" || in.Password != "" {
		private.User = url.UserPassword(in.Username, in.Password)
	}
	file, e := os.CreateTemp("", "cctvscan-connection-*.ffconcat")
	if e != nil {
		out.Reason = "Cannot create private decoder input."
		return out
	}
	name := file.Name()
	defer os.Remove(name)
	// Credentials stay in a 0600 private file, never process arguments or retained output.
	source := strings.ReplaceAll(private.String(), "'", "%27")
	_, e = fmt.Fprintf(file, "ffconcat version 1.0\nfile '%s'\noption rtsp_transport tcp\noption timeout 5000000\noption probesize 1000000\noption analyzeduration 3000000\n", source)
	closeErr := file.Close()
	if e != nil || closeErr != nil {
		return out
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-hide_banner", "-loglevel", "error", "-max_alloc", "16777216", "-threads", "1", "-filter_threads", "1", "-protocol_whitelist", "file,rtsp,tcp", "-f", "concat", "-safe", "0", "-i", name, "-an", "-sn", "-frames:v", "1", "-threads", "1", "-f", "image2pipe", "-vcodec", "png", "pipe:1")
	var output boundedOutput
	cmd.Stdout = &output
	cmd.Stderr = io.Discard
	if cmd.Run() == nil && setFrame(&out, output.Bytes()) {
		out.Reason = "A video frame decoded over explicitly enabled RTSP/TCP. Model identity is not proven."
	}
	return out
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, fmt.Errorf("frame output exceeds limit")
	}
	return b.Buffer.Write(p)
}
func describe(ctx context.Context, u *url.URL, auth string) (status int, challenge, sdp string, err error) {
	conn, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", u.Host)
	if e != nil {
		return 0, "", "", e
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(6 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	req := "DESCRIBE " + u.String() + " RTSP/1.0\r\nCSeq: 1\r\nAccept: application/sdp\r\nUser-Agent: CCTVScan\r\n"
	if auth != "" {
		req += "Authorization: " + auth + "\r\n"
	}
	req += "\r\n"
	if _, e = io.WriteString(conn, req); e != nil {
		return 0, "", "", e
	}
	reader := bufio.NewReader(io.LimitReader(conn, 65536))
	line, e := reader.ReadString('\n')
	if e != nil {
		return 0, "", "", e
	}
	if !strings.HasPrefix(line, "RTSP/1.0 ") {
		return 0, "", "", fmt.Errorf("not RTSP")
	}
	if _, e = fmt.Sscanf(line, "RTSP/1.0 %d", &status); e != nil {
		return 0, "", "", e
	}
	h, e := textproto.NewReader(reader).ReadMIMEHeader()
	if e != nil {
		return 0, "", "", e
	}
	challenge = chooseChallenge(h.Values("WWW-Authenticate"))
	n, _ := strconv.Atoi(h.Get("Content-Length"))
	if n < 0 || n > 32768 {
		return status, challenge, "", fmt.Errorf("RTSP response too large")
	}
	if n > 0 {
		body := make([]byte, n)
		if _, e = io.ReadFull(reader, body); e != nil {
			return status, challenge, "", e
		}
		sdp = string(body)
	}
	return status, challenge, sdp, nil
}
