package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
)

// Store handles all Postgres interactions.
type Store struct {
	db *sql.DB
}

// New opens a Postgres connection and pings it.
func New(databaseURL string) (*Store, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("store: ping: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	return &Store{db: db}, nil
}

// Close closes the underlying DB pool.
func (s *Store) Close() error { return s.db.Close() }

// DB exposes the underlying sql.DB for testing.
func (s *Store) DB() *sql.DB { return s.db }

// Ping checks DB liveness.
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// UpsertAttempts inserts events, ignoring duplicates by event_id.
func (s *Store) UpsertAttempts(ctx context.Context, attempts []event.TestAttempt) error {
	if len(attempts) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO test_case_attempts (
			event_id, repo, suite, framework, env,
			branch, commit_sha, run_id, run_attempt,
			test_id, test_name, status, duration_ms, attempt_index,
			failure_category, failure_fingerprint, failure_message_excerpt,
			artifact_url, pr_number, started_at
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,
			$10,$11,$12,$13,$14,
			$15,$16,$17,
			$18,$19,$20
		) ON CONFLICT (event_id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("store: prepare: %w", err)
	}
	defer stmt.Close()

	for _, a := range attempts {
		_, err := stmt.ExecContext(ctx,
			a.EventID, a.Repo, a.Suite, a.Framework, a.Env,
			nullStr(a.Branch), nullStr(a.CommitSHA), a.RunID, a.RunAttempt,
			a.TestID, nullStr(a.TestName), a.Status, a.DurationMS, a.AttemptIndex,
			nullStr(a.FailureCategory), nullStr(a.FailureFingerprint), nullStr(a.FailureMessageExcerpt),
			nullStr(a.ArtifactURL), nullInt(a.PRNumber), a.StartedAt,
		)
		if err != nil {
			return fmt.Errorf("store: insert %s: %w", a.EventID, err)
		}
	}
	return tx.Commit()
}

// HistorySummary holds aggregated statistics for one test over a window.
type HistorySummary struct {
	Repo          string
	Suite         string
	Env           string
	TestID        string
	Window        string
	Attempts      int
	Passed        int
	Failed        int
	Skipped       int
	FailureRate   float64
	P95DurationMS int
	LastFailedAt  *time.Time
	LastPassedAt  *time.Time
}

// QueryHistory returns aggregated history for one test over a window (7d, 30d, 90d).
func (s *Store) QueryHistory(ctx context.Context, repo, suite, env, testID, window string) (*HistorySummary, error) {
	interval, err := windowToInterval(window)
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRowContext(ctx, `
		SELECT
			count(*) AS attempts,
			count(*) FILTER (WHERE status = 'passed') AS passed,
			count(*) FILTER (WHERE status = 'failed') AS failed,
			count(*) FILTER (WHERE status = 'skipped') AS skipped,
			COALESCE(
				100.0 * count(*) FILTER (WHERE status = 'failed') / NULLIF(count(*), 0), 0
			) AS failure_rate,
			COALESCE(
				percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms), 0
			) AS p95_duration_ms,
			max(started_at) FILTER (WHERE status = 'failed') AS last_failed_at,
			max(started_at) FILTER (WHERE status = 'passed') AS last_passed_at
		FROM test_case_attempts
		WHERE repo = $1 AND suite = $2 AND env = $3 AND test_id = $4
		  AND started_at >= now() - `+interval,
		repo, suite, env, testID,
	)

	hs := &HistorySummary{Repo: repo, Suite: suite, Env: env, TestID: testID, Window: window}
	var p95 float64
	if err := row.Scan(
		&hs.Attempts, &hs.Passed, &hs.Failed, &hs.Skipped,
		&hs.FailureRate, &p95, &hs.LastFailedAt, &hs.LastPassedAt,
	); err != nil {
		return nil, fmt.Errorf("store: query history: %w", err)
	}
	hs.P95DurationMS = int(p95)
	return hs, nil
}

// TrendBucket is one day's aggregated stats.
type TrendBucket struct {
	Date          time.Time
	Attempts      int
	Passed        int
	Failed        int
	FailureRate   float64
	P95DurationMS int
}

// QueryTrends returns day-bucketed failure rate and p95 duration for a suite.
func (s *Store) QueryTrends(ctx context.Context, repo, suite, env, window string) ([]TrendBucket, error) {
	interval, err := windowToInterval(window)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			date_trunc('day', started_at) AS day,
			count(*) AS attempts,
			count(*) FILTER (WHERE status = 'passed') AS passed,
			count(*) FILTER (WHERE status = 'failed') AS failed,
			COALESCE(
				100.0 * count(*) FILTER (WHERE status = 'failed') / NULLIF(count(*), 0), 0
			) AS failure_rate,
			COALESCE(
				percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_ms), 0
			) AS p95_duration_ms
		FROM test_case_attempts
		WHERE repo = $1 AND suite = $2 AND env = $3
		  AND started_at >= now() - `+interval+`
		GROUP BY 1
		ORDER BY 1`,
		repo, suite, env,
	)
	if err != nil {
		return nil, fmt.Errorf("store: query trends: %w", err)
	}
	defer rows.Close()

	var out []TrendBucket
	for rows.Next() {
		var b TrendBucket
		var p95 float64
		if err := rows.Scan(&b.Date, &b.Attempts, &b.Passed, &b.Failed, &b.FailureRate, &p95); err != nil {
			return nil, err
		}
		b.P95DurationMS = int(p95)
		out = append(out, b)
	}
	return out, rows.Err()
}

func windowToInterval(w string) (string, error) {
	switch w {
	case "7d", "":
		return "interval '7 days'", nil
	case "30d":
		return "interval '30 days'", nil
	case "90d":
		return "interval '90 days'", nil
	default:
		return "", fmt.Errorf("store: unsupported window %q (use 7d, 30d, or 90d)", w)
	}
}

func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

func nullInt(i int) sql.NullInt32 {
	return sql.NullInt32{Int32: int32(i), Valid: i != 0}
}
