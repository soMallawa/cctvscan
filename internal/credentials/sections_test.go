package credentials

import "testing"

func TestVendorSectionsSelectOnlyMatchingAndGenericPairs(t *testing.T) {
	set, e := Parse([]byte("# Hikvision\nfixture-h:fixture-one\n# Dahua\nfixture-d:fixture-two\n# Samsung\nfixture-s:fixture-three\n# Generic / common\nfixture-g:fixture-four\n"))
	if e != nil {
		t.Fatal(e)
	}
	entries := set.Select("Hikvision")
	if len(entries) != 2 || entries[0].Pair != "fixture-h:fixture-one" || entries[1].Pair != "fixture-g:fixture-four" {
		t.Fatal("vendor selection lost order or included another vendor")
	}
	if got := set.Select("Samsung Techwin"); len(got) != 2 || got[0].Vendor != "samsung" {
		t.Fatal("existing Samsung label was misclassified")
	}
	if got := set.Select("Unknown cam"); len(got) != 1 || got[0].Vendor != "" {
		t.Fatal("unknown devices received unrelated vendor credentials")
	}
}
func TestPlainSetsRemainCompatibleAndPasswordsMayContainColon(t *testing.T) {
	set, e := Parse([]byte("fixture-user:fixture:password\nfixture-user:fixture:password\n"))
	if e != nil {
		t.Fatal(e)
	}
	if len(set.Select("Axis")) != 1 {
		t.Fatal("plain credential compatibility or deduplication lost")
	}
}
