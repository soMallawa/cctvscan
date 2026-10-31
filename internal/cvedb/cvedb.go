// Package cvedb provides curated vendor-associated research leads, not confirmed
// findings. Applicability always requires checking the actual model/firmware.
package cvedb

import "strings"

// Reviewed against the CVE Program CNA records (2026-10-31).
// Deliberately omit unverified, unrelated and placeholder records.
var db = map[string][]string{
	"hikvision": {"CVE-2021-36260", "CVE-2017-7921"},
	"dahua":     {"CVE-2021-33044"},
	"axis":      {"CVE-2018-10660"},
	"sony":      {"CVE-2018-3937"},
}

func ForBrand(brand string) []string { return append([]string(nil), db[strings.ToLower(brand)]...) }
