package runner

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/postfix/cctvscan/internal/controlplane/privacy"
	"github.com/postfix/cctvscan/internal/controlplane/store"
	"strconv"
)

func decodeHost(a store.Attempt, raw json.RawMessage) (row store.ResultInput, files []string, partial bool, e error) {
	var h struct {
		Host, Brand, State, CredentialState string
		CameraDetection                     struct{ IsCamera bool }
		StreamResults                       []struct {
			State     string
			Verified  bool
			SavedPath string
		}
	}
	if len(raw) > 1<<20 || json.Unmarshal(raw, &h) != nil || !allowedHost(h.Host, a.Job.Config.Targets) {
		e = errors.New("invalid host evidence")
		return
	}
	if h.State != "complete" && h.State != "inconclusive" && h.State != "canceled" {
		e = errors.New("invalid host outcome")
		return
	}
	streams := 0
	for _, s := range h.StreamResults {
		if s.Verified && (s.State == "media_header" || s.State == "frame_decoded") {
			streams++
			if a.Job.Config.Capture && s.SavedPath != "" {
				files = append(files, s.SavedPath)
			}
		}
	}
	partial = h.State != "complete" || h.CredentialState == "inconclusive"
	row = store.ResultInput{Target: h.Host, Brand: h.Brand, State: h.State, Data: privacy.Sanitize(raw), Camera: h.CameraDetection.IsCamera, Streams: streams, Inconclusive: partial}
	return
}
func (w *Worker) observe(a store.Attempt, directory string) func(context.Context, json.RawMessage) error {
	var previous uint64
	return func(ctx context.Context, raw json.RawMessage) error {
		var frame struct {
			Version int             `json:"schema_version"`
			Seq     string          `json:"seq"`
			Kind    string          `json:"kind"`
			Data    json.RawMessage `json:"data"`
		}
		if json.Unmarshal(raw, &frame) != nil || frame.Version != 1 {
			return errors.New("invalid observation version")
		}
		seq, e := strconv.ParseUint(frame.Seq, 10, 64)
		if e != nil || seq != previous+1 {
			return errors.New("invalid observation sequence")
		}
		previous = seq
		switch frame.Kind {
		case "phase":
			var data struct {
				Phase string `json:"phase"`
			}
			if json.Unmarshal(frame.Data, &data) != nil {
				return errors.New("invalid phase")
			}
			switch data.Phase {
			case "port_discovery", "host_processing", "stopping":
				return w.Store.Phase(ctx, a, data.Phase)
			default:
				return errors.New("unrecognized phase")
			}
		case "host_result":
			row, files, _, e := decodeHost(a, frame.Data)
			if e != nil {
				return e
			}
			for _, path := range files {
				artifact, e := inspectCapture(directory, a.JobID, a.ID, path)
				if e != nil {
					return e
				}
				if e = w.Store.SaveArtifacts(ctx, a, []store.ArtifactInput{artifact}); e != nil {
					return e
				}
			}
			return w.Store.SaveResults(ctx, a, []store.ResultInput{row})
		default:
			return errors.New("unrecognized observation")
		}
	}
}
