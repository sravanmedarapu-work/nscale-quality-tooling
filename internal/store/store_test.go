//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func setupDB(t *testing.T) (*store.Store, func()) {
	t.Helper()
	ctx := context.Background()

	pgc, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("test_history"),
		tcpostgres.WithUsername("test_history"),
		tcpostgres.WithPassword("test_history"),
		tcpostgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	s, err := store.New(dsn)
	require.NoError(t, err)

	migration, err := os.ReadFile("../../migrations/001_initial_schema.sql")
	require.NoError(t, err)
	_, err = s.DB().ExecContext(ctx, string(migration))
	require.NoError(t, err)

	return s, func() {
		s.Close()
		pgc.Terminate(ctx) //nolint:errcheck
	}
}

func TestUpsertAttempts_idempotent(t *testing.T) {
	s, cleanup := setupDB(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	attempts := []event.TestAttempt{
		{
			EventID: "evt-001", Repo: "org/repo", Suite: "s", Framework: "playwright",
			Env: "dev", Branch: "main", RunID: "1", RunAttempt: 1,
			TestID: "file::test1", Status: event.StatusPassed, DurationMS: 500, StartedAt: now,
		},
	}

	require.NoError(t, s.UpsertAttempts(ctx, attempts))
	require.NoError(t, s.UpsertAttempts(ctx, attempts), "replay must not error")

	var count int
	s.DB().QueryRowContext(ctx, `SELECT count(*) FROM test_case_attempts WHERE event_id = 'evt-001'`).Scan(&count)
	assert.Equal(t, 1, count, "idempotent: duplicate event_id must not create two rows")
}

func TestQueryHistory(t *testing.T) {
	s, cleanup := setupDB(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	attempts := []event.TestAttempt{
		{
			EventID: "h1", Repo: "org/repo", Suite: "s", Framework: "playwright", Env: "dev",
			RunID: "run-1", RunAttempt: 1, TestID: "f::t",
			Status: event.StatusPassed, DurationMS: 400,
			CommitSHA: "commit-abc",
			StartedAt: now.Add(-time.Minute),
		},
		{
			EventID: "h2", Repo: "org/repo", Suite: "s", Framework: "playwright", Env: "dev",
			RunID: "run-2", RunAttempt: 1, TestID: "f::t",
			Status: event.StatusFailed, DurationMS: 900,
			CommitSHA: "commit-def", FailureMessageExcerpt: "panic: nil pointer",
			StartedAt: now,
		},
	}
	require.NoError(t, s.UpsertAttempts(ctx, attempts))

	hs, err := s.QueryHistory(ctx, "org/repo", "s", "dev", "f::t", "30d")
	require.NoError(t, err)
	assert.Equal(t, 2, hs.Attempts)
	assert.Equal(t, 1, hs.Passed)
	assert.Equal(t, 1, hs.Failed)
	assert.InDelta(t, 50.0, hs.FailureRate, 0.1)

	// Last failure message and most-recent commit SHA are surfaced.
	require.NotNil(t, hs.LastFailureExcerpt)
	assert.Equal(t, "panic: nil pointer", *hs.LastFailureExcerpt)
	require.NotNil(t, hs.LastCommitSHA)
	assert.Equal(t, "commit-def", *hs.LastCommitSHA, "most recent run's commit SHA")
}

func TestQueryTrends(t *testing.T) {
	s, cleanup := setupDB(t)
	defer cleanup()
	ctx := context.Background()

	now := time.Now().UTC().Truncate(time.Millisecond)
	attempts := []event.TestAttempt{
		{EventID: "t1", Repo: "org/repo", Suite: "s", Framework: "playwright", Env: "dev",
			RunID: "1", RunAttempt: 1, TestID: "f::t", Status: event.StatusPassed, DurationMS: 300, StartedAt: now},
		{EventID: "t2", Repo: "org/repo", Suite: "s", Framework: "playwright", Env: "dev",
			RunID: "2", RunAttempt: 1, TestID: "f::t", Status: event.StatusFailed, DurationMS: 800, StartedAt: now},
	}
	require.NoError(t, s.UpsertAttempts(ctx, attempts))

	buckets, err := s.QueryTrends(ctx, "org/repo", "s", "dev", "7d")
	require.NoError(t, err)
	require.Len(t, buckets, 1)
	assert.Equal(t, 2, buckets[0].Attempts)
}
