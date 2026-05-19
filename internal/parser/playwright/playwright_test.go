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
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Playwright Parser Suite", suiteConfig, reporterConfig)
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "pw-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}

var _ = Describe("playwright.Parse", func() {
	Context("When parsing the real fixture file", func() {
		var f *os.File

		BeforeEach(func() {
			var err error
			f, err = os.Open("../../../testdata/fixtures/playwright-results.json")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.Close)
		})

		It("should return 4 results covering passed, failed, retry, and skipped", func() {
			results, err := playwright.Parse(f)
			Expect(err).NotTo(HaveOccurred())
			// 3 specs: 1 passed, 1 failed+retry (2 results), 1 skipped = 4 total
			Expect(results).To(HaveLen(4))
			GinkgoWriter.Printf("parsed %d results\n", len(results))

			passed := results[0]
			Expect(passed.Status).To(Equal(event.StatusPassed))
			Expect(passed.TestID).To(Equal("tests/network/vpc.spec.ts::create and delete VPC"))
			Expect(passed.DurationMS).To(Equal(4200))
			Expect(passed.AttemptIndex).To(Equal(0))
			GinkgoWriter.Printf("passed: %s (%dms)\n", passed.TestID, passed.DurationMS)

			failedFirst := results[1]
			Expect(failedFirst.Status).To(Equal(event.StatusFailed))
			Expect(failedFirst.AttemptIndex).To(Equal(0))
			Expect(failedFirst.FailureMessage).To(ContainSubstring("Timeout"))
			GinkgoWriter.Printf("failed attempt 0: %s\n", failedFirst.FailureMessage)

			failedRetry := results[2]
			Expect(failedRetry.Status).To(Equal(event.StatusPassed))
			Expect(failedRetry.AttemptIndex).To(Equal(1))

			skipped := results[3]
			Expect(skipped.Status).To(Equal(event.StatusSkipped))
		})
	})

	Context("When the input has empty suites", func() {
		It("should return an empty result set", func() {
			results, err := playwright.Parse(stringReader(`{"suites":[]}`))
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(BeEmpty())
		})
	})
})
