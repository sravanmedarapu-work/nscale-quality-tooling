package junit_test

import (
	"os"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_fixture(t *testing.T) {
	f, err := os.Open("../../../testdata/fixtures/playwright-junit.xml")
	require.NoError(t, err)
	defer f.Close()

	cases, err := junit.Parse(f)
	require.NoError(t, err)
	require.Len(t, cases, 3)

	passed := cases[0]
	assert.Equal(t, event.StatusPassed, passed.Status)
	assert.Equal(t, "tests/network/vpc.spec.ts::create and delete VPC", passed.TestID)
	assert.Equal(t, 4200, passed.DurationMS)

	failed := cases[1]
	assert.Equal(t, event.StatusFailed, failed.Status)
	assert.Contains(t, failed.FailureMessage, "Timeout")

	skipped := cases[2]
	assert.Equal(t, event.StatusSkipped, skipped.Status)
}

func TestParse_bare_testsuite(t *testing.T) {
	xml := `<testsuite name="my-suite">
		<testcase classname="pkg" name="TestA" time="1.0"/>
		<testcase classname="pkg" name="TestB" time="0.5">
			<failure message="assert failed">body</failure>
		</testcase>
	</testsuite>`

	cases, err := junit.Parse(stringReader(xml))
	require.NoError(t, err)
	require.Len(t, cases, 2)
	assert.Equal(t, event.StatusPassed, cases[0].Status)
	assert.Equal(t, event.StatusFailed, cases[1].Status)
	assert.Equal(t, "assert failed", cases[1].FailureMessage)
}

func TestParse_error_element(t *testing.T) {
	xml := `<testsuites><testsuite name="s">
		<testcase classname="pkg" name="TestC" time="2.0">
			<error message="panic: nil pointer"/>
		</testcase>
	</testsuite></testsuites>`

	cases, err := junit.Parse(stringReader(xml))
	require.NoError(t, err)
	require.Len(t, cases, 1)
	assert.Equal(t, event.StatusFailed, cases[0].Status)
	assert.Equal(t, "panic: nil pointer", cases[0].FailureMessage)
}

func stringReader(s string) *os.File {
	// Write to temp file so we can use io.Reader interface
	f, _ := os.CreateTemp("", "junit-*.xml")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}
