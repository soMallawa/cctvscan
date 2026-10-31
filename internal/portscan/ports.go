package portscan

import (
	"fmt"
	"strconv"
	"strings"
)

// PortCount validates numeric port/range syntax without issuing network traffic.
// Port zero is retained when explicitly supplied, including 0-65535.
func PortCount(spec string) (int, error) {
	var seen [65536]bool
	n := 0
	for _, part := range strings.Split(spec, ",") {
		bounds := strings.Split(strings.TrimSpace(part), "-")
		if len(bounds) > 2 || len(bounds) == 0 {
			return 0, fmt.Errorf("invalid port specification")
		}
		a, e := strconv.Atoi(bounds[0])
		if e != nil || a < 0 || a > 65535 {
			return 0, fmt.Errorf("invalid port specification")
		}
		b := a
		if len(bounds) == 2 {
			b, e = strconv.Atoi(bounds[1])
			if e != nil || b < a || b > 65535 {
				return 0, fmt.Errorf("invalid port range")
			}
		}
		for p := a; p <= b; p++ {
			if !seen[p] {
				seen[p] = true
				n++
			}
		}
	}
	return n, nil
}
