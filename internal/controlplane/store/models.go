// Package store owns durable control-plane data. No scans or workers are started.
package store

import (
	"encoding/json"
	"errors"
	"github.com/postfix/cctvscan/internal/controlplane/config"
	"time"
)

var ErrNotFound = errors.New("job not found")
var ErrInvalidInput = errors.New("invalid job input")

// ConflictError is an idempotency conflict; HTTP adapters should map it to 409.
type ConflictError struct{}

func (*ConflictError) Error() string   { return "idempotency key already used for a different request" }
func (*ConflictError) StatusCode() int { return 409 }

type JobDTO struct {
	ID                string            `json:"id"`
	Name              string            `json:"name"`
	State             string            `json:"state"`
	Phase             string            `json:"phase"`
	Config            config.ScanConfig `json:"config"`
	Priority          int               `json:"priority"`
	CreatedAt         time.Time         `json:"created_at"`
	StartedAt         *time.Time        `json:"started_at"`
	FinishedAt        *time.Time        `json:"finished_at"`
	Error             string            `json:"error"`
	ResultCount       int64             `json:"result_count"`
	CameraCount       int64             `json:"camera_count"`
	StreamCount       int64             `json:"stream_count"`
	InconclusiveCount int64             `json:"inconclusive_count"`
	CancelRequested   bool              `json:"cancel_requested"`
	Attempt           int               `json:"attempt"`
	RetryOf           *string           `json:"retry_of"`
}

// CreateJobInput accepts only secret references in Config. Policy must be supplied
// by trusted server configuration. Reference resolution is reserved for B2/API.
// Priority uses River's range 1 (highest) through 4. Zero is invalid.
// RetryOf may refer only to a terminal job. QueueScheduledAt nil means now.
type CreateJobInput struct {
	Name             string
	Config           config.ScanConfig
	Policy           config.Policy
	Priority         int
	IdempotencyKey   string
	QueueScheduledAt *time.Time
	RetryOf          *string
}

type ListJobsInput struct {
	Search string
	State  string
	Limit  int
	Offset int
}
type Event struct {
	Seq       int64           `json:"seq,string"`
	JobID     string          `json:"job_id"`
	Kind      string          `json:"kind"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// ScanJobArgs is the stable River payload consumed by the future scanner worker.
// Load the immutable configuration from GetJob, never put resolved secrets here.
type ScanJobArgs struct {
	JobID string `json:"job_id"`
}

func (ScanJobArgs) Kind() string { return "controlplane_scan" }
