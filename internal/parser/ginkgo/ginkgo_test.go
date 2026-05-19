package ginkgo_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
)

func TestGinkgoParser(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Ginkgo Parser Suite", suiteConfig, reporterConfig)
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "ginkgo-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}

var _ = Describe("ginkgo.Parse", func() {
	Context("When parsing the real fixture file", func() {
		var f *os.File

		BeforeEach(func() {
			var err error
			f, err = os.Open("../../../testdata/fixtures/ginkgo-results.json")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.Close)
		})

		It("should return 3 results with correct statuses and fields", func() {
			results, err := ginkgo.Parse(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(3))
			GinkgoWriter.Printf("parsed %d results\n", len(results))

			passed := results[0]
			Expect(passed.TestID).To(Equal("VPC > create and delete::should succeed and clean up"))
			Expect(passed.Status).To(Equal(event.StatusPassed))
			Expect(passed.DurationMS).To(Equal(5200))

			failed := results[1]
			Expect(failed.TestID).To(Equal("VPC > create and delete::should fail on missing auth"))
			Expect(failed.Status).To(Equal(event.StatusFailed))
			Expect(failed.FailureMessage).To(Equal("Expected status 401, got 500"))
			GinkgoWriter.Printf("failure message: %s\n", failed.FailureMessage)

			skipped := results[2]
			Expect(skipped.TestID).To(Equal("VPC::list all VPCs"))
			Expect(skipped.Status).To(Equal(event.StatusSkipped))
		})
	})

	Context("When a spec has multiple attempts (retries)", func() {
		It("should produce one result per attempt with intermediate attempts as failed", func() {
			json := `[{"SuiteDescription":"s","SpecReports":[
				{"ContainerHierarchyTexts":["Ctx"],"LeafNodeText":"flaky test",
				 "State":"passed","RunTime":1000000000,"NumAttempts":3,
				 "StartTime":"2026-05-19T10:00:00Z"}
			]}]`

			results, err := ginkgo.Parse(stringReader(json))
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(3), "3 attempts should produce 3 results")
			GinkgoWriter.Printf("retry results: %d\n", len(results))

			Expect(results[0].Status).To(Equal(event.StatusFailed), "first attempt: failed")
			Expect(results[1].Status).To(Equal(event.StatusFailed), "second attempt: failed")
			Expect(results[2].Status).To(Equal(event.StatusPassed), "last attempt: final state")
		})
	})

	Context("When the report contains setup nodes (BeforeSuite/AfterSuite)", func() {
		It("should exclude setup nodes with no LeafNodeText", func() {
			json := `[{"SuiteDescription":"s","SpecReports":[
				{"ContainerHierarchyTexts":[],"LeafNodeType":"BeforeSuite","LeafNodeText":"",
				 "State":"passed","RunTime":500000000,"NumAttempts":1,
				 "StartTime":"2026-05-19T10:00:00Z"},
				{"ContainerHierarchyTexts":["Suite"],"LeafNodeText":"real spec",
				 "State":"passed","RunTime":200000000,"NumAttempts":1,
				 "StartTime":"2026-05-19T10:00:01Z"}
			]}]`

			results, err := ginkgo.Parse(stringReader(json))
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(1), "setup node must be excluded")
			Expect(results[0].TestID).To(Equal("Suite::real spec"))
			GinkgoWriter.Printf("excluded BeforeSuite, kept: %s\n", results[0].TestID)
		})
	})
})
