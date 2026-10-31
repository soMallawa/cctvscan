package fingerprint

import (
	"crypto/sha256"
	"encoding/json"
	"github.com/postfix/cctvscan/internal/cvedb"
	"sync"
)

const maxBrandCache = 1024

var brandCache = struct {
	sync.RWMutex
	cache map[[32]byte]BrandResult
	keys  [][32]byte
	next  int
}{cache: make(map[[32]byte]BrandResult)}

type BrandResult struct{ Brand, Note string }

func OptimizedDetect(serverHdr, body, rtspServer string) (string, string) {
	raw, _ := json.Marshal([3]string{serverHdr, body, rtspServer})
	key := sha256.Sum256(raw)
	brandCache.RLock()
	r, ok := brandCache.cache[key]
	brandCache.RUnlock()
	if ok {
		return r.Brand, r.Note
	}
	b, n := detectBrand(serverHdr, body, rtspServer)
	brandCache.Lock()
	if _, exists := brandCache.cache[key]; !exists {
		if len(brandCache.keys) < maxBrandCache {
			brandCache.keys = append(brandCache.keys, key)
		} else {
			delete(brandCache.cache, brandCache.keys[brandCache.next])
			brandCache.keys[brandCache.next] = key
			brandCache.next = (brandCache.next + 1) % maxBrandCache
		}
		brandCache.cache[key] = BrandResult{b, n}
	}
	brandCache.Unlock()
	return b, n
}
func detectBrand(serverHdr, body, rtspServer string) (string, string) {
	r := DetectWithVersion(serverHdr, body, rtspServer)
	return r.Brand, r.Note
}

// CVE data is already a direct immutable brand map; scanning body-cache entries
// was slower and mixed two unrelated caches. ForBrand returns a defensive copy.
func OptimizedCVEsForBrand(brand string) []string { return cvedb.ForBrand(brand) }
func OptimizedCVELinks(cves []string) []string    { return CVELinks(cves) }
func ClearCache() {
	brandCache.Lock()
	defer brandCache.Unlock()
	brandCache.cache = make(map[[32]byte]BrandResult)
	brandCache.keys = nil
	brandCache.next = 0
}
func GetCacheStats() (int, int) {
	brandCache.RLock()
	defer brandCache.RUnlock()
	return len(brandCache.cache), 0
}
