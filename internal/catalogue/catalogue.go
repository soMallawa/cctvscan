// Package catalogue is an immutable, offline reference, not a claim of device identity.
package catalogue

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"unicode"
)

//go:embed assets/index.json.gz
var indexData []byte

//go:embed assets/records.pack
var recordData []byte

type Manifest struct {
	Source        string            `json:"source"`
	Revision      string            `json:"revision"`
	License       string            `json:"license"`
	Models        int               `json:"models"`
	Brands        int               `json:"brands"`
	Profiles      int               `json:"profiles"`
	SourceHashes  map[string]string `json:"source_hashes"`
	PackSHA256    string            `json:"pack_sha256"`
	FormatVersion int               `json:"format_version"`
}
type Model struct {
	ID         string   `json:"id"`
	Brand      string   `json:"brand"`
	BrandKey   string   `json:"brand_key"`
	Model      string   `json:"model"`
	Type       string   `json:"type"`
	Megapixels *float64 `json:"megapixels"`
	Resolution string   `json:"resolution"`
	Protocols  []string `json:"protocols"`
}
type Brand struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Count        int    `json:"count"`
	ProfileCount int    `json:"profile_count"`
	Offset       int64  `json:"offset"`
	Length       int64  `json:"length"`
}
type Profile struct {
	ID         string `json:"id"`
	Brand      string `json:"brand"`
	BrandKey   string `json:"brand_key"`
	Label      string `json:"label"`
	Scheme     string `json:"scheme"`
	Port       int    `json:"port"`
	Path       string `json:"path"`
	Codec      string `json:"codec"`
	Status     string `json:"status"`
	Source     string `json:"source"`
	VerifiedOn string `json:"verified_on"`
	Notes      string `json:"notes"`
}
type Catalogue struct {
	Manifest    Manifest  `json:"manifest"`
	Models      []Model   `json:"models"`
	Brands      []Brand   `json:"brands"`
	Profiles    []Profile `json:"profiles"`
	byID        map[string]int
	byBrand     map[string]int
	byProfile   map[string]int
	modelTokens map[string][]int
	searchable  []string
}
type Page struct {
	Items  []Model `json:"items"`
	Total  int     `json:"total"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
}
type Assessment struct {
	State      string   `json:"state"`
	Reason     string   `json:"reason"`
	Brand      string   `json:"brand,omitempty"`
	Candidates []Model  `json:"candidates"`
	ProfileIDs []string `json:"profile_ids"`
	Revision   string   `json:"revision"`
}

var once sync.Once
var singleton *Catalogue
var loadErr error

func Default() (*Catalogue, error) {
	once.Do(func() { singleton, loadErr = load() })
	return singleton, loadErr
}
func load() (*Catalogue, error) {
	z, e := gzip.NewReader(bytes.NewReader(indexData))
	if e != nil {
		return nil, e
	}
	defer z.Close()
	c := &Catalogue{}
	if e = json.NewDecoder(io.LimitReader(z, 16<<20)).Decode(c); e != nil {
		return nil, e
	}
	sum := sha256.Sum256(recordData)
	if hex.EncodeToString(sum[:]) != c.Manifest.PackSHA256 {
		return nil, fmt.Errorf("catalogue pack integrity mismatch")
	}
	c.byID = map[string]int{}
	c.byBrand = map[string]int{}
	c.byProfile = map[string]int{}
	c.modelTokens = map[string][]int{}
	for i, m := range c.Models {
		c.byID[m.ID] = i
		c.searchable = append(c.searchable, strings.ToLower(m.Brand+" "+m.Model+" "+m.Type+" "+m.Resolution))
		token := normalize(m.Model)
		if len(token) >= 5 && strings.ContainsAny(token, "0123456789") {
			c.modelTokens[token] = append(c.modelTokens[token], i)
		}
	}
	for i, b := range c.Brands {
		c.byBrand[b.Key] = i
	}
	for i, p := range c.Profiles {
		c.byProfile[p.ID] = i
	}
	return c, nil
}
func normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func (c *Catalogue) Search(q, brand, protocol string, limit, offset int) Page {
	if limit < 1 || limit > 50 {
		limit = 10
	}
	if offset < 0 {
		offset = 0
	}
	page := Page{Items: []Model{}, Limit: limit, Offset: offset}
	terms := strings.Fields(strings.ToLower(q))
	for i, m := range c.Models {
		if brand != "" && m.BrandKey != brand && !strings.EqualFold(m.Brand, brand) {
			continue
		}
		if protocol != "" {
			found := false
			for _, p := range m.Protocols {
				found = found || strings.EqualFold(p, protocol)
			}
			if !found {
				continue
			}
		}
		match := true
		for _, term := range terms {
			if !strings.Contains(c.searchable[i], term) {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		if page.Total >= offset && len(page.Items) < limit {
			page.Items = append(page.Items, m)
		}
		page.Total++
	}
	return page
}
func (c *Catalogue) Detail(id string) (json.RawMessage, bool) {
	i, ok := c.byID[id]
	if !ok {
		return nil, false
	}
	bi, ok := c.byBrand[c.Models[i].BrandKey]
	if !ok {
		return nil, false
	}
	b := c.Brands[bi]
	if b.Offset < 0 || b.Length < 1 || b.Offset+b.Length > int64(len(recordData)) {
		return nil, false
	}
	z, e := gzip.NewReader(bytes.NewReader(recordData[b.Offset : b.Offset+b.Length]))
	if e != nil {
		return nil, false
	}
	defer z.Close()
	scan := bufio.NewScanner(z)
	scan.Buffer(make([]byte, 8192), 1<<20)
	for scan.Scan() {
		var v struct {
			ID string `json:"id"`
		}
		raw := scan.Bytes()
		if json.Unmarshal(raw, &v) == nil && v.ID == id {
			return append(json.RawMessage(nil), raw...), true
		}
	}
	return nil, false
}
func (c *Catalogue) Profile(id string) (Profile, bool) {
	i, ok := c.byProfile[id]
	if !ok {
		return Profile{}, false
	}
	return c.Profiles[i], true
}
func (c *Catalogue) ProfilesFor(brand string) []Profile {
	out := []Profile{}
	for _, p := range c.Profiles {
		if p.BrandKey == brand || strings.EqualFold(p.Brand, brand) {
			out = append(out, p)
		}
	}
	return out
}

var evidenceTokens = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._/-]{3,}`)

