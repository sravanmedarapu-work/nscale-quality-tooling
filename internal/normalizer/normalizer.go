package normalizer

import (
	"fmt"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/playwright"
)

// Context holds the CI/run metadata injected by the CLI from GitHub Actions env vars.
type Context struct {
	Repo        string
	Suite       string
	Framework   string
	Env         string
	Branch      string
	CommitSHA   string
	RunID       string
	RunAttempt  int
	ArtifactURL string
}

// FromJUnit converts JUnit parse results into normalized TestAttempts.
func FromJUnit(cases []junit.RawCase, ctx Context) []event.TestAttempt {
	out := make([]event.TestAttempt, 0, len(cases))
	for i, c := range cases {
		started := c.StartedAt
		if started.IsZero() {
			started = time.Now().UTC()
		}
		a := event.TestAttempt{
			EventID:               event.NewEventID(ctx.Repo, ctx.RunID, ctx.RunAttempt, c.TestID, i),
			Repo:                  ctx.Repo,
			Suite:                 ctx.Suite,
			Framework:             ctx.Framework,
			Env:                   ctx.Env,
			Branch:                ctx.Branch,
			CommitSHA:             ctx.CommitSHA,
			RunID:                 ctx.RunID,
			RunAttempt:            ctx.RunAttempt,
			TestID:                c.TestID,
			TestName:              c.Name,
			Status:                c.Status,
			DurationMS:            c.DurationMS,
			AttemptIndex:          0,
			FailureMessageExcerpt: c.FailureMessage,
			ArtifactURL:           ctx.ArtifactURL,
			StartedAt:             started,
		}
		out = append(out, a)
	}
	return out
}

// FromPlaywright converts Playwright parse results into normalized TestAttempts.
func FromPlaywright(results []playwright.RawResult, ctx Context) []event.TestAttempt {
	out := make([]event.TestAttempt, 0, len(results))
	for _, r := range results {
		started := r.StartedAt
		if started.IsZero() {
			started = time.Now().UTC()
		}
		a := event.TestAttempt{
			EventID:               event.NewEventID(ctx.Repo, ctx.RunID, ctx.RunAttempt, r.TestID, r.AttemptIndex),
			Repo:                  ctx.Repo,
			Suite:                 ctx.Suite,
			Framework:             event.FrameworkPlaywright,
			Env:                   ctx.Env,
			Branch:                ctx.Branch,
			CommitSHA:             ctx.CommitSHA,
			RunID:                 ctx.RunID,
			RunAttempt:            ctx.RunAttempt,
			TestID:                r.TestID,
			TestName:              r.TestName,
			Status:                r.Status,
			DurationMS:            r.DurationMS,
			AttemptIndex:          r.AttemptIndex,
			FailureMessageExcerpt: r.FailureMessage,
			ArtifactURL:           ctx.ArtifactURL,
			StartedAt:             started,
		}
		out = append(out, a)
	}
	return out
}

// FromGinkgo converts Ginkgo parse results into normalized TestAttempts.
func FromGinkgo(results []ginkgo.RawResult, ctx Context) []event.TestAttempt {
	out := make([]event.TestAttempt, 0, len(results))
	for _, r := range results {
		started := r.StartedAt
		if started.IsZero() {
			started = time.Now().UTC()
		}
		a := event.TestAttempt{
			EventID:               event.NewEventID(ctx.Repo, ctx.RunID, ctx.RunAttempt, r.TestID, r.AttemptIndex),
			Repo:                  ctx.Repo,
			Suite:                 ctx.Suite,
			Framework:             event.FrameworkGinkgo,
			Env:                   ctx.Env,
			Branch:                ctx.Branch,
			CommitSHA:             ctx.CommitSHA,
			RunID:                 ctx.RunID,
			RunAttempt:            ctx.RunAttempt,
			TestID:                r.TestID,
			TestName:              r.TestName,
			Status:                r.Status,
			DurationMS:            r.DurationMS,
			AttemptIndex:          r.AttemptIndex,
			FailureMessageExcerpt: r.FailureMessage,
			ArtifactURL:           ctx.ArtifactURL,
			StartedAt:             started,
		}
		out = append(out, a)
	}
	return out
}

// ArtifactURL builds the standard GitHub Actions run URL.
func ArtifactURL(repo, runID string) string {
	return fmt.Sprintf("https://github.com/%s/actions/runs/%s", repo, runID)
}
