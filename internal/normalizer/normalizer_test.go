package normalizer_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/normalizer"
	ginkgoparser "github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/playwright"
)

func TestNormalizerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Normalizer Suite", suiteConfig, reporterConfig)
}

var normCtx = normalizer.Context{
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

var _ = Describe("normalizer.FromPlaywright", func() {
	Context("When converting a passed result", func() {
		It("should set all required fields correctly", func() {
			now := time.Now().UTC()
			results := []playwright.RawResult{
				{TestID: "file::test1", TestName: "test1", Status: event.StatusPassed, DurationMS: 500, StartedAt: now},
			}
			attempts := normalizer.FromPlaywright(results, normCtx)
			Expect(attempts).To(HaveLen(1))

			a := attempts[0]
			Expect(a.EventID).NotTo(BeEmpty())
			Expect(a.EventID).To(HaveLen(64))
			Expect(a.Repo).To(Equal("org/repo"))
			Expect(a.Suite).To(Equal("my-suite"))
			Expect(a.Framework).To(Equal(event.FrameworkPlaywright))
			Expect(a.Env).To(Equal("dev"))
			Expect(a.Branch).To(Equal("main"))
			Expect(a.RunID).To(Equal("999"))
			Expect(a.Status).To(Equal(event.StatusPassed))
			GinkgoWriter.Printf("event ID: %s, framework: %s\n", a.EventID, a.Framework)
		})
	})

	Context("When converting the same result twice", func() {
		It("should produce deterministic event IDs", func() {
			now := time.Now().UTC()
			r := playwright.RawResult{TestID: "f::t", Status: event.StatusPassed, DurationMS: 100, StartedAt: now}
			a1 := normalizer.FromPlaywright([]playwright.RawResult{r}, normCtx)
			a2 := normalizer.FromPlaywright([]playwright.RawResult{r}, normCtx)
			Expect(a1[0].EventID).To(Equal(a2[0].EventID))
			GinkgoWriter.Printf("deterministic ID: %s\n", a1[0].EventID)
		})
	})
})

var _ = Describe("normalizer.FromGinkgo", func() {
	Context("When converting a failed result with a failure message", func() {
		It("should map all fields including failure excerpt and framework", func() {
			now := time.Now().UTC()
			results := []ginkgoparser.RawResult{
				{TestID: "Ctx::spec", TestName: "spec", Status: event.StatusFailed,
					DurationMS: 1200, FailureMessage: "assert failed", StartedAt: now},
			}
			attempts := normalizer.FromGinkgo(results, normCtx)
			Expect(attempts).To(HaveLen(1))
			Expect(attempts[0].Status).To(Equal(event.StatusFailed))
			Expect(attempts[0].FailureMessageExcerpt).To(Equal("assert failed"))
			Expect(attempts[0].Framework).To(Equal(event.FrameworkGinkgo))
			GinkgoWriter.Printf("failure excerpt: %s\n", attempts[0].FailureMessageExcerpt)
		})
	})
})

var _ = Describe("normalizer.FromJUnit", func() {
	Context("When converting a passed case", func() {
		It("should map test ID and status correctly", func() {
			now := time.Now().UTC()
			cases := []junit.RawCase{
				{TestID: "cls::name", Name: "name", Status: event.StatusPassed, DurationMS: 300, StartedAt: now},
			}
			attempts := normalizer.FromJUnit(cases, normCtx)
			Expect(attempts).To(HaveLen(1))
			Expect(attempts[0].TestID).To(Equal("cls::name"))
			Expect(attempts[0].Status).To(Equal(event.StatusPassed))
		})
	})
})

var _ = Describe("normalizer.ArtifactURL", func() {
	Context("When given a repo and run ID", func() {
		It("should return the correct GitHub Actions URL", func() {
			u := normalizer.ArtifactURL("org/repo", "123")
			Expect(u).To(Equal("https://github.com/org/repo/actions/runs/123"))
			GinkgoWriter.Printf("artifact URL: %s\n", u)
		})
	})
})
