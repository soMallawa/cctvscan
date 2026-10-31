package probe

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/postfix/cctvscan/internal/util"
)

type RTSPInfo struct {
	Status       int
	AuthRequired bool
	Error        error `json:"-"`
	Any          bool
	Server       string
	Public       string
}

func IsRTSPPort(port int) bool {
	switch port {
	case 554, 8554, 10554, 1554, 2554, 3554, 4554, 5554, 6554, 7554, 9554:
		return true
	}
	return false
}

func FilterRTSP(ports []int) []int {
	var out []int
	for _, p := range ports {
		switch p {
		case 554, 8554, 10554, 1554, 2554, 3554, 4554, 5554, 6554, 7554, 9554:
			out = append(out, p)
		}
	}
	return out
}

func ProbeRTSP(ctx context.Context, host string, ports []int) RTSPInfo {
	var info RTSPInfo
	for _, p := range ports {
		if err := ctx.Err(); err != nil {
			info.Error = err
			break
		}
		addr := net.JoinHostPort(host, util.Itoa(p))
		c, err := dialSocket(ctx, "tcp", addr, 1500*time.Millisecond)
		if err != nil {
			info.Error = err
			continue
		}
		fmt.Fprintf(c, "OPTIONS rtsp://%s RTSP/1.0\r\nCSeq: 1\r\n\r\n", addr)
		br := bufio.NewReader(io.LimitReader(c, 16*1024))
		status, err := br.ReadString('\n')
		if err != nil {
			c.Close()
			info.Error = err
			continue
		}
		fields := strings.Fields(status)
		if len(fields) < 2 || fields[0] != "RTSP/1.0" {
			c.Close()
			continue
		}
		code := util.Atoi(fields[1])
		if code < 100 || code > 599 {
			c.Close()
			continue
		}
		info.Any = true
		info.Status = code
		info.AuthRequired = code == 401 || code == 403
		for {
			line, err := br.ReadString('\n')
			line = strings.TrimSpace(line)
			if line == "" || err != nil {
				break
			}
			key, val, ok := strings.Cut(line, ":")
			if !ok {
				continue
			}
			switch strings.ToLower(key) {
			case "server":
				info.Server = strings.TrimSpace(val)
			case "public":
				info.Public = strings.TrimSpace(val)
			}
		}
		c.Close()
		info.Error = nil
		break
	}
	if ctx.Err() != nil {
		info.Error = ctx.Err()
	}
	return info
}

// RTSPPaths contains common RTSP stream paths
var RTSPPaths = []string{
	"/live", "/live.sdp", "/h264", "/h264.sdp", "/mpeg4", "/stream1", "/stream2", "/main", "/sub", "/1",
	"/ch0_0.264", "/Streaming/Channels/1", "/Streaming/Channels/101", "/onvif/streaming/channels/1",
	"/axis-media/media.amp", "/cam/realmonitor?channel=1&subtype=0",
}

// RTSPCommands contains RTSP commands for capability detection
var RTSPCommands = []string{
	"OPTIONS", "DESCRIBE", "PLAY", "PAUSE", "SETUP", "TEARDOWN", "SET_PARAMETER", "GET_PARAMETER",
}

// ProbeRTSPDescribe performs DESCRIBE request to validate RTSP streams
func ProbeRTSPDescribe(ctx context.Context, host string, port int, path string) (int, bool, error) {
	addr := net.JoinHostPort(host, util.Itoa(port))
	c, err := dialSocket(ctx, "tcp", addr, 2*time.Second)
	if err != nil {
		return -1, false, err
	}
	defer c.Close()

	url := "rtsp://" + addr + path
	fmt.Fprintf(c, "DESCRIBE %s RTSP/1.0\r\nCSeq: 2\r\nUser-Agent: CCTVScan/1.0\r\nAccept: application/sdp\r\n\r\n", url)

	br := bufio.NewReader(c)
	status, err := br.ReadString('\n')
	if err != nil {
		return -1, false, err
	}

	var codeOut int = -1
	if strings.HasPrefix(status, "RTSP/1.0 ") {
		parts := strings.Split(status, " ")
		if len(parts) >= 2 {
			codeOut = util.Atoi(parts[1])
		}
	}

	// Read headers
	var contentType string
	var contentLength int = -1
	for {
		line, err := br.ReadString('\n')
		if err != nil || line == "\r\n" || line == "\n" {
			break
		}
		line = strings.TrimSpace(line)
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "content-type:") {
			contentType = strings.TrimSpace(line[13:])
		}
		if strings.HasPrefix(low, "content-length:") {
			contentLength = util.Atoi(strings.TrimSpace(line[15:]))
		}
	}

	// Read partial body to validate SDP
	var body []byte
	if contentLength > 0 {
		body = make([]byte, min(contentLength, 2048))
		_, _ = io.ReadFull(br, body)
	} else {
		// Read what we can get in reasonable time
		body, _ = io.ReadAll(io.LimitReader(br, 2048))
	}

	// Validate SDP content
	bodyStr := string(body)
	headerSdp := strings.Contains(strings.ToLower(contentType), "application/sdp") || strings.Contains(contentType, "/sdp")
	looksSdp := strings.Contains(bodyStr, "v=0") && strings.Contains(bodyStr, "m=video")

	return codeOut, (headerSdp && looksSdp), nil
}
