package event_test

import (
	"strings"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/stretchr/testify/assert"
)

func TestNewEventID_deterministic(t *testing.T) {
	id1 := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
	id2 := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
	assert.Equal(t, id1, id2, "same inputs must produce same ID")
	assert.Len(t, id1, 64, "SHA-256 hex is 64 chars")
}

func TestNewEventID_unique(t *testing.T) {
	base := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
	retry := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 1)
	diffTest := event.NewEventID("org/repo", "123", 1, "pkg.TestBar", 0)
	diffRun := event.NewEventID("org/repo", "456", 1, "pkg.TestFoo", 0)

	assert.NotEqual(t, base, retry, "different attempt_index must differ")
	assert.NotEqual(t, base, diffTest, "different test_id must differ")
	assert.NotEqual(t, base, diffRun, "different run_id must differ")
}

func TestNormalizeStatus_playwright(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"passed", event.StatusPassed},
		{"failed", event.StatusFailed},
		{"timedOut", event.StatusFailed},
		{"interrupted", event.StatusFailed},
		{"skipped", event.StatusSkipped},
	}
	for _, c := range cases {
		got := event.NormalizeStatus(event.FrameworkPlaywright, c.raw)
		assert.Equal(t, c.want, got, "playwright %q", c.raw)
	}
}

func TestNormalizeStatus_ginkgo(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"passed", event.StatusPassed},
		{"failed", event.StatusFailed},
		{"panicked", event.StatusFailed},
		{"skipped", event.StatusSkipped},
		{"pending", event.StatusSkipped},
	}
	for _, c := range cases {
		got := event.NormalizeStatus(event.FrameworkGinkgo, c.raw)
		assert.Equal(t, c.want, got, "ginkgo %q", c.raw)
	}
}

func TestNormalizeStatus_pytest(t *testing.T) {
	cases := []struct{ raw, want string }{
		{"passed", event.StatusPassed},
		{"failed", event.StatusFailed},
		{"error", event.StatusFailed},
		{"skipped", event.StatusSkipped},
		{"xfailed", event.StatusSkipped},
	}
	for _, c := range cases {
		got := event.NormalizeStatus(event.FrameworkPytest, c.raw)
		assert.Equal(t, c.want, got, "pytest %q", c.raw)
	}
}

func TestTruncateExcerpt(t *testing.T) {
	short := "hello"
	assert.Equal(t, short, event.TruncateExcerpt(short))

	long := strings.Repeat("a", 600)
	got := event.TruncateExcerpt(long)
	assert.Len(t, []rune(got), 500)
}
