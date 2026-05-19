package junit_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
)

func TestJUnitParser(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "JUnit Parser Suite", suiteConfig, reporterConfig)
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "junit-*.xml")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}

var _ = Describe("junit.Parse", func() {
	Context("When parsing the real Playwright JUnit fixture", func() {
		var f *os.File

		BeforeEach(func() {
			var err error
			f, err = os.Open("../../../testdata/fixtures/playwright-junit.xml")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.Close)
		})

		It("should return 3 cases with correct statuses and fields", func() {
			cases, err := junit.Parse(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(HaveLen(3))
			GinkgoWriter.Printf("parsed %d cases\n", len(cases))

			passed := cases[0]
			Expect(passed.Status).To(Equal(event.StatusPassed))
			Expect(passed.TestID).To(Equal("tests/network/vpc.spec.ts::create and delete VPC"))
			Expect(passed.DurationMS).To(Equal(4200))

			failed := cases[1]
			Expect(failed.Status).To(Equal(event.StatusFailed))
			Expect(failed.FailureMessage).To(ContainSubstring("Timeout"))
			GinkgoWriter.Printf("failure message: %s\n", failed.FailureMessage)

			skipped := cases[2]
			Expect(skipped.Status).To(Equal(event.StatusSkipped))
		})
	})

	Context("When parsing a bare <testsuite> (no <testsuites> wrapper)", func() {
		It("should return passed and failed cases correctly", func() {
			xml := `<testsuite name="my-suite">
				<testcase classname="pkg" name="TestA" time="1.0"/>
				<testcase classname="pkg" name="TestB" time="0.5">
					<failure message="assert failed">body</failure>
				</testcase>
			</testsuite>`

			cases, err := junit.Parse(stringReader(xml))
			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(HaveLen(2))
			Expect(cases[0].Status).To(Equal(event.StatusPassed))
			Expect(cases[1].Status).To(Equal(event.StatusFailed))
			Expect(cases[1].FailureMessage).To(Equal("assert failed"))
		})
	})

	Context("When a testcase contains an <error> element", func() {
		It("should map error to StatusFailed", func() {
			xml := `<testsuites><testsuite name="s">
				<testcase classname="pkg" name="TestC" time="2.0">
					<error message="panic: nil pointer"/>
				</testcase>
			</testsuite></testsuites>`

			cases, err := junit.Parse(stringReader(xml))
			Expect(err).NotTo(HaveOccurred())
			Expect(cases).To(HaveLen(1))
			Expect(cases[0].Status).To(Equal(event.StatusFailed))
			Expect(cases[0].FailureMessage).To(Equal("panic: nil pointer"))
			GinkgoWriter.Printf("error mapped to: %s\n", cases[0].Status)
		})
	})
})
