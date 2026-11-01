package store

import (
	"context"
)

// AttemptSecrets returns ciphertext only to the trusted worker. It does not
// expose secret material through public resource metadata DTOs.
func (s *Store) AttemptSecrets(ctx context.Context, a Attempt) (proxy, credentials []byte, err error) {
	if _, err = s.ActiveAttempt(ctx, a); err != nil {
		return
	}
	if a.Job.Config.ProxyVersionID != "" {
		err = s.pool.QueryRow(ctx, `SELECT sealed_url FROM proxy_versions WHERE id=$1`, a.Job.Config.ProxyVersionID).Scan(&proxy)
		if err != nil {
			return
		}
	}
	if a.Job.Config.CheckCredentials {
		err = s.pool.QueryRow(ctx, `SELECT sealed_credentials FROM credential_sets WHERE id=$1`, a.Job.Config.CredentialSetID).Scan(&credentials)
	}
	return
}
