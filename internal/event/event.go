package event

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// Status values for a test attempt.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"
)

// Framework identifiers.
const (
	FrameworkPlaywright = "playwright"
	FrameworkGinkgo     = "ginkgo"
	FrameworkPytest     = "pytest"
	FrameworkJUnit      = "junit"
)

// TestAttempt is the normalized representation of one test attempt.
// One event per attempt — retries are separate events with incrementing AttemptIndex.
type TestAttempt struct {
	EventID   string `json:"event_id"`
	Repo      string `json:"repo"`
	Suite     string `json:"suite"`
	Framework string `json:"framework"`
	Env       string `json:"env"`

	Branch     string `json:"branch,omitempty"`
	CommitSHA  string `json:"commit,omitempty"`
	RunID      string `json:"run_id"`
	RunAttempt int    `json:"run_attempt"`

	TestID   string `json:"test_id"`
	TestName string `json:"test_name,omitempty"`

	Status     string `json:"status"`
	DurationMS int    `json:"duration_ms"`

	AttemptIndex int `json:"attempt_index"`

	FailureCategory       string `json:"failure_category,omitempty"`
	FailureFingerprint    string `json:"failure_fingerprint,omitempty"`
	FailureMessageExcerpt string `json:"failure_message_excerpt,omitempty"`

	ArtifactURL string `json:"artifact_url,omitempty"`
	PRNumber    int    `json:"pr_number,omitempty"`

	StartedAt time.Time `json:"started_at"`
}

// NewEventID generates a deterministic SHA-256–based ID for a test attempt.
// Inputs must be stable: same run + test + attempt always produces the same ID,
// making bulk replays idempotent.
func NewEventID(repo, runID string, runAttempt int, testID string, attemptIndex int) string {
	raw := fmt.Sprintf("%s:%s:%d:%s:%d", repo, runID, runAttempt, testID, attemptIndex)
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

// NormalizeStatus maps framework-specific status strings to the canonical set.
func NormalizeStatus(framework, raw string) string {
	switch framework {
	case FrameworkPlaywright:
		switch raw {
		case "passed":
			return StatusPassed
		case "failed", "timedOut", "interrupted":
			return StatusFailed
		default:
			return StatusSkipped
		}
	case FrameworkGinkgo:
		switch raw {
		case "passed":
			return StatusPassed
		case "failed", "panicked", "aborted", "timedout":
			return StatusFailed
		default:
			return StatusSkipped
		}
	case FrameworkPytest:
		switch raw {
		case "passed":
			return StatusPassed
		case "failed", "error":
			return StatusFailed
		default:
			return StatusSkipped
		}
	default:
		// JUnit fallback: caller sets status directly from element presence
		switch raw {
		case StatusPassed, StatusFailed, StatusSkipped:
			return raw
		default:
			return StatusSkipped
		}
	}
}

// TruncateExcerpt caps failure message excerpts at 500 chars.
func TruncateExcerpt(s string) string {
	const maxLen = 500
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}
