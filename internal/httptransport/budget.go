package httptransport

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"
)

type hostSlots struct {
	slots chan struct{}
	refs  int
}
type requestBudget struct {
	all      chan struct{}
	perHost  int
	mu       sync.Mutex
	hosts    map[string]*hostSlots
	interval time.Duration
	next     time.Time
}

var budgets = struct {
	sync.RWMutex
	b *requestBudget
}{b: &requestBudget{all: make(chan struct{}, 5), perHost: 2, hosts: map[string]*hostSlots{}}}

// ConfigureLimits is startup-only. Existing requests retain their original budget.
func ConfigureLimits(concurrent, perHost int, rps float64) error {
	if concurrent < 1 || perHost < 1 || rps < 0 {
		return errors.New("invalid HTTP request budget")
	}
	b := &requestBudget{all: make(chan struct{}, concurrent), perHost: perHost, hosts: map[string]*hostSlots{}}
	if rps > 0 {
		b.interval = time.Duration(float64(time.Second) / rps)
	}
	budgets.Lock()
	budgets.b = b
	budgets.Unlock()
	return nil
}
func (b *requestBudget) acquire(ctx context.Context, host string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	h := b.hosts[host]
	if h == nil {
		h = &hostSlots{slots: make(chan struct{}, b.perHost)}
		b.hosts[host] = h
	}
	h.refs++
	b.mu.Unlock()
	drop := func() {
		b.mu.Lock()
		h.refs--
		if h.refs == 0 {
			delete(b.hosts, host)
		}
		b.mu.Unlock()
	}
	select {
	case h.slots <- struct{}{}:
	case <-ctx.Done():
		drop()
		return nil, ctx.Err()
	}
	select {
	case b.all <- struct{}{}:
	case <-ctx.Done():
		<-h.slots
		drop()
		return nil, ctx.Err()
	}
	var once sync.Once
	release := func() { once.Do(func() { <-b.all; <-h.slots; drop() }) }
	// Pace actual admissions, not callers that may still wait on body slots.
	// Recheck after each timer so delayed waiters cannot consume stale times
	// together. Cancellation after either slot acquisition releases both slots.
	for {
		if err := ctx.Err(); err != nil {
			release()
			return nil, err
		}
		b.mu.Lock()
		now := time.Now()
		wait := b.next.Sub(now)
		if b.interval == 0 || wait <= 0 {
			b.next = now.Add(b.interval)
			b.mu.Unlock()
			return release, nil
		}
		b.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			release()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

type budgetBody struct {
	io.ReadCloser
	release func()
}

func (b *budgetBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	if e != nil {
		b.release()
	}
	return n, e
}
func (b *budgetBody) Close() error { e := b.ReadCloser.Close(); b.release(); return e }

type limitedTransport struct{ base http.RoundTripper }

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	budgets.RLock()
	b := budgets.b
	budgets.RUnlock()
	release, e := b.acquire(req.Context(), req.URL.Hostname())
	if e != nil {
		return nil, e
	}
	resp, e := t.base.RoundTrip(req)
	if e != nil {
		release()
		return nil, e
	}
	if resp.Body == nil || resp.Body == http.NoBody {
		release()
	} else {
		resp.Body = &budgetBody{resp.Body, release}
	}
	return resp, nil
}

// Do applies budgets per wire request, including redirects, while retaining the
// caller's timeout, redirect policy, cookie jar and underlying connection pool.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	copy := *client
	base := copy.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	copy.Transport = &limitedTransport{base}
	return copy.Do(req)
}

var clients = struct {
	sync.Mutex
	transport http.RoundTripper
	byTimeout map[time.Duration]*http.Client
}{byTimeout: map[time.Duration]*http.Client{}}

// Client shares connections across first-party helpers. Target TLS remains
// permissive for cameras; NewTransport separately verifies HTTPS proxy identity.
func Client(timeout time.Duration) *http.Client {
	clients.Lock()
	defer clients.Unlock()
	if c := clients.byTimeout[timeout]; c != nil {
		return c
	}
	if clients.transport == nil {
		clients.transport = NewTransport(&http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, DialContext: (&net.Dialer{Timeout: 1200 * time.Millisecond}).DialContext, TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxIdleConns: 32, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 16, IdleConnTimeout: 30 * time.Second})
	}
	c := &http.Client{Timeout: timeout, Transport: clients.transport}
	if len(clients.byTimeout) < 16 {
		clients.byTimeout[timeout] = c
	}
	return c
}
func resetClients() {
	clients.Lock()
	defer clients.Unlock()
	if c, ok := clients.transport.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
	clients.transport = nil
	clients.byTimeout = map[time.Duration]*http.Client{}
}
