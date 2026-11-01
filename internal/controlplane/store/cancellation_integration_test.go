//go:build integration

package store

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCancellationDurableReplay(t *testing.T) {
	s, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Include the queue-claim race: River is running while the app still says queued.
	for _, initial := range []string{"queued", "running", "canceling", "claimed"} {
		t.Run(initial, func(t *testing.T) {
			j, err := s.CreateJob(ctx, input())
			if err != nil {
				t.Fatal(err)
			}
			if initial != "queued" {
				state := initial
				if state == "claimed" {
					state = "queued"
				}
				if _, err = s.pool.Exec(ctx, `UPDATE jobs SET state=$2 WHERE id=$1`, j.ID, state); err != nil {
					t.Fatal(err)
				}
				if _, err = s.pool.Exec(ctx, `UPDATE river_job SET state='running',attempted_at=now() WHERE args->>'job_id'=$1`, j.ID); err != nil {
					t.Fatal(err)
				}
			}
			token := uuid.NewString()
			attempt := uuid.NewString()
			if _, err = s.pool.Exec(ctx, `INSERT INTO attempts(id,job_id,number,fencing_token,state) VALUES($1,$2,1,$3,'running')`, attempt, j.ID, token); err != nil {
				t.Fatal(err)
			}
			before, err := s.GetEvents(ctx, j.ID, 0, 100)
			if err != nil {
				t.Fatal(err)
			}
			cursor := before[len(before)-1].Seq
			want := "canceling"
			if initial == "queued" {
				want = "canceled"
			}
			got, err := s.CancelQueued(ctx, j.ID)
			if err != nil || got.State != want || !got.CancelRequested {
				t.Fatalf("cancel: %+v %v", got, err)
			}
			events, err := s.GetEvents(ctx, j.ID, cursor, 100)
			if err != nil || len(events) != 1 {
				t.Fatalf("committed cancellation must have exactly one event: %+v %v", events, err)
			}
			var data struct {
				State           string `json:"state"`
				CancelRequested bool   `json:"cancel_requested"`
			}
			if err = json.Unmarshal(events[0].Data, &data); err != nil || events[0].Kind != "state" || data.State != want || !data.CancelRequested {
				t.Fatalf("state event: %+v %+v %v", events[0], data, err)
			}
			for _, filter := range []string{"", j.ID} {
				replay, err := s.GlobalEvents(ctx, filter, cursor, 500)
				if err != nil {
					t.Fatal(err)
				}
				count := 0
				for _, event := range replay {
					if event.JobID == j.ID {
						count++
						if event.Seq != events[0].Seq {
							t.Fatal("replay sequence differs")
						}
					}
				}
				if count != 1 {
					t.Fatalf("global/per-job replay %q count=%d", filter, count)
				}
			}
			again, err := s.CancelQueued(ctx, j.ID)
			if err != nil || again.State != want {
				t.Fatalf("repeat: %+v %v", again, err)
			}
			repeated, err := s.GetEvents(ctx, j.ID, cursor, 100)
			if err != nil || len(repeated) != 1 {
				t.Fatalf("repeat manufactured events: %+v %v", repeated, err)
			}
			var actualToken, attemptState string
			if err = s.pool.QueryRow(ctx, `SELECT fencing_token::text,state FROM attempts WHERE id=$1`, attempt).Scan(&actualToken, &attemptState); err != nil || actualToken != token || attemptState != "running" {
				t.Fatal("attempt fencing changed", err)
			}
			if initial != "queued" && got.FinishedAt != nil {
				t.Fatal("claimed/running cancellation became terminal")
			}
		})
	}
}
