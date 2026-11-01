package config

import (
	"encoding/binary"
	"errors"
	"math/bits"
	"net/netip"
	"strings"
)

// compactRange covers an inclusive IPv4 interval exactly, without expanding
// individual addresses or including addresses outside the requested endpoints.
func compactRange(raw string) ([]string, error) {
	left, right, ok := strings.Cut(raw, "-")
	first, e1 := netip.ParseAddr(strings.TrimSpace(left))
	last, e2 := netip.ParseAddr(strings.TrimSpace(right))
	if !ok || e1 != nil || e2 != nil || !first.Is4() || !last.Is4() || first.Compare(last) > 0 {
		return nil, errors.New("invalid IPv4 range")
	}
	a, b := first.As4(), last.As4()
	start, end := uint64(binary.BigEndian.Uint32(a[:])), uint64(binary.BigEndian.Uint32(b[:]))
	var out []string
	for start <= end {
		width := min(bits.TrailingZeros64(start), bits.Len64(end-start+1)-1, 32)
		var ip [4]byte
		binary.BigEndian.PutUint32(ip[:], uint32(start))
		address := netip.AddrFrom4(ip)
		if width == 0 {
			out = append(out, address.String())
		} else {
			out = append(out, netip.PrefixFrom(address, 32-width).String())
		}
		start += uint64(1) << width
	}
	return out, nil
}
