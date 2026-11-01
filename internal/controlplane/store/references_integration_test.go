//go:build integration
package store
import("context";"testing")
func TestSubmissionRejectsMissingReference(t *testing.T){s,_:=fixture(t);in:=input();in.Config.ProxyVersionID="00000000-0000-4000-8000-000000000001";j,e:=s.CreateJob(context.Background(),in);if e==nil{t.Fatalf("missing proxy reference accepted job %s",j.ID)}}
