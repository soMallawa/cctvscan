package cvedb

import (
	"regexp"
	"testing"
)

func TestCuratedIdentifiers(t *testing.T) {
	r := regexp.MustCompile(`^CVE-\d{4}-\d{4,}$`)
	for brand, ids := range db {
		for _, id := range ids {
			if !r.MatchString(id) {
				t.Errorf("invalid %s advisory %s", brand, id)
			}
			if brand == "hikvision" && id == "CVE-2021-31955" {
				t.Error("Windows advisory assigned to camera vendor")
			}
		}
	}
}