func (c *Catalogue) Identify(brand, text string) Assessment {
	a := Assessment{State: "unmatched", Reason: "No explicit catalogue brand or model identity in retained evidence.", Candidates: []Model{}, ProfileIDs: []string{}, Revision: c.Manifest.Revision}
	if len(text) > 32768 {
		text = text[:32768]
	}
	var bk string
	for _, b := range c.Brands {
		if strings.EqualFold(b.Name, brand) || b.Key == brand {
			bk = b.Key
			a.Brand = b.Name
			break
		}
	}
	seen := map[int]bool{}
	for _, token := range evidenceTokens.FindAllString(text, -1) {
		for _, i := range c.modelTokens[normalize(token)] {
			m := c.Models[i]
			if !seen[i] && (bk == "" || m.BrandKey == bk) {
				seen[i] = true
				if len(a.Candidates) < 5 {
					a.Candidates = append(a.Candidates, m)
				}
			}
		}
	}
	if len(a.Candidates) > 0 {
		a.State = "model_candidate"
		a.Reason = "Observed model text matches catalogue candidates; catalogue matching does not prove device identity."
		if bk == "" {
			first := a.Candidates[0]
			same := true
			for _, m := range a.Candidates {
				same = same && m.BrandKey == first.BrandKey
			}
			if same {
				bk = first.BrandKey
				a.Brand = first.Brand
			}
		}
	} else if bk != "" {
		a.State = "brand_only"
		a.Reason = "Observed brand has reference profiles; exact model is unknown."
	}
	if bk != "" {
		for _, p := range c.ProfilesFor(bk) {
			if len(a.ProfileIDs) < 12 {
				a.ProfileIDs = append(a.ProfileIDs, p.ID)
			}
		}
	}
	return a
}

// AnalyzeRecord is an offline reassessment; it never mutates historical evidence.
func (c *Catalogue) AnalyzeRecord(raw json.RawMessage) Assessment {
	var r struct {
		Brand           string
		CameraDetection struct{ Brand, Model, PageTitle, ServerHeader string }
		HTTPMeta        struct {
			Endpoints []struct {
				Server       string
				RootEvidence struct{ BytesBase64 string }
			}
		}
	}
	if json.Unmarshal(raw, &r) != nil {
		return c.Identify("", "")
	}
	brand := r.CameraDetection.Brand
	if brand == "" {
		brand = r.Brand
	}
	var text strings.Builder
	text.WriteString(r.CameraDetection.Model + " " + r.CameraDetection.PageTitle + " " + r.CameraDetection.ServerHeader)
	for _, e := range r.HTTPMeta.Endpoints {
		if text.Len() > 24576 {
			break
		}
		text.WriteString(" " + e.Server)
		if len(e.RootEvidence.BytesBase64) <= 11000 {
			if b, err := base64.StdEncoding.DecodeString(e.RootEvidence.BytesBase64); err == nil && len(b) <= 8192 {
				text.Write(b)
			}
		}
	}
	return c.Identify(brand, text.String())
}
