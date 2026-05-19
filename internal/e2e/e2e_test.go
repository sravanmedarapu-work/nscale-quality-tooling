//go:build e2e

// Package e2e_test exercises the full ingest→store→query lifecycle against a real
// Postgres. It supports two modes:
//
//   - Black-box mode (CI):  TEST_HISTORY_API_URL points at the real running binary.
//     A fresh devstack (postgres service container + API binary subprocess) is
//     created by the CI workflow for every execution. No Docker inside the test.
//
//   - In-process mode (local dev):  TEST_HISTORY_API_URL is unset. testcontainers
//     spins up Postgres and the handler is served via httptest.Server, giving a
//     fast feedback loop without needing a running binary.
package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	_ "github.com/lib/pq"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/nscale-test-history/ingest"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/test-history-api/handler"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
)

func TestE2ESuite(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "E2E Suite", suiteConfig, reporterConfig)
}

const (
	e2eSecret = "e2e-secret"
	e2eRepo   = "org/e2e-repo"
)

// srvURL and e2eToken are set in BeforeSuite. All helpers and tests use these
// variables so they work in both black-box (real binary) and in-process modes.
var (
	srvURL      string
	e2eToken    string
	fixturesDir string
	moduleRoot  string
)

// ---- response types mirroring handler output (no direct internal imports) ----

type historySummary struct {
	Repo               string     `json:"Repo"`
	Suite              string     `json:"Suite"`
	Env                string     `json:"Env"`
	TestID             string     `json:"TestID"`
	Window             string     `json:"Window"`
	Attempts           int        `json:"Attempts"`
	Passed             int        `json:"Passed"`
	Failed             int        `json:"Failed"`
	Skipped            int        `json:"Skipped"`
	FailureRate        float64    `json:"FailureRate"`
	P95DurationMS      int        `json:"P95DurationMS"`
	LastFailedAt       *time.Time `json:"LastFailedAt"`
	LastPassedAt       *time.Time `json:"LastPassedAt"`
	LastFailureExcerpt *string    `json:"LastFailureExcerpt"`
	LastCommitSHA      *string    `json:"LastCommitSHA"`
}

type trendBucket struct {
	Date          time.Time `json:"Date"`
	Attempts      int       `json:"Attempts"`
	Passed        int       `json:"Passed"`
	Failed        int       `json:"Failed"`
	FailureRate   float64   `json:"FailureRate"`
	P95DurationMS int       `json:"P95DurationMS"`
}

type trendsResponse struct {
	Repo    string        `json:"repo"`
	Suite   string        `json:"suite"`
	Env     string        `json:"env"`
	Window  string        `json:"window"`
	Buckets []trendBucket `json:"buckets"`
}

// ---- BeforeSuite / AfterSuite -----------------------------------------------

var stopSrv func()
var stopDB func()
var e2eSt *store.Store

var _ = BeforeSuite(func() {
	_, filename, _, _ := runtime.Caller(0)
	moduleRoot = filepath.Join(filepath.Dir(filename), "..", "..")
	fixturesDir = filepath.Join(moduleRoot, "testdata", "fixtures")

	// Black-box mode: CI has already started a fresh devstack (postgres +
	// API binary). Just point at it and run.
	if externalURL := os.Getenv("TEST_HISTORY_API_URL"); externalURL != "" {
		srvURL = externalURL
		e2eToken = os.Getenv("TEST_HISTORY_TOKEN")
		if e2eToken == "" {
			e2eToken = e2eSecret
		}
		GinkgoWriter.Printf("e2e mode: black-box (external API at %s)\n", srvURL)
		return
	}

	GinkgoWriter.Printf("e2e mode: in-process (testcontainers + httptest.Server)\n")
	// In-process mode: spin up postgres (testcontainers) + httptest.Server.
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")

	if dbURL == "" {
		var err error
		dbURL, stopDB, err = startPostgres(ctx)
		Expect(err).NotTo(HaveOccurred(), "e2e: start postgres")
	} else {
		stopDB = func() {}
	}

	err := applyMigration(dbURL, filepath.Join(moduleRoot, "migrations", "001_initial_schema.sql"))
	Expect(err).NotTo(HaveOccurred(), "e2e: migrate")

	e2eSt, err = store.New(dbURL)
	Expect(err).NotTo(HaveOccurred(), "e2e: store")

	e2eToken = e2eSecret
	srv := httptest.NewServer(handler.New(e2eSt, e2eToken))
	srvURL = srv.URL
	stopSrv = srv.Close
})

