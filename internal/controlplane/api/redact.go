package api

import (
	"encoding/json"
	"github.com/postfix/cctvscan/internal/controlplane/privacy"
)

func redact(raw json.RawMessage) json.RawMessage { return privacy.Sanitize(raw) }
