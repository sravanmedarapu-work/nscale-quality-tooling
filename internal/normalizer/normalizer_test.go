package normalizer_test

import (
	"testing"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/normalizer"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/playwright"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testCtx = normalizer.Context{
	Repo:        "org/repo",
	Suite:       "my-suite",
	Framework:   "playwright",
	Env:         "dev",
	Branch:      "main",
	CommitSHA:   "abc123",
	RunID:       "999",
	RunAttempt:  1,
	ArtifactURL: "https://github.com/org/repo/actions/runs/999",
}

func TestFromPlaywright_sets_required_fields(t *testing.T) {
	now := time.Now().UTC()
	results := []playwright.RawResult{
		{TestID: "file::test1", TestName: "test1", Status: event.StatusPassed, DurationMS: 500, StartedAt: now},
	}
	attempts := normalizer.FromPlaywright(results, testCtx)
	require.Len(t, attempts, 1)

	a := attempts[0]
	assert.NotEmpty(t, a.EventID)
	assert.Len(t, a.EventID, 64)
	assert.Equal(t, "org/repo", a.Repo)
	assert.Equal(t, "my-suite", a.Suite)
	assert.Equal(t, event.FrameworkPlaywright, a.Framework)
	assert.Equal(t, "dev", a.Env)
	assert.Equal(t, "main", a.Branch)
	assert.Equal(t, "999", a.RunID)
	assert.Equal(t, event.StatusPassed, a.Status)
}

func TestFromPlaywright_event_id_deterministic(t *testing.T) {
	now := time.Now().UTC()
	r := playwright.RawResult{TestID: "f::t", Status: event.StatusPassed, DurationMS: 100, StartedAt: now}
	a1 := normalizer.FromPlaywright([]playwright.RawResult{r}, testCtx)
	a2 := normalizer.FromPlaywright([]playwright.RawResult{r}, testCtx)
	assert.Equal(t, a1[0].EventID, a2[0].EventID)
}

func TestFromGinkgo_maps_fields(t *testing.T) {
	now := time.Now().UTC()
	results := []ginkgo.RawResult{
		{TestID: "Ctx::spec", TestName: "spec", Status: event.StatusFailed,
			DurationMS: 1200, FailureMessage: "assert failed", StartedAt: now},
	}
	attempts := normalizer.FromGinkgo(results, testCtx)
	require.Len(t, attempts, 1)
	assert.Equal(t, event.StatusFailed, attempts[0].Status)
	assert.Equal(t, "assert failed", attempts[0].FailureMessageExcerpt)
	assert.Equal(t, event.FrameworkGinkgo, attempts[0].Framework)
}

func TestFromJUnit_maps_fields(t *testing.T) {
	now := time.Now().UTC()
	cases := []junit.RawCase{
		{TestID: "cls::name", Name: "name", Status: event.StatusPassed, DurationMS: 300, StartedAt: now},
	}
	attempts := normalizer.FromJUnit(cases, testCtx)
	require.Len(t, attempts, 1)
	assert.Equal(t, "cls::name", attempts[0].TestID)
	assert.Equal(t, event.StatusPassed, attempts[0].Status)
}

func TestArtifactURL(t *testing.T) {
	url := normalizer.ArtifactURL("org/repo", "123")
	assert.Equal(t, "https://github.com/org/repo/actions/runs/123", url)
}
