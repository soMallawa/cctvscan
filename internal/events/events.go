// Package events provides opt-in observations without taking ownership of CLI stdout.
package events

import (
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"sync"
	"time"
)

type Writer struct {
	out io.Writer
	mu  sync.Mutex
	seq uint64
	err error
}

func New(out io.Writer) *Writer { return &Writer{out: out} }
func (w *Writer) Emit(kind string, data any) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	if w.out == nil {
		return io.ErrClosedPipe
	}
	if kind == "" {
		return errors.New("event kind required")
	}
	frame := struct {
		Version int    `json:"schema_version"`
		Seq     string `json:"seq"`
		Kind    string `json:"kind"`
		At      string `json:"at"`
		Data    any    `json:"data"`
	}{1, strconv.FormatUint(w.seq+1, 10), kind, time.Now().UTC().Format(time.RFC3339Nano), data}
	raw, e := json.Marshal(frame)
	if e != nil {
		return e
	}
	if len(raw) > 1<<20 {
		return errors.New("event exceeds byte budget")
	}
	raw = append(raw, '\n')
	n, e := w.out.Write(raw)
	if e == nil && n != len(raw) {
		e = io.ErrShortWrite
	}
	if e != nil {
		w.err = e
		return e
	}
	w.seq++
	return nil
}
