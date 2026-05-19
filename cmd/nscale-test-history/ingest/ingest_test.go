package ingest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/nscale-test-history/ingest"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureDir returns the absolute path to testdata/fixtures.
func fixtureDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "fixtures")
}

func TestRun_playwright_json(t *testing.T) {
	fixtures := fixtureDir()
	var received []event.TestAttempt
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
		var body struct{ Events []event.TestAttempt }
		json.NewDecoder(r.Body).Decode(&body)
		received = body.Events
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv("TEST_HISTORY_API_URL", srv.URL)
	t.Setenv("TEST_HISTORY_TOKEN", "test-token")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")
	t.Setenv("GITHUB_RUN_ID", "42")
	t.Setenv("GITHUB_RUN_ATTEMPT", "1")

	dir := t.TempDir()
	t.Chdir(dir)

	ingest.Run([]string{
		"--suite", "console-e2e",
		"--framework", "playwright",
		"--env", "dev",
		"--json", filepath.Join(fixtures, "playwright-results.json"),
	})

	assert.Len(t, received, 4, "4 attempts from the playwright fixture")
	for _, a := range received {
		assert.Equal(t, "org/repo", a.Repo)
		assert.Equal(t, "console-e2e", a.Suite)
		assert.Len(t, a.EventID, 64)
	}

	_, err := os.Stat(filepath.Join(dir, ".test-history", "events.ndjson"))
	require.NoError(t, err, "spool file must be written")
}

func TestRun_ginkgo_json(t *testing.T) {
	fixtures := fixtureDir()
	var received []event.TestAttempt
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Events []event.TestAttempt }
		json.NewDecoder(r.Body).Decode(&body)
		received = body.Events
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv("TEST_HISTORY_API_URL", srv.URL)
	t.Setenv("TEST_HISTORY_TOKEN", "test-token")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")
	t.Setenv("GITHUB_RUN_ID", "99")

	dir := t.TempDir()
	t.Chdir(dir)

	ingest.Run([]string{
		"--suite", "uni-region-api",
		"--framework", "ginkgo",
		"--env", "dev",
		"--json", filepath.Join(fixtures, "ginkgo-results.json"),
	})

	require.Len(t, received, 3)
	assert.Equal(t, "ginkgo", received[0].Framework)
}

func TestRun_junit_fallback(t *testing.T) {
	fixtures := fixtureDir()
	var received []event.TestAttempt
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Events []event.TestAttempt }
		json.NewDecoder(r.Body).Decode(&body)
		received = body.Events
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	t.Setenv("TEST_HISTORY_API_URL", srv.URL)
	t.Setenv("TEST_HISTORY_TOKEN", "test-token")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")
	t.Setenv("GITHUB_RUN_ID", "77")

	dir := t.TempDir()
	t.Chdir(dir)

	ingest.Run([]string{
		"--suite", "pytest-suite",
		"--framework", "pytest",
		"--env", "dev",
		"--junit", filepath.Join(fixtures, "playwright-junit.xml"),
	})

	assert.Len(t, received, 3)
}

func TestRun_api_down_exits_gracefully(t *testing.T) {
	fixtures := fixtureDir()
	t.Setenv("TEST_HISTORY_API_URL", "http://127.0.0.1:19999") // nothing listening
	t.Setenv("TEST_HISTORY_TOKEN", "test-token")
	t.Setenv("GITHUB_REPOSITORY", "org/repo")
	t.Setenv("GITHUB_RUN_ID", "55")

	dir := t.TempDir()
	t.Chdir(dir)

	// Must not panic
	ingest.Run([]string{
		"--suite", "s", "--framework", "playwright", "--env", "dev",
		"--json", filepath.Join(fixtures, "playwright-results.json"),
	})

	// Spool must still be written despite API failure
	_, err := os.Stat(filepath.Join(dir, ".test-history", "events.ndjson"))
	require.NoError(t, err, "spool must be written even when API is down")
}

func TestRun_missing_required_flags(t *testing.T) {
	// Should not panic — just warn and return
	ingest.Run([]string{"--suite", "s"})
}