var _ = AfterSuite(func() {
	if stopSrv != nil {
		stopSrv()
	}
	if e2eSt != nil {
		e2eSt.Close()
	}
	if stopDB != nil {
		stopDB()
	}
})

func startPostgres(ctx context.Context) (dsn string, stop func(), err error) {
	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("test_history"),
		tcpostgres.WithUsername("test_history"),
		tcpostgres.WithPassword("test_history"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		return "", nil, err
	}
	dsn, err = c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		c.Terminate(ctx) //nolint:errcheck
		return "", nil, err
	}
	return dsn, func() { c.Terminate(context.Background()) }, nil //nolint:errcheck
}

func applyMigration(dbURL, path string) error {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return err
	}
	defer db.Close()
	migration, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = db.Exec(string(migration))
	return err
}

// ---- helpers ----------------------------------------------------------------

// suiteName returns a unique, DB-safe suite name per spec to prevent cross-test
// contamination in the shared Postgres instance.
func suiteName() string {
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, CurrentSpecReport().FullText())
}

// runID returns a unique run ID for each call (nanosecond timestamp).
func runID() string {
	return fmt.Sprintf("e2e-%d", time.Now().UnixNano())
}

// ingestFixture calls ingest.Run with the provided fixture files, wiring in the
// shared test server. It changes to a temp dir to isolate spool files.
func ingestFixture(suite, framework, env, jsonPath, junitPath string) {
	dir := GinkgoT().TempDir()
	oldDir, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(os.Chdir, oldDir)
	Expect(os.Chdir(dir)).To(Succeed())

	GinkgoT().Setenv("GITHUB_REPOSITORY", e2eRepo)
	GinkgoT().Setenv("GITHUB_RUN_ID", runID())
	GinkgoT().Setenv("GITHUB_RUN_ATTEMPT", "1")
	GinkgoT().Setenv("GITHUB_REF_NAME", "main")
	GinkgoT().Setenv("GITHUB_SHA", "e2esha")
	GinkgoT().Setenv("TEST_HISTORY_API_URL", srvURL)
	GinkgoT().Setenv("TEST_HISTORY_TOKEN", e2eToken)

	args := []string{"--suite", suite, "--framework", framework, "--env", env}
	if jsonPath != "" {
		args = append(args, "--json", jsonPath)
	}
	if junitPath != "" {
		args = append(args, "--junit", junitPath)
	}
	GinkgoWriter.Printf("ingesting fixture: suite=%s framework=%s env=%s\n", suite, framework, env)
	ingest.Run(args)
}

