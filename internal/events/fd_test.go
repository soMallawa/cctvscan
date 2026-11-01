package events

import (
	"encoding/json"
	"os"
	"testing"
)

func TestPrivateDescriptorRoundTrip(t *testing.T) {
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	defer w.Close()
	observer, close, e := OpenFD(int(w.Fd()))
	if e != nil {
		t.Fatal(e)
	}
	if e = observer.Emit("phase", map[string]string{"phase": "port_discovery"}); e != nil {
		t.Fatal(e)
	}
	close()
	var frame map[string]any
	if e = json.NewDecoder(r).Decode(&frame); e != nil {
		t.Fatal(e)
	}
	if frame["kind"] != "phase" || frame["schema_version"] != float64(1) {
		t.Fatal("invalid wire frame")
	}
}
func TestObserverRejectsRegularFiles(t *testing.T) {
	f, e := os.CreateTemp(t.TempDir(), "fixture")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if _, close, e := OpenFD(int(f.Fd())); e == nil {
		close()
		t.Fatal("regular output file accepted as private IPC descriptor")
	}
}
