package registryauth

import (
	"context"

	"github.com/google/uuid"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// DBStore implements Store using sqlc queries.
type DBStore struct {
	Q         *db.Queries
	MasterKey []byte
}

func (s DBStore) ListRegistryAuth(ctx context.Context, clusterID uuid.UUID) ([]ConfiguredAuth, error) {
	rows, err := s.Q.ListRegistryAuth(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	out := make([]ConfiguredAuth, 0, len(rows))
	for _, row := range rows {
		cfg := ConfiguredAuth{
			Target:            row.Target,
			Kind:              row.Kind,
			Method:            row.Method,
			SecretNamespace:   row.SecretNamespace.String,
			SecretName:        row.SecretName.String,
			SecretUsernameKey: row.SecretUsernameKey.String,
			SecretPasswordKey: row.SecretPasswordKey.String,
		}
		if row.Method == "basic" {
			user, pass, err := DecryptConfigured(s.MasterKey,
				row.UsernameEnc, row.UsernameNonce, row.PasswordEnc, row.PasswordNonce)
			if err != nil {
				return nil, err
			}
			cfg.Username = user
			cfg.Password = pass
		}
		out = append(out, cfg)
	}
	return out, nil
}