func doGet(path string) *http.Response {
	req, err := http.NewRequest(http.MethodGet, srvURL+path, nil)
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Authorization", "Bearer "+e2eToken)
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func doGetNoAuth(path string) *http.Response {
	req, err := http.NewRequest(http.MethodGet, srvURL+path, nil)
	Expect(err).NotTo(HaveOccurred())
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func doPost(path string, body any, token string) *http.Response {
	b, err := json.Marshal(body)
	Expect(err).NotTo(HaveOccurred())
	req, err := http.NewRequest(http.MethodPost, srvURL+path, strings.NewReader(string(b)))
	Expect(err).NotTo(HaveOccurred())
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	Expect(err).NotTo(HaveOccurred())
	return resp
}

func historyURL(repo, suite, env, testID, window string) string {
	return "/v1/tests/history?" + url.Values{
		"repo": {repo}, "suite": {suite}, "env": {env},
		"test_id": {testID}, "window": {window},
	}.Encode()
}

func trendsURL(repo, suite, env, window string) string {
	return "/v1/tests/trends?" + url.Values{
		"repo": {repo}, "suite": {suite}, "env": {env}, "window": {window},
	}.Encode()
}

func decodeHistory(resp *http.Response) historySummary {
	defer resp.Body.Close()
	var hs historySummary
	Expect(json.NewDecoder(resp.Body).Decode(&hs)).To(Succeed())
	return hs
}

func decodeTrends(resp *http.Response) trendsResponse {
	defer resp.Body.Close()
	var tr trendsResponse
	Expect(json.NewDecoder(resp.Body).Decode(&tr)).To(Succeed())
	return tr
}

func totalAttempts(tr trendsResponse) int {
	n := 0
	for _, b := range tr.Buckets {
		n += b.Attempts
	}
	return n
}

// ============================================================================
// API contract tests
// ============================================================================

var _ = Describe("API contract", func() {
	Describe("GET /healthz", func() {
		It("returns 200 with status ok and db ok when DB is up", func() {
			resp := doGet("/healthz")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var body map[string]string
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["status"]).To(Equal("ok"))
			Expect(body["db"]).To(Equal("ok"))
		})
	})

	Describe("Authentication", func() {
		Context("no token on all endpoints", func() {
			type endpointCase struct {
				method string
				path   string
			}
			endpoints := []endpointCase{
				{"POST", "/v1/runs/ingest"},
				{"GET", "/v1/tests/history?repo=r&suite=s&env=e&test_id=t"},
				{"GET", "/v1/tests/trends?repo=r&suite=s&env=e"},
			}
			for _, tc := range endpoints {
				tc := tc
				It("returns 401 for "+tc.method+" "+tc.path, func() {
					var resp *http.Response
					if tc.method == "POST" {
						resp = doPost(tc.path, map[string]any{"events": []any{}}, "")
					} else {
						resp = doGetNoAuth(tc.path)
					}
					defer resp.Body.Close()
					Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
				})
			}
		})

		It("returns 401 for wrong token on POST /v1/runs/ingest", func() {
			resp := doPost("/v1/runs/ingest", map[string]any{"events": []any{}}, "bad-token")
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
		})
	})

	Describe("POST /v1/runs/ingest", func() {
		It("returns 400 for an empty events array", func() {
			resp := doPost("/v1/runs/ingest", map[string]any{"events": []any{}}, e2eToken)
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
		})

		It("returns 400 and names event_id in the error when event_id is missing", func() {
			resp := doPost("/v1/runs/ingest", map[string]any{
				"events": []any{map[string]any{
					// event_id deliberately omitted
					"repo": e2eRepo, "suite": "s", "framework": "ginkgo",
					"env": "dev", "run_id": "1", "run_attempt": 1,
					"test_id": "t::test", "status": "passed",
					"started_at": time.Now(),
				}},
			}, e2eToken)
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
			var body map[string]string
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Expect(body["error"]).To(ContainSubstring("event_id"))
		})

		Describe("missing required fields", func() {
			// Only identity/analytics fields are required. Metadata fields (framework, env,
			// started_at, run_attempt) are silently defaulted so data is never lost.
			required := []string{"event_id", "repo", "suite", "run_id", "test_id", "status"}
			for _, field := range required {
				field := field
				It("returns 400 when "+field+" is missing", func() {
					evt := map[string]any{
						"event_id":    "eid-" + field,
						"repo":        e2eRepo,
						"suite":       "s",
						"framework":   "ginkgo",
						"env":         "dev",
						"run_id":      "1",
						"run_attempt": 1,
						"test_id":     "t::test",
						"status":      "passed",
						"started_at":  time.Now(),
					}
					delete(evt, field)
					resp := doPost("/v1/runs/ingest", map[string]any{"events": []any{evt}}, e2eToken)
					defer resp.Body.Close()
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				})
			}
		})

		It("accepts events missing optional metadata fields and stores them with defaults", func() {
			suite := suiteName()
			rid := runID()
			now := time.Now().UTC().Truncate(time.Millisecond)

			batch := []map[string]any{
				{
					"event_id":   event.NewEventID(e2eRepo, rid, 1, "pkg::TestDefaulted", 0),
					"repo":       e2eRepo,
					"suite":      suite,
					"run_id":     rid,
					"test_id":    "pkg::TestDefaulted",
					"status":     "passed",
					"started_at": now,
					// framework, env, run_attempt intentionally omitted
				},
			}

			resp := doPost("/v1/runs/ingest", map[string]any{"events": batch}, e2eToken)
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

			// The event should be queryable via trends using the defaulted env="unknown".
			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "unknown", "30d")))
			Expect(totalAttempts(tr)).To(Equal(1), "event stored under env=unknown default")
		})
	})

	Describe("GET /v1/tests/history", func() {
		Context("missing required params", func() {
			paths := []string{
				"/v1/tests/history",
				"/v1/tests/history?repo=r",
				"/v1/tests/history?repo=r&suite=s",
				"/v1/tests/history?repo=r&suite=s&env=e",
			}
			for _, path := range paths {
				path := path
				It("returns 400 for "+path, func() {
					resp := doGet(path)
					defer resp.Body.Close()
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				})
			}
		})
	})

	Describe("GET /v1/tests/trends", func() {
		Context("missing required params", func() {
			paths := []string{
				"/v1/tests/trends",
				"/v1/tests/trends?repo=r",
				"/v1/tests/trends?repo=r&suite=s",
			}
			for _, path := range paths {
				path := path
				It("returns 400 for "+path, func() {
					resp := doGet(path)
					defer resp.Body.Close()
					Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
				})
			}
		})
	})
})

