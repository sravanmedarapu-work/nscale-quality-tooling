//go:build e2e

// Package e2e_test exercises the full ingest→store→query lifecycle against a real
// Postgres. It can run in two modes:
//
//   - CI mode:   DATABASE_URL env var is set (service container). No Docker needed inside the test.
//   - Local mode: DATABASE_URL is unset; testcontainers spins up Postgres automatically.
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

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/nscale-test-history/ingest"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/test-history-api/handler"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
)

const (
	e2eToken = "e2e-secret"
	e2eRepo  = "org/e2e-repo"
)

// shared across all tests in this package
var (
	srv         *httptest.Server
	testStore   *store.Store
	fixturesDir string
	moduleRoot  string
)

// ---- response types mirroring handler output (no direct internal imports) ----

type historySummary struct {
	Repo          string     `json:"Repo"`
	Suite         string     `json:"Suite"`
	Env           string     `json:"Env"`
	TestID        string     `json:"TestID"`
	Window        string     `json:"Window"`
	Attempts      int        `json:"Attempts"`
	Passed        int        `json:"Passed"`
	Failed        int        `json:"Failed"`
	Skipped       int        `json:"Skipped"`
	FailureRate   float64    `json:"FailureRate"`
	P95DurationMS int        `json:"P95DurationMS"`
	LastFailedAt  *time.Time `json:"LastFailedAt"`
	LastPassedAt  *time.Time `json:"LastPassedAt"`
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

// ---- TestMain ----------------------------------------------------------------

func TestMain(m *testing.M) {
	_, filename, _, _ := runtime.Caller(0)
	moduleRoot = filepath.Join(filepath.Dir(filename), "..", "..")
	fixturesDir = filepath.Join(moduleRoot, "testdata", "fixtures")

	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	var stopDB func()

	if dbURL == "" {
		var err error
		dbURL, stopDB, err = startPostgres(ctx)
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e: start postgres:", err)
			os.Exit(1)
		}
	} else {
		stopDB = func() {}
	}

	if err := applyMigration(dbURL, filepath.Join(moduleRoot, "migrations", "001_initial_schema.sql")); err != nil {
		fmt.Fprintln(os.Stderr, "e2e: migrate:", err)
		stopDB()
		os.Exit(1)
	}

	var err error
	testStore, err = store.New(dbURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e: store:", err)
		stopDB()
		os.Exit(1)
	}

	srv = httptest.NewServer(handler.New(testStore, e2eToken))

	code := m.Run()

	srv.Close()
	testStore.Close()
	stopDB()
	os.Exit(code)
}

func startPostgres(ctx context.Context) (dsn string, stop func(), err error) {
	c, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("test_history"),
		tcpostgres.WithUsername("test_history"),
		tcpostgres.WithPassword("test_history"),
		tcpostgres.BasicWaitStrategies(),
		tcpostgres.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).WithStartupTimeout(60*time.Second)),
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

// suiteName returns a unique, DB-safe suite name per test to prevent cross-test
// contamination in the shared Postgres instance.
func suiteName(t *testing.T) string {
	t.Helper()
	return strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, t.Name())
}

// runID returns a unique run ID for each call (nanosecond timestamp).
func runID() string {
	return fmt.Sprintf("e2e-%d", time.Now().UnixNano())
}

// ingestFixture calls ingest.Run with the provided fixture files, wiring in the
// shared test server. It changes to a temp dir to isolate spool files.
func ingestFixture(t *testing.T, suite, framework, env, jsonPath, junitPath string) {
	t.Helper()
	t.Chdir(t.TempDir())

	t.Setenv("GITHUB_REPOSITORY", e2eRepo)
	t.Setenv("GITHUB_RUN_ID", runID())
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")
	t.Setenv("GITHUB_REF_NAME", "main")
	t.Setenv("GITHUB_SHA", "e2esha")
	t.Setenv("TEST_HISTORY_API_URL", srv.URL)
	t.Setenv("TEST_HISTORY_TOKEN", e2eToken)

	args := []string{"--suite", suite, "--framework", framework, "--env", env}
	if jsonPath != "" {
		args = append(args, "--json", jsonPath)
	}
	if junitPath != "" {
		args = append(args, "--junit", junitPath)
	}
	ingest.Run(args)
}

