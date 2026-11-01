// Package credentials keeps vendor annotations alongside private credential pairs.
// Pair values must never be included in observations, logs or API responses.
package credentials

import (
	"errors"
	"github.com/postfix/cctvscan/internal/catalogue"
	"strings"
	"unicode"
)

type Entry struct {
	Vendor string
	Pair   string `json:"-"`
}
type Set struct {
	Entries     []Entry
	VendorAware bool
}

func CanonicalVendor(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
	switch s {
	case "samsungtechwin":
		return "samsung"
	case "axiscommunications":
		return "axis"
	}
	return s
}
func heading(line string) (string, bool) {
	h := strings.ToLower(strings.TrimSpace(strings.TrimLeft(line, "# \t=-[]")))
	if strings.HasPrefix(h, "vendor:") {
		v := CanonicalVendor(strings.TrimSpace(strings.TrimPrefix(h, "vendor:")))
		return v, v != ""
	}
	names := []string{"Samsung", "Samsung Techwin", "CP Plus", "Axis Communications"}
	if c, e := catalogue.Default(); e == nil {
		for _, b := range c.Brands {
			names = append(names, b.Name)
		}
	}
	best := ""
	for _, name := range names {
		n := strings.ToLower(name)
		if strings.HasPrefix(h, n) && (len(h) == len(n) || !unicode.IsLetter(rune(h[len(n)]))) {
			if len(n) > len(best) {
				best = n
			}
		}
	}
	if best != "" {
		return CanonicalVendor(best), true
	}
	for _, word := range strings.FieldsFunc(h, func(r rune) bool { return !unicode.IsLetter(r) }) {
		if word == "generic" || word == "common" || word == "universal" {
			return "", true
		}
	}
	return "", false
}
func Parse(content []byte) (Set, error) {
	var set Set
	if len(content) > 1<<20 {
		return set, errors.New("credential file exceeds 1 MiB limit")
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	for _, s := range lines {
		if strings.HasPrefix(strings.TrimSpace(s), "#") {
			if v, ok := heading(s); ok && v != "" {
				set.VendorAware = true
			}
		}
	}
	vendor := ""
	for _, s := range lines {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if strings.ContainsAny(s, "\x00\r") || len(s) > 4096 {
			return Set{}, errors.New("invalid credential file format")
		}
		if strings.HasPrefix(s, "#") {
			if v, ok := heading(s); ok {
				vendor = v
			} else if set.VendorAware {
				vendor = "unclassified"
			}
			continue
		}
		if !strings.Contains(s, ":") {
			return Set{}, errors.New("use one username:password pair per line, with optional vendor comment headers")
		}
		set.Entries = append(set.Entries, Entry{Vendor: vendor, Pair: s})
		if len(set.Entries) > 10000 {
			return Set{}, errors.New("credential file exceeds 10000 entries")
		}
	}
	return set, nil
}
func (s Set) Select(brand string) []Entry {
	out := []Entry{}
	seen := map[string]bool{}
	wanted := CanonicalVendor(brand)
	add := func(v string) {
		for _, e := range s.Entries {
			if e.Vendor == v && !seen[e.Pair] {
				seen[e.Pair] = true
				out = append(out, e)
			}
		}
	}
	if wanted != "" && wanted != "unclassified" {
		add(wanted)
	}
	add("")
	return out
}
