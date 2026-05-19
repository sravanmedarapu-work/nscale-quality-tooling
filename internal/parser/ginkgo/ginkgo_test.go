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
	RunSpecs(t, "Ginkgo Parser Suite")
}

func stringReader(s string) *os.File {
	f, _ := os.CreateTemp("", "ginkgo-*.json")
	f.WriteString(s)
	f.Seek(0, 0)
	return f
}

var _ = Describe("ginkgo.Parse", func() {
	Describe("fixture file", func() {
		var f *os.File

		BeforeEach(func() {
			var err error
			f, err = os.Open("../../../testdata/fixtures/ginkgo-results.json")
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(f.Close)
		})

		It("parses 3 results with correct fields", func() {
			results, err := ginkgo.Parse(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(3))

			passed := results[0]
			Expect(passed.TestID).To(Equal("VPC > create and delete::should succeed and clean up"))
			Expect(passed.Status).To(Equal(event.StatusPassed))
			Expect(passed.DurationMS).To(Equal(5200))

			failed := results[1]
			Expect(failed.TestID).To(Equal("VPC > create and delete::should fail on missing auth"))
			Expect(failed.Status).To(Equal(event.StatusFailed))
			Expect(failed.FailureMessage).To(Equal("Expected status 401, got 500"))

			skipped := results[2]
			Expect(skipped.TestID).To(Equal("VPC::list all VPCs"))
			Expect(skipped.Status).To(Equal(event.StatusSkipped))
		})
	})

	Describe("retry creates multiple attempts", func() {
		It("produces one result per attempt", func() {
			json := `[{"SuiteDescription":"s","SpecReports":[
				{"ContainerHierarchyTexts":["Ctx"],"LeafNodeText":"flaky test",
				 "State":"passed","RunTime":1000000000,"NumAttempts":3,
				 "StartTime":"2026-05-19T10:00:00Z"}
			]}]`

			results, err := ginkgo.Parse(stringReader(json))
			Expect(err).NotTo(HaveOccurred())
			Expect(results).To(HaveLen(3), "3 attempts should produce 3 results")

			Expect(results[0].Status).To(Equal(event.StatusFailed), "first attempt: failed")
			Expect(results[1].Status).To(Equal(event.StatusFailed), "second attempt: failed")
			Expect(results[2].Status).To(Equal(event.StatusPassed), "last attempt: final state")
		})
	})
})
