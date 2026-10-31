// Package targets validates compact IPv4 targets and offers bounded, cancellable
// expansion for legacy callers. The CLI passes compact ranges to scan backends.
package targets

import (
	"bufio"
	"context"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strings"
)

const MaxExpandedTargets = 1 << 20

func Normalize(ctx context.Context, args []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(raw string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		raw = strings.TrimSpace(raw)
		if raw == "" || strings.HasPrefix(raw, "#") {
			return nil
		}
		var canonical string
		if p, err := netip.ParsePrefix(raw); err == nil {
			if !p.Addr().Is4() {
				return fmt.Errorf("IPv6 scanning is not supported: %q", raw)
			}
			canonical = p.Masked().String()
		} else if a, err := netip.ParseAddr(raw); err == nil && a.Unmap().Is4() {
			canonical = a.Unmap().String()
		} else {
			return fmt.Errorf("invalid IPv4 target %q", raw)
		}
		if !seen[canonical] {
			seen[canonical] = true
			out = append(out, canonical)
		}
		return nil
	}
	for _, arg := range args {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if info, err := os.Stat(arg); err == nil {
			if !info.Mode().IsRegular() {
				return nil, fmt.Errorf("target file is not regular: %s", arg)
			}
			err = func() error {
				f, e := os.Open(arg)
				if e != nil {
					return e
				}
				defer f.Close()
				sc := bufio.NewScanner(f)
				sc.Buffer(make([]byte, 4096), 1024*1024)
				for sc.Scan() {
					if e = add(sc.Text()); e != nil {
						return e
					}
				}
				return sc.Err()
			}()
			if err != nil {
				return nil, err
			}
		} else {
			if err = add(arg); err != nil {
				return nil, err
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// Count returns an upper bound: overlapping prefixes are not silently sampled.
func Count(targets []string) uint64 {
	var n uint64
	for _, s := range targets {
		if p, e := netip.ParsePrefix(s); e == nil && p.Addr().Is4() {
			n += uint64(1) << uint(32-p.Bits())
		} else {
			n++
		}
	}
	return n
}
func ExpandContext(ctx context.Context, args []string) ([]string, error) {
	compact, err := Normalize(ctx, args)
	if err != nil {
		return nil, err
	}
	n := Count(compact)
	if n > MaxExpandedTargets {
		return nil, fmt.Errorf("expansion exceeds %d addresses; use compact ranges", MaxExpandedTargets)
	}
	out := make([]string, 0, int(n))
	seen := make(map[string]bool, int(n))
	appendAddr := func(a netip.Addr) {
		s := a.String()
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range compact {
		if p, e := netip.ParsePrefix(s); e == nil {
			for a := p.Masked().Addr(); a.IsValid() && p.Contains(a); a = a.Next() {
				if err = ctx.Err(); err != nil {
					return nil, err
				}
				appendAddr(a)
			}
		} else {
			if err = ctx.Err(); err != nil {
				return nil, err
			}
			a, _ := netip.ParseAddr(s)
			appendAddr(a)
		}
	}
	return out, nil
}
func Expand(args []string) ([]string, error) { return ExpandContext(context.Background(), args) }
func FromArgsOrFile(args []string, file string) ([]string, error) {
	if file != "" {
		args = append(append([]string{}, args...), file)
	}
	return Expand(args)
}

// Kept for existing package-local callers; iterators use netip.Addr.Next, whose
// invalid wrap sentinel makes /0 termination explicit.
