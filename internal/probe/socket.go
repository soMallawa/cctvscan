package probe

import (
	"context"
	"net"
	"time"
)

type contextConn struct {
	net.Conn
	stop func() bool
}

func (c *contextConn) Close() error { c.stop(); return c.Conn.Close() }
func dialSocket(ctx context.Context, network, address string, timeout time.Duration) (net.Conn, error) {
	c, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, network, address)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	c.SetDeadline(deadline)
	stop := context.AfterFunc(ctx, func() { c.Close() })
	return &contextConn{c, stop}, nil
}
