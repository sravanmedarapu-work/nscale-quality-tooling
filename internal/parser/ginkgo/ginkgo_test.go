package ginkgo_test

import (
	"os"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_fixture(t *testing.T) {
	f, err := os.Open("../../../testdata/fixtures/ginkgo-results.json")
	require.NoError(t, err)
	defer f.Close()

	results, err := ginkgo.Parse(f)
	require.NoError(t, err)
	require.Len(t, results, 3)

	passed := results[0]
	assert.Equal(t, "VPC > create and delete::should succeed and clean up", passed.TestID)
	assert.Equal(t, event.StatusPassed, passed.Status)
	assert.Equal(t, 5200, passed.DurationMS)

	failed := results[1]
	assert.Equal(t, "VPC > create and delete::should fail on missing auth", failed.TestID)
	assert.Equal(t, event.StatusFailed, failed.Status)
	assert.Equal(t, "Expected status 401, got 500", failed.FailureMessage)

	skipped := results[2]
	assert.Equal(t, "VPC::list all VPCs", skipped.TestID)
	assert.Equal(t, event.StatusSkipped, skipped.Status)
}

func TestParse_retry_creates_multiple_attempts(t *testing.T) {
	json := `[{"SuiteDescription":"s","SpecReports":[
		{"ContainerHierarchyTexts":["Ctx"],"LeafNodeText":"flaky test",
		 "State":"passed","RunTime":1000000000,"NumAttempts":3,
		 "StartTime":"2026-05-19T10:00:00Z"}
	]}]`

	results, err := ginkgo.Parse(stringReader(json))
	require.NoError(t, err)
	require.Len(t, results, 3, "3 attempts should produce 3 results")

	assert.Equal(t, event.StatusFailed, results[0].Status, "first attempt: failed")
	assert.Equal(t, event.StatusFailed, results[1].Status, "second attempt: failed")
	assert.Equal(t, event.StatusPassed, results[2].Status, "last attempt: final state")
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "ginkgo-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}
