package catalogue

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPinnedOfflineCatalogueSearchAndOriginalDetail(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	page := c.Search("RLC-823A", "", "", 10, 0)
	if page.Total < 1 || len(page.Items) < 1 {
		t.Fatal("real upstream model missing")
	}
	raw, ok := c.Detail(page.Items[0].ID)
	if !ok || !json.Valid(raw) {
		t.Fatal("original record missing")
	}
	var detail map[string]any
	json.Unmarshal(raw, &detail)
	if detail["model"] != page.Items[0].Model || detail["sources"] == nil {
		t.Fatal("detail lost original identity or provenance")
	}
	if c.Manifest.Models != len(c.Models) || c.Manifest.Models < 29000 || c.Manifest.Revision == "" {
		t.Fatal("missing pinned coverage")
	}
}
func TestCatalogueMatchDoesNotManufactureCameraOrModelProof(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	a := c.Identify("Hikvision", "")
	if a.State != "brand_only" || len(a.Candidates) != 0 || len(a.ProfileIDs) == 0 {
		t.Fatalf("brand match should not guess a model: %#v", a)
	}
	a = c.Identify("", "DVR products and Web Client")
	if a.State != "unmatched" {
		t.Fatal("generic words are not catalogue identity")
	}
	var chosen Model
	for _, m := range c.Models {
		if strings.EqualFold(m.Brand, "Hikvision") && strings.Contains(m.Model, "-") {
			chosen = m
			break
		}
	}
	a = c.Identify("Hikvision", "Model: "+chosen.Model)
	if a.State != "model_candidate" || len(a.Candidates) == 0 || a.Candidates[0].ID != chosen.ID {
		t.Fatal("explicit model evidence not matched")
	}
	for _, id := range a.ProfileIDs {
		p, ok := c.Profile(id)
		if !ok || p.Status != "documented" || p.Source == "" {
			t.Fatal("unverified/source-free profile exposed")
		}
	}
}
