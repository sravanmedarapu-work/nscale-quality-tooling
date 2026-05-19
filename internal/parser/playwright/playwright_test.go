package playwright_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/playwright"
)

func TestPlaywrightParser(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Playwright Parser Suite")
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "pw-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}

var _ = Describe("playwright.Parse", func() {
	Describe("fixture file", func() {
		var f *os.File

		BeforeEach(func() {
			var err error
			f, err = os.Open("../../../testdata/fixtures/playwright-results.json")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.Close)
		})

		It("parses 4 results with correct fields", func() {
			results, err := playwright.Parse(f)
			Expect(err).NotTo(HaveOccurred())

			// fixture has 3 specs: 1 passed, 1 failed+retry (2 results), 1 skipped = 4 total results
			Expect(results).To(HaveLen(4))

			passed := results[0]
			Expect(passed.Status).To(Equal(event.StatusPassed))
			Expect(passed.TestID).To(Equal("tests/network/vpc.spec.ts::create and delete VPC"))
			Expect(passed.DurationMS).To(Equal(4200))
			Expect(passed.AttemptIndex).To(Equal(0))

			failedFirst := results[1]
			Expect(failedFirst.Status).To(Equal(event.StatusFailed))
			Expect(failedFirst.AttemptIndex).To(Equal(0))
			Expect(failedFirst.FailureMessage).To(ContainSubstring("Timeout"))

			failedRetry := results[2]
			Expect(failedRetry.Status).To(Equal(event.StatusPassed)) // passed on retry
			Expect(failedRetry.AttemptIndex).To(Equal(1))

			skipped := results[3]
			Expect(skipped.Status).To(Equal(event.StatusSkipped))
		})
	})

	Describe("empty suites", func() {
		It("returns empty results", func() {
			results, err := playwright.Parse(stringReader(`{"suites":[]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(BeEmpty())
		})
	})
})