// ============================================================================
// Lifecycle tests: parse → ingest → store → query
// ============================================================================

var _ = Describe("Lifecycle", func() {
	Describe("Playwright fixture", func() {
		It("ingests 4 attempts and returns correct history and trends", func() {
			suite := suiteName()

			ingestFixture(suite, "playwright", "e2e",
				filepath.Join(fixturesDir, "playwright-results.json"), "")

			// "create VPC fails on timeout" has 2 results: 1 failed (attempt 0) + 1 passed (attempt 1)
			hs := decodeHistory(doGet(historyURL(e2eRepo, suite, "e2e",
				"tests/network/vpc.spec.ts::create VPC fails on timeout", "30d")))
			Expect(hs.Attempts).To(Equal(2), "both attempts stored")
			Expect(hs.Failed).To(Equal(1))
			Expect(hs.Passed).To(Equal(1))
			Expect(hs.FailureRate).To(BeNumerically("~", 50.0, 0.1))
			Expect(hs.LastFailedAt).NotTo(BeNil())
			Expect(hs.LastPassedAt).NotTo(BeNil())

			// Failure reason and commit SHA are surfaced in the summary.
			Expect(hs.LastFailureExcerpt).NotTo(BeNil(), "failure message must be captured")
			Expect(*hs.LastFailureExcerpt).To(ContainSubstring("Timeout"))
			Expect(hs.LastCommitSHA).NotTo(BeNil(), "commit SHA must be stored and returned")
			Expect(*hs.LastCommitSHA).To(Equal("e2esha"))

			// "create and delete VPC" — 1 passed, no failures
			hs2 := decodeHistory(doGet(historyURL(e2eRepo, suite, "e2e",
				"tests/network/vpc.spec.ts::create and delete VPC", "30d")))
			Expect(hs2.Attempts).To(Equal(1))
			Expect(hs2.Passed).To(Equal(1))
			Expect(hs2.Failed).To(Equal(0))
			Expect(hs2.FailureRate).To(BeNumerically("~", 0.0, 0.01))

			// Trends should reflect all 4 attempts (1 create, 2 timeout, 1 skipped)
			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "e2e", "30d")))
			Expect(tr.Buckets).NotTo(BeEmpty())
			Expect(totalAttempts(tr)).To(Equal(4))
		})
	})

	Describe("Ginkgo uni-compute-dev fixture", func() {
		It("ingests 43 attempts (42 passed, 1 failed)", func() {
			suite := suiteName()

			ingestFixture(suite, "ginkgo", "dev",
				filepath.Join(fixturesDir, "uni-compute-dev", "ginkgo-results.json"), "")

			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "dev", "30d")))
			Expect(tr.Buckets).NotTo(BeEmpty())
			Expect(totalAttempts(tr)).To(Equal(43))

			var totalFailed int
			for _, b := range tr.Buckets {
				totalFailed += b.Failed
			}
			Expect(totalFailed).To(Equal(1))
		})
	})

	Describe("Ginkgo uni-compute-uat fixture", func() {
		It("ingests 43 attempts with 2 failed", func() {
			suite := suiteName()

			ingestFixture(suite, "ginkgo", "uat",
				filepath.Join(fixturesDir, "uni-compute-uat", "ginkgo-results.json"), "")

			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "uat", "30d")))
			Expect(tr.Buckets).NotTo(BeEmpty())
			Expect(totalAttempts(tr)).To(Equal(43))

			var totalFailed int
			for _, b := range tr.Buckets {
				totalFailed += b.Failed
			}
			Expect(totalFailed).To(Equal(2))
		})
	})

	Describe("Ginkgo uni-region-uat fixture", func() {
		It("ingests 66 attempts with 12 passed", func() {
			suite := suiteName()

			ingestFixture(suite, "ginkgo", "uat",
				filepath.Join(fixturesDir, "uni-region-uat", "ginkgo-results.json"), "")

			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "uat", "30d")))
			Expect(tr.Buckets).NotTo(BeEmpty())
			Expect(totalAttempts(tr)).To(Equal(66))

			var totalPassed int
			for _, b := range tr.Buckets {
				totalPassed += b.Passed
			}
			Expect(totalPassed).To(Equal(12))
		})
	})

	Describe("JUnit fallback", func() {
		It("parses playwright JUnit XML under pytest framework and stores 3 attempts", func() {
			suite := suiteName()

			ingestFixture(suite, "pytest", "dev",
				"", filepath.Join(fixturesDir, "playwright-junit.xml"))

			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "dev", "30d")))
			Expect(tr.Buckets).NotTo(BeEmpty())
			Expect(totalAttempts(tr)).To(Equal(3))
		})
	})

	Describe("JUnit uni-compute-dev fixture", func() {
		It("ingests 43 attempts from JUnit XML", func() {
			suite := suiteName()

			ingestFixture(suite, "ginkgo", "dev",
				"", filepath.Join(fixturesDir, "uni-compute-dev", "junit.xml"))

			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "dev", "30d")))
			Expect(tr.Buckets).NotTo(BeEmpty())
			Expect(totalAttempts(tr)).To(Equal(43))
		})
	})

	Describe("Idempotency", func() {
		It("does not create duplicate rows when the same batch is ingested twice", func() {
			suite := suiteName()
			rid := runID()
			now := time.Now().UTC().Truncate(time.Millisecond)

			batch := []event.TestAttempt{
				{
					EventID: event.NewEventID(e2eRepo, rid, 1, "pkg::TestAlpha", 0),
					Repo: e2eRepo, Suite: suite, Framework: "ginkgo", Env: "e2e",
					RunID: rid, RunAttempt: 1,
					TestID: "pkg::TestAlpha", Status: "passed", DurationMS: 120, StartedAt: now,
				},
				{
					EventID: event.NewEventID(e2eRepo, rid, 1, "pkg::TestBeta", 0),
					Repo: e2eRepo, Suite: suite, Framework: "ginkgo", Env: "e2e",
					RunID: rid, RunAttempt: 1,
					TestID: "pkg::TestBeta", Status: "failed", DurationMS: 880, StartedAt: now,
				},
				{
					EventID: event.NewEventID(e2eRepo, rid, 1, "pkg::TestGamma", 0),
					Repo: e2eRepo, Suite: suite, Framework: "ginkgo", Env: "e2e",
					RunID: rid, RunAttempt: 1,
					TestID: "pkg::TestGamma", Status: "skipped", DurationMS: 0, StartedAt: now,
				},
			}

			body := map[string]any{"events": batch}

			resp1 := doPost("/v1/runs/ingest", body, e2eToken)
			defer resp1.Body.Close()
			Expect(resp1.StatusCode).To(Equal(http.StatusAccepted))

			// Ingest the exact same batch again — must not error.
			resp2 := doPost("/v1/runs/ingest", body, e2eToken)
			defer resp2.Body.Close()
			Expect(resp2.StatusCode).To(Equal(http.StatusAccepted))

			// Verify via the API that duplicates were silently dropped.
			for _, tc := range []struct {
				testID string
			}{
				{"pkg::TestAlpha"},
				{"pkg::TestBeta"},
				{"pkg::TestGamma"},
			} {
				hs := decodeHistory(doGet(historyURL(e2eRepo, suite, "e2e", tc.testID, "30d")))
				Expect(hs.Attempts).To(Equal(1), "%s: double-ingest must not create duplicate rows", tc.testID)
			}

			// Trends for the suite must also show 3 total attempts (not 6).
			tr := decodeTrends(doGet(trendsURL(e2eRepo, suite, "e2e", "30d")))
			Expect(totalAttempts(tr)).To(Equal(len(batch)), "suite total must equal original batch size")
		})
	})

	Describe("Window filtering", func() {
		for _, window := range []string{"7d", "30d", "90d"} {
			window := window
			It("returns the recent attempt within "+window, func() {
				suite := suiteName()
				rid := runID()
				now := time.Now().UTC().Truncate(time.Millisecond)

				batch := []event.TestAttempt{{
					EventID: event.NewEventID(e2eRepo, rid, 1, "pkg::TestWindow", 0),
					Repo: e2eRepo, Suite: suite, Framework: "ginkgo", Env: "e2e",
					RunID: rid, RunAttempt: 1,
					TestID: "pkg::TestWindow", Status: "passed", DurationMS: 50, StartedAt: now,
				}}
				resp := doPost("/v1/runs/ingest", map[string]any{"events": batch}, e2eToken)
				defer resp.Body.Close()
				Expect(resp.StatusCode).To(Equal(http.StatusAccepted))

				hs := decodeHistory(doGet(
					historyURL(e2eRepo, suite, "e2e", "pkg::TestWindow", window)))
				Expect(hs.Attempts).To(Equal(1))
			})
		}
	})

	Describe("Empty results", func() {
		It("returns a zero-value history summary for a nonexistent test", func() {
			resp := doGet(historyURL(e2eRepo, "nonexistent-suite", "e2e", "no::such::test", "30d"))
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			hs := decodeHistory(resp)
			Expect(hs.Attempts).To(Equal(0))
		})

		It("returns empty buckets for a suite with no data", func() {
			resp := doGet(trendsURL(e2eRepo, "nonexistent-suite", "e2e", "30d"))
			defer resp.Body.Close()
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			tr := decodeTrends(resp)
			Expect(tr.Buckets).To(BeEmpty())
		})
	})
})
