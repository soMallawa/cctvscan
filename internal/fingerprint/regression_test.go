package fingerprint

import (
	"fmt"
	"testing"
)

func TestExplicitVendorBeatsGenericRecorder(t *testing.T) {
	for _, tc := range []struct{ body, want string }{{"Dahua NVR", "Dahua"}, {"SDK download", ""}, {"Welcome to BBS documentation", ""}, {"Foscam network camera", "Foscam"}, {"CP Plus DVR", "CP Plus"}, {"Foscam security systems", "Foscam"}, {"SNC documentation", ""}, {"UVR manual", ""}} {
		b, _ := OptimizedDetect("nginx", tc.body, "")
		if b != tc.want {
			t.Errorf("%q got %q want %q", tc.body, b, tc.want)
		}
		if r := DetectWithVersion("nginx", tc.body, ""); r.Brand != tc.want {
			t.Errorf("unoptimized %q got %q want %q", tc.body, r.Brand, tc.want)
		}
	}
}
func TestBrandCacheBounded(t *testing.T) {
	ClearCache()
	defer ClearCache()
	for i := 0; i < 3000; i++ {
		OptimizedDetect("nginx", fmt.Sprintf("unique document %d", i), "")
	}
	n, _ := GetCacheStats()
	if n > 1024 {
		t.Fatalf("unbounded entries: %d", n)
	}
}
func TestLoginDetectionDeterministic(t *testing.T) {
	for i := 0; i < 300; i++ {
		if b := DetectLoginSystem("Hikvision login"); b != "Hikvision" {
			t.Fatalf("ambiguous rule selected %s", b)
		}
	}
}
