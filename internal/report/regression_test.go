package report

import (
	"bytes"
	"testing"
)

func TestCredentialValuesNotSerialized(t *testing.T) {
	r := TargetResult{Host: "fixture.invalid", FoundCred: "fixture:synthetic-secret"}
	if bytes.Contains(r.JSON(), []byte("synthetic-secret")) {
		t.Fatal("credential value exposed in report")
	}
}
