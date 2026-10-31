package fingerprint

import (
	"regexp"
	"strings"

	"github.com/postfix/cctvscan/internal/cvedb"
)

// Brand keyword lists
var brandKeysHikvision = []string{"hikvision", "hik-connect", "ivms"}
var brandKeysDahua = []string{"dahua", "smartpss", "dmss"}
var brandKeysAxis = []string{"axis", "axis communications", "axis camera", "axis mjpg"}
var brandKeysSony = []string{"sony", "ipela", "sony network camera"}
var brandKeysBosch = []string{"bosch", "flexidome", "dinion", "autodome"}
var brandKeysSamsung = []string{"samsung", "samsung techwin", "samsung sds", "hanwha", "wisenet"}
var brandKeysPanasonic = []string{"panasonic"}
var brandKeysVivotek = []string{"vivotek"}
var brandKeysCPPlus = []string{"cp plus", "cp-plus", "cpplus", "cp_plus", "uvr-0401e1", "uvr0401e1"}
var brandKeysFoscam = []string{"foscam", "foscam camera", "foscam ip camera"}
var brandKeysDLink = []string{"d-link", "dlink", "d-link camera", "dlink camera"}
var brandKeysEasyN = []string{"easyn", "easyn camera", "easyn ip camera"}
var brandKeysEdimax = []string{"edimax", "edimax camera", "edimax ip camera"}
var brandKeysIntellinet = []string{"intellinet", "intellinet camera", "intellinet ip camera"}
var brandKeysTPLink = []string{"tp-link", "tplink", "tp-link camera", "tplink camera"}
var brandKeysTrendnet = []string{"trendnet", "trendnet camera", "trendnet ip camera"}
var brandKeysMobotix = []string{"mobotix", "mobotix camera", "mobotix ip camera"}
var brandKeysGeneric = []string{"camera", "webcam", "surveillance", "ip camera", "network camera", "dvr", "nvr", "recorder"}

// Version detection patterns for different brands
var versionPatterns = map[string]*regexp.Regexp{
	"Hikvision": regexp.MustCompile(`(?i)(?:hikvision|hik-connect|ivms).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Dahua":     regexp.MustCompile(`(?i)(?:dahua|dss|smartpss).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Axis":      regexp.MustCompile(`(?i)(?:axis|axis communications).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Sony":      regexp.MustCompile(`(?i)(?:sony|ipela).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Bosch":     regexp.MustCompile(`(?i)(?:bosch|flexidome|dinion).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Samsung":   regexp.MustCompile(`(?i)(?:samsung|hanwha|wisenet).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Panasonic": regexp.MustCompile(`(?i)(?:panasonic|wv|bb|blc).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"Vivotek":   regexp.MustCompile(`(?i)(?:vivotek|fd|sd).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
}

// Web page content patterns for brand detection

// Title patterns for brand detection

// DetectResult contains brand detection results with version information
type DetectResult struct {
	Brand   string
	Note    string
	Version string
}

// Detect performs enhanced brand detection with version enumeration
func Detect(serverHdr, body, rtspServer string) (brand, note string) {
	result := DetectWithVersion(serverHdr, body, rtspServer)
	return result.Brand, result.Note
}

// DetectWithVersion performs brand detection and returns version information
func DetectWithVersion(serverHdr, body, rtspServer string) DetectResult {
	text := strings.ToLower(serverHdr + "\n" + body + "\n" + rtspServer)
	for _, brand := range []string{"Hikvision", "Dahua", "Axis", "Sony", "Bosch", "Samsung", "Panasonic", "Vivotek", "CP Plus", "Foscam", "D-Link", "EasyN", "Edimax", "Intellinet", "TP-Link", "Trendnet", "Mobotix"} {
		if headerContainsAny(text, getBrandKeys(brand)) {
			version := extractVersion(body, brand)
			if version == "" {
				version = extractVersion(rtspServer, brand)
			}
			note := ""
			if version != "" {
				note = "Version: " + version
			}
			return DetectResult{Brand: brand, Note: note, Version: version}
		}
	}
	if headerContainsAny(text, brandKeysGeneric) {
		return DetectResult{Brand: "Unknown cam"}
	}
	return DetectResult{}
}

// extractVersion extracts version information from content for a specific brand
func extractVersion(content, brand string) string {
	if pattern, exists := versionPatterns[brand]; exists {
		matches := pattern.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1]
		}
	}
	return ""
}

// getBrandKeys returns the keyword list for a specific brand
func getBrandKeys(brand string) []string {
	switch brand {
	case "Hikvision":
		return brandKeysHikvision
	case "Dahua":
		return brandKeysDahua
	case "Axis":
		return brandKeysAxis
	case "Sony":
		return brandKeysSony
	case "Bosch":
		return brandKeysBosch
	case "Samsung":
		return brandKeysSamsung
	case "Panasonic":
		return brandKeysPanasonic
	case "Vivotek":
		return brandKeysVivotek
	case "CP Plus":
		return brandKeysCPPlus
	case "Foscam":
		return brandKeysFoscam
	case "D-Link":
		return brandKeysDLink
	case "EasyN":
		return brandKeysEasyN
	case "Edimax":
		return brandKeysEdimax
	case "Intellinet":
		return brandKeysIntellinet
	case "TP-Link":
		return brandKeysTPLink
	case "Trendnet":
		return brandKeysTrendnet
	case "Mobotix":
		return brandKeysMobotix
	default:
		return []string{}
	}
}

func headerContainsAny(hdr string, keys []string) bool {
	h := strings.ToLower(hdr)
	for _, kw := range keys {
		if strings.Contains(h, kw) {
			return true
		}
	}
	return false
}

// AnalyzeWebContent performs deep analysis of web content for brand detection
func AnalyzeWebContent(body string) DetectResult { return DetectWithVersion("", body, "") }

// ExtractSoftwareInfo extracts software information from web content
// Common software version patterns
var softwarePatterns = map[string]*regexp.Regexp{
	"firmware":   regexp.MustCompile(`(?i)(?:firmware|fw).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"software":   regexp.MustCompile(`(?i)(?:software|sw).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"version":    regexp.MustCompile(`(?i)(?:version|ver).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"build":      regexp.MustCompile(`(?i)(?:build|bld).*?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"release":    regexp.MustCompile(`(?i)(?:release|rel).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"kernel":     regexp.MustCompile(`(?i)(?:kernel|kern).*?(\d+\.\d+\.\d+(?:\.\d+)?)`),
	"bootloader": regexp.MustCompile(`(?i)(?:bootloader|boot).*?v?(\d+\.\d+\.\d+(?:\.\d+)?)`),
}

func ExtractSoftwareInfo(body string) map[string]string {
	info := make(map[string]string)

	for key, pattern := range softwarePatterns {
		matches := pattern.FindStringSubmatch(body)
		if len(matches) > 1 {
			info[key] = matches[1]
		}
	}

	return info
}

// DetectLoginSystem detects the login system used by the camera
var genericLogin = regexp.MustCompile(`(?i)(?:login|admin|webadmin|viewer)`)

func DetectLoginSystem(body string) string {
	if r := DetectWithVersion("", body, ""); r.Brand != "" && r.Brand != "Unknown cam" {
		return r.Brand
	}
	if genericLogin.MatchString(body) {
		return "Generic"
	}
	return "Unknown"
}

func CVEsForBrand(brand string) []string { return cvedb.ForBrand(strings.ToLower(brand)) }
func CVELinks(cves []string) []string {
	out := make([]string, 0, len(cves))
	for _, c := range cves {
		out = append(out, "https://nvd.nist.gov/vuln/detail/"+c)
	}
	return out
}