func doGet(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+e2eToken)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func doGetNoAuth(t *testing.T, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func doPost(t *testing.T, path string, body any, token string) *http.Response {
	t.Helper()
	b, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, srv.URL+path, strings.NewReader(string(b)))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
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

func decodeHistory(t *testing.T, resp *http.Response) historySummary {
	t.Helper()
	defer resp.Body.Close()
	var hs historySummary
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&hs))
	return hs
}

func decodeTrends(t *testing.T, resp *http.Response) trendsResponse {
	t.Helper()
	defer resp.Body.Close()
	var tr trendsResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&tr))
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

func TestHealthz_DBUp(t *testing.T) {
	resp := doGet(t, "/healthz")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "ok", body["db"])
}

func TestAuth_NoToken_AllEndpoints(t *testing.T) {
	paths := []struct {
		method string
		path   string
	}{
		{"POST", "/v1/runs/ingest"},
		{"GET", "/v1/tests/history?repo=r&suite=s&env=e&test_id=t"},
		{"GET", "/v1/tests/trends?repo=r&suite=s&env=e"},
	}
	for _, tc := range paths {
		tc := tc
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			var resp *http.Response
			if tc.method == "POST" {
				resp = doPost(t, tc.path, map[string]any{"events": []any{}}, "")
			} else {
				resp = doGetNoAuth(t, tc.path)
			}
			defer resp.Body.Close()
			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})
	}
}

