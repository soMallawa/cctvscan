package events

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strconv"
	"sync"
	"testing"
)

func TestOrderedVersionedObservations(t *testing.T) {
	var out bytes.Buffer
	w := New(&out)
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			if e := w.Emit("phase", map[string]string{"phase": "host_processing"}); e != nil {
				t.Error(e)
			}
		}()
	}
	group.Wait()
	scan := bufio.NewScanner(&out)
	n := 0
	for scan.Scan() {
		n++
		var frame struct {
			SchemaVersion int               `json:"schema_version"`
			Seq           string            `json:"seq"`
			Kind          string            `json:"kind"`
			At            string            `json:"at"`
			Data          map[string]string `json:"data"`
		}
		if e := json.Unmarshal(scan.Bytes(), &frame); e != nil {
			t.Fatal(e)
		}
		if frame.SchemaVersion != 1 || frame.Seq != strconv.Itoa(n) || frame.Kind != "phase" || frame.At == "" || frame.Data["phase"] != "host_processing" {
			t.Fatal("invalid ordered observation")
		}
	}
	if n != 20 {
		t.Fatalf("got %d observations", n)
	}
}
