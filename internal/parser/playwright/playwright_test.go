package playwright_test

import (
	"os"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/playwright"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_fixture(t *testing.T) {
	f, err := os.Open("../../../testdata/fixtures/playwright-results.json")
	require.NoError(t, err)
	defer f.Close()

	results, err := playwright.Parse(f)
	require.NoError(t, err)

	// fixture has 3 specs: 1 passed, 1 failed+retry (2 results), 1 skipped = 4 total results
	require.Len(t, results, 4)

	passed := results[0]
	assert.Equal(t, event.StatusPassed, passed.Status)
	assert.Equal(t, "tests/network/vpc.spec.ts::create and delete VPC", passed.TestID)
	assert.Equal(t, 4200, passed.DurationMS)
	assert.Equal(t, 0, passed.AttemptIndex)

	failedFirst := results[1]
	assert.Equal(t, event.StatusFailed, failedFirst.Status)
	assert.Equal(t, 0, failedFirst.AttemptIndex)
	assert.Contains(t, failedFirst.FailureMessage, "Timeout")

	failedRetry := results[2]
	assert.Equal(t, event.StatusPassed, failedRetry.Status) // passed on retry
	assert.Equal(t, 1, failedRetry.AttemptIndex)

	skipped := results[3]
	assert.Equal(t, event.StatusSkipped, skipped.Status)
}

func TestParse_empty_suites(t *testing.T) {
	results, err := playwright.Parse(stringReader(`{"suites":[]}`))
	require.NoError(t, err)
	assert.Empty(t, results)
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "pw-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}
