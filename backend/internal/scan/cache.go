package scan

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/banshee86vr/omastx/backend/internal/store/db"
)

// dbCache implements oci.Cache backed by the latest_cache table. It stores the raw
// registry tag listing (the expensive, rate-limited call) so repeated scans and
// different installed tags of the same image reuse one upstream fetch (SPEC §2.2).
type dbCache struct {
	q *db.Queries
}

func (c dbCache) GetTags(ctx context.Context, identity, kind string) ([]string, bool, error) {
	row, err := c.q.GetLatestCache(ctx, db.GetLatestCacheParams{Identity: identity, Kind: kind})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var tags []string
	if len(row.Candidates) > 0 {
		if err := json.Unmarshal(row.Candidates, &tags); err != nil {
			return nil, false, nil
		}
	}
	return tags, row.Fresh, nil
}

func (c dbCache) PutTags(ctx context.Context, identity, kind, latest string, tags []string, ttl time.Duration) error {
	candidates, err := json.Marshal(tags)
	if err != nil {
		return err
	}
	return c.q.UpsertLatestCache(ctx, db.UpsertLatestCacheParams{
		Identity:      identity,
		Kind:          kind,
		LatestVersion: pgText(latest),
		Candidates:    candidates,
		Ttl:           pgInterval(ttl),
	})
}

func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func pgInterval(d time.Duration) pgtype.Interval {
	return pgtype.Interval{Microseconds: d.Microseconds(), Valid: true}
}

func pgInt4(n int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(n), Valid: true}
}
