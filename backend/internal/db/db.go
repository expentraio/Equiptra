package db

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Connect(ctx context.Context) (*pgxpool.Pool, error) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		// No local-dev fallback here on purpose. This is shared by the
		// long-running API server and by one-off write scripts
		// (cmd/migrate, cmd/migrate-photos) — a silent default is a real
		// hazard for the latter: it's entirely possible for a script to
		// write real data to a correctly-configured remote system (e.g.
		// Supabase Storage) while its DB writes silently land on local
		// dev instead, with no error, because nothing ties the two
		// together. That happened for real during the photo backfill —
		// see README's "One-off backfill" section. Failing loudly here
		// costs a local dev one extra explicit env var; it costs a
		// misdirected script nothing less than a wasted production
		// operation and a silent data gap.
		return nil, fmt.Errorf("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return pool, nil
}
