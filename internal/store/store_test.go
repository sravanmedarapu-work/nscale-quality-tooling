//go:build integration

package store_test

import (
	"context"
	"os"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestStoreSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store Integration Suite")
}

func setupDB(ctx context.Context) *store.Store {
	pgc, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("test_history"),
		tcpostgres.WithUsername("test_history"),
		tcpostgres.WithPassword("test_history"),
		tcpostgres.BasicWaitStrategies(),
	)
	Expect(err).NotTo(HaveOccurred())

	dsn, err := pgc.ConnectionString(ctx, "sslmode=disable")
	Expect(err).NotTo(HaveOccurred())

	s, err := store.New(dsn)
	Expect(err).NotTo(HaveOccurred())

	migration, err := os.ReadFile("../../migrations/001_initial_schema.sql")
	Expect(err).NotTo(HaveOccurred())
	_, err = s.DB().ExecContext(ctx, string(migration))
	Expect(err).NotTo(HaveOccurred())

	DeferCleanup(func() {
		s.Close()
		pgc.Terminate(ctx) //nolint:errcheck
	})

	return s
}

var _ = Describe("Store", func() {
	var (
		s   *store.Store
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		s = setupDB(ctx)
	})

	Describe("UpsertAttempts", func() {
		It("is idempotent — replaying the same event does not create a duplicate row", func() {
			now := time.Now().UTC().Truncate(time.Millisecond)
			attempts := []event.TestAttempt{
				{
					EventID: "evt-001", Repo: "org/repo", Suite: "s", Framework: "playwright",
					Env: "dev", Branch: "main", RunID: "1", RunAttempt: 1,
					TestID: "file::test1", Status: event.StatusPassed, DurationMS: 500, StartedAt: now,
				},
			}

			Expect(s.UpsertAttempts(ctx, attempts)).To(Succeed())
			Expect(s.UpsertAttempts(ctx, attempts)).To(Succeed(), "replay must not error")

			var count int
			s.DB().QueryRowContext(ctx, `SELECT count(*) FROM test_case_attempts WHERE event_id = 'evt-001'`).Scan(&count)
			Expect(count).To(Equal(1), "idempotent: duplicate event_id must not create two rows")
		})
	})

	Describe("QueryHistory", func() {
		It("returns correct aggregate statistics", func() {
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
			Expect(s.UpsertAttempts(ctx, attempts)).To(Succeed())

			hs, err := s.QueryHistory(ctx, "org/repo", "s", "dev", "f::t", "30d")
			Expect(err).NotTo(HaveOccurred())
			Expect(hs.Attempts).To(Equal(2))
			Expect(hs.Passed).To(Equal(1))
			Expect(hs.Failed).To(Equal(1))
			Expect(hs.FailureRate).To(BeNumerically("~", 50.0, 0.1))

			// Last failure message and most-recent commit SHA are surfaced.
			Expect(hs.LastFailureExcerpt).NotTo(BeNil())
			Expect(*hs.LastFailureExcerpt).To(Equal("panic: nil pointer"))
			Expect(hs.LastCommitSHA).NotTo(BeNil())
			Expect(*hs.LastCommitSHA).To(Equal("commit-def"), "most recent run's commit SHA")
		})
	})

	Describe("QueryTrends", func() {
		It("groups attempts into buckets", func() {
			now := time.Now().UTC().Truncate(time.Millisecond)
			attempts := []event.TestAttempt{
				{EventID: "t1", Repo: "org/repo", Suite: "s", Framework: "playwright", Env: "dev",
					RunID: "1", RunAttempt: 1, TestID: "f::t", Status: event.StatusPassed, DurationMS: 300, StartedAt: now},
				{EventID: "t2", Repo: "org/repo", Suite: "s", Framework: "playwright", Env: "dev",
					RunID: "2", RunAttempt: 1, TestID: "f::t", Status: event.StatusFailed, DurationMS: 800, StartedAt: now},
			}
			Expect(s.UpsertAttempts(ctx, attempts)).To(Succeed())

			buckets, err := s.QueryTrends(ctx, "org/repo", "s", "dev", "7d")
			Expect(err).NotTo(HaveOccurred())
			Expect(buckets).To(HaveLen(1))
			Expect(buckets[0].Attempts).To(Equal(2))
		})
	})
})
