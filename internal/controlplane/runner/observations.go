package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

func readObservations(ctx context.Context, r io.Reader, callback func(context.Context, json.RawMessage) error) error {
	scan := bufio.NewScanner(r)
	scan.Buffer(make([]byte, 64<<10), (1<<20)+1)
	for scan.Scan() {
		if e := ctx.Err(); e != nil {
			return e
		}
		if !json.Valid(scan.Bytes()) {
			return errors.New("invalid scanner observation")
		}
		raw := append(json.RawMessage(nil), scan.Bytes()...)
		bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
		e := callback(bounded, raw)
		cancel()
		if e != nil {
			return errors.New("scanner observation could not be persisted")
		}
	}
	return scan.Err()
}