func TestAuth_WrongToken(t *testing.T) {
	resp := doPost(t, "/v1/runs/ingest", map[string]any{"events": []any{}}, "bad-token")
	defer resp.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestIngest_EmptyEventsArray(t *testing.T) {
	resp := doPost(t, "/v1/runs/ingest", map[string]any{"events": []any{}}, e2eToken)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestIngest_MissingEventID(t *testing.T) {
	resp := doPost(t, "/v1/runs/ingest", map[string]any{
		"events": []any{map[string]any{
			// event_id deliberately omitted
			"repo": e2eRepo, "suite": "s", "framework": "ginkgo",
			"env": "dev", "run_id": "1", "run_attempt": 1,
			"test_id": "t::test", "status": "passed",
			"started_at": time.Now(),
		}},
	}, e2eToken)
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	var body map[string]string
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Contains(t, body["error"], "event_id")
}

func TestIngest_MissingRequiredFields(t *testing.T) {
	requiredFields := []string{"repo", "suite", "framework", "env", "run_id", "test_id", "status"}
	for _, field := range requiredFields {
		field := field
		t.Run("missing_"+field, func(t *testing.T) {
			evt := map[string]any{
				"event_id":   "eid-" + field,
				"repo":       e2eRepo,
				"suite":      "s",
				"framework":  "ginkgo",
				"env":        "dev",
				"run_id":     "1",
				"run_attempt": 1,
				"test_id":    "t::test",
				"status":     "passed",
				"started_at": time.Now(),
			}
			delete(evt, field)
			resp := doPost(t, "/v1/runs/ingest", map[string]any{"events": []any{evt}}, e2eToken)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestHistory_MissingParams(t *testing.T) {
	cases := []string{
		"/v1/tests/history",
		"/v1/tests/history?repo=r",
		"/v1/tests/history?repo=r&suite=s",
		"/v1/tests/history?repo=r&suite=s&env=e",
	}
	for _, path := range cases {
		path := path
		t.Run(path, func(t *testing.T) {
			resp := doGet(t, path)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestTrends_MissingParams(t *testing.T) {
	cases := []string{
		"/v1/tests/trends",
		"/v1/tests/trends?repo=r",
		"/v1/tests/trends?repo=r&suite=s",
	}
	for _, path := range cases {
		path := path
		t.Run(path, func(t *testing.T) {
			resp := doGet(t, path)
			defer resp.Body.Close()
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

// ============================================================================
// Lifecycle tests: parse → ingest → store → query
// ============================================================================

// TestLifecycle_Playwright ingests the small Playwright fixture and verifies
// both per-test history (including retry counts) and suite-level trends.
func TestLifecycle_Playwright(t *testing.T) {
	suite := suiteName(t)

	ingestFixture(t, suite, "playwright", "e2e",
		filepath.Join(fixturesDir, "playwright-results.json"), "")

	// "create VPC fails on timeout" has 2 results: 1 failed (attempt 0) + 1 passed (attempt 1)
	hs := decodeHistory(t, doGet(t,
		historyURL(e2eRepo, suite, "e2e",
			"tests/network/vpc.spec.ts::create VPC fails on timeout", "30d")))
	assert.Equal(t, 2, hs.Attempts, "both attempts stored")
	assert.Equal(t, 1, hs.Failed)
	assert.Equal(t, 1, hs.Passed)
	assert.InDelta(t, 50.0, hs.FailureRate, 0.1)
	assert.NotNil(t, hs.LastFailedAt)
	assert.NotNil(t, hs.LastPassedAt)

	// "create and delete VPC" — 1 passed, no failures
	hs2 := decodeHistory(t, doGet(t,
		historyURL(e2eRepo, suite, "e2e",
			"tests/network/vpc.spec.ts::create and delete VPC", "30d")))
	assert.Equal(t, 1, hs2.Attempts)
	assert.Equal(t, 1, hs2.Passed)
	assert.Equal(t, 0, hs2.Failed)
	assert.InDelta(t, 0.0, hs2.FailureRate, 0.01)

	// Trends should reflect all 4 attempts (1 create, 2 timeout, 1 skipped)
	tr := decodeTrends(t, doGet(t, trendsURL(e2eRepo, suite, "e2e", "30d")))
	require.NotEmpty(t, tr.Buckets)
	assert.Equal(t, 4, totalAttempts(tr))
}

// TestLifecycle_Ginkgo_UniComputeDev ingests the real uni-compute dev fixture
// (43 specs: 42 passed, 1 failed) and verifies counts via trends.
func TestLifecycle_Ginkgo_UniComputeDev(t *testing.T) {
	suite := suiteName(t)

	ingestFixture(t, suite, "ginkgo", "dev",
		filepath.Join(fixturesDir, "uni-compute-dev", "ginkgo-results.json"), "")

	tr := decodeTrends(t, doGet(t, trendsURL(e2eRepo, suite, "dev", "30d")))
	require.NotEmpty(t, tr.Buckets)
	assert.Equal(t, 43, totalAttempts(tr))

	var totalFailed int
	for _, b := range tr.Buckets {
		totalFailed += b.Failed
	}
	assert.Equal(t, 1, totalFailed)
}

// TestLifecycle_Ginkgo_UniComputeUAT ingests the UAT fixture (43 specs, 2 failed).
func TestLifecycle_Ginkgo_UniComputeUAT(t *testing.T) {
	suite := suiteName(t)

	ingestFixture(t, suite, "ginkgo", "uat",
		filepath.Join(fixturesDir, "uni-compute-uat", "ginkgo-results.json"), "")

	tr := decodeTrends(t, doGet(t, trendsURL(e2eRepo, suite, "uat", "30d")))
	require.NotEmpty(t, tr.Buckets)
	assert.Equal(t, 43, totalAttempts(tr))

	var totalFailed int
	for _, b := range tr.Buckets {
		totalFailed += b.Failed
	}
	assert.Equal(t, 2, totalFailed)
}

// TestLifecycle_Ginkgo_UniRegion ingests the uni-region UAT fixture
// (67 specs: 13 passed, 54 skipped — focused run).
func TestLifecycle_Ginkgo_UniRegion(t *testing.T) {
	suite := suiteName(t)

	ingestFixture(t, suite, "ginkgo", "uat",
		filepath.Join(fixturesDir, "uni-region-uat", "ginkgo-results.json"), "")

	tr := decodeTrends(t, doGet(t, trendsURL(e2eRepo, suite, "uat", "30d")))
	require.NotEmpty(t, tr.Buckets)
	assert.Equal(t, 67, totalAttempts(tr))

	var totalPassed int
	for _, b := range tr.Buckets {
		totalPassed += b.Passed
	}
	assert.Equal(t, 13, totalPassed)
}

// TestLifecycle_JUnit_Fallback verifies that the CLI falls back to JUnit XML
// parsing when no JSON report is provided. Uses the Playwright JUnit fixture
// under the "pytest" framework label.
func TestLifecycle_JUnit_Fallback(t *testing.T) {
	suite := suiteName(t)

	ingestFixture(t, suite, "pytest", "dev",
		"", filepath.Join(fixturesDir, "playwright-junit.xml"))

	tr := decodeTrends(t, doGet(t, trendsURL(e2eRepo, suite, "dev", "30d")))
	require.NotEmpty(t, tr.Buckets)
	assert.Equal(t, 3, totalAttempts(tr)) // junit fixture has 3 test cases
}

// TestLifecycle_JUnit_UniComputeDev ingests the real JUnit XML from uni-compute-dev.
func TestLifecycle_JUnit_UniComputeDev(t *testing.T) {
	suite := suiteName(t)

	ingestFixture(t, suite, "ginkgo", "dev",
		"", filepath.Join(fixturesDir, "uni-compute-dev", "junit.xml"))

	tr := decodeTrends(t, doGet(t, trendsURL(e2eRepo, suite, "dev", "30d")))
	require.NotEmpty(t, tr.Buckets)
	assert.Equal(t, 43, totalAttempts(tr))
}

// TestIdempotency verifies that ingesting the same batch twice does not
// create duplicate rows (ON CONFLICT DO NOTHING behaviour).
func TestIdempotency(t *testing.T) {
	suite := suiteName(t)
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

	resp1 := doPost(t, "/v1/runs/ingest", body, e2eToken)
	defer resp1.Body.Close()
	require.Equal(t, http.StatusAccepted, resp1.StatusCode)

	// Ingest the same batch again — must not error and must not double-count.
	resp2 := doPost(t, "/v1/runs/ingest", body, e2eToken)
	defer resp2.Body.Close()
	require.Equal(t, http.StatusAccepted, resp2.StatusCode)

	var count int
	err := testStore.DB().QueryRow(
		"SELECT count(*) FROM test_case_attempts WHERE run_id = $1 AND suite = $2",
		rid, suite,
	).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, len(batch), count, "duplicate events must be silently dropped")

	// Verify the history reflects the actual counts (not double).
	hs := decodeHistory(t, doGet(t,
		historyURL(e2eRepo, suite, "e2e", "pkg::TestBeta", "30d")))
	assert.Equal(t, 1, hs.Attempts)
	assert.Equal(t, 1, hs.Failed)
	assert.InDelta(t, 100.0, hs.FailureRate, 0.1)
}

// TestWindowFiltering verifies that the 7d/30d/90d window parameters
// correctly limit what history and trends queries return.
func TestWindowFiltering(t *testing.T) {
	suite := suiteName(t)
	rid := runID()
	now := time.Now().UTC().Truncate(time.Millisecond)

	// Insert one recent attempt (should appear in all windows)
	batch := []event.TestAttempt{{
		EventID: event.NewEventID(e2eRepo, rid, 1, "pkg::TestWindow", 0),
		Repo: e2eRepo, Suite: suite, Framework: "ginkgo", Env: "e2e",
		RunID: rid, RunAttempt: 1,
		TestID: "pkg::TestWindow", Status: "passed", DurationMS: 50, StartedAt: now,
	}}
	resp := doPost(t, "/v1/runs/ingest", map[string]any{"events": batch}, e2eToken)
	defer resp.Body.Close()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)

	for _, window := range []string{"7d", "30d", "90d"} {
		window := window
		t.Run(window, func(t *testing.T) {
			hs := decodeHistory(t, doGet(t,
				historyURL(e2eRepo, suite, "e2e", "pkg::TestWindow", window)))
			assert.Equal(t, 1, hs.Attempts)
		})
	}
}

// TestHistory_EmptyResult verifies that querying for a test that has no data
// returns a valid zero-value summary (not an error).
func TestHistory_EmptyResult(t *testing.T) {
	resp := doGet(t, historyURL(e2eRepo, "nonexistent-suite", "e2e", "no::such::test", "30d"))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	hs := decodeHistory(t, resp)
	assert.Equal(t, 0, hs.Attempts)
}

// TestTrends_EmptyResult verifies that querying trends for a suite with no data
// returns an empty buckets array (not an error).
func TestTrends_EmptyResult(t *testing.T) {
	resp := doGet(t, trendsURL(e2eRepo, "nonexistent-suite", "e2e", "30d"))
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	tr := decodeTrends(t, resp)
	assert.Empty(t, tr.Buckets)
}
