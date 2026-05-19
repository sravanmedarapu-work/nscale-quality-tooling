package event_test

import (
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
)

func TestEventSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Event Suite", suiteConfig, reporterConfig)
}

var _ = Describe("event.NewEventID", func() {
	Context("When given identical inputs", func() {
		It("should produce a deterministic 64-char hex ID", func() {
			id1 := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
			id2 := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
			Expect(id1).To(Equal(id2))
			Expect(id1).To(HaveLen(64))
			GinkgoWriter.Printf("event ID: %s\n", id1)
		})
	})

	Context("When inputs differ", func() {
		It("should produce unique IDs for different attempt indices", func() {
			base := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
			retry := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 1)
			Expect(base).NotTo(Equal(retry))
		})

		It("should produce unique IDs for different test IDs", func() {
			base := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
			diffTest := event.NewEventID("org/repo", "123", 1, "pkg.TestBar", 0)
			Expect(base).NotTo(Equal(diffTest))
		})

		It("should produce unique IDs for different run IDs", func() {
			base := event.NewEventID("org/repo", "123", 1, "pkg.TestFoo", 0)
			diffRun := event.NewEventID("org/repo", "456", 1, "pkg.TestFoo", 0)
			Expect(base).NotTo(Equal(diffRun))
		})
	})
})

var _ = Describe("event.NormalizeStatus", func() {
	Context("When framework is Playwright", func() {
		DescribeTable("should map raw status to canonical status",
			func(raw, want string) {
				got := event.NormalizeStatus(event.FrameworkPlaywright, raw)
				Expect(got).To(Equal(want))
				GinkgoWriter.Printf("playwright %q → %q\n", raw, got)
			},
			Entry("passed → passed", "passed", event.StatusPassed),
			Entry("failed → failed", "failed", event.StatusFailed),
			Entry("timedOut → failed", "timedOut", event.StatusFailed),
			Entry("interrupted → failed", "interrupted", event.StatusFailed),
			Entry("skipped → skipped", "skipped", event.StatusSkipped),
		)
	})

	Context("When framework is Ginkgo", func() {
		DescribeTable("should map raw status to canonical status",
			func(raw, want string) {
				got := event.NormalizeStatus(event.FrameworkGinkgo, raw)
				Expect(got).To(Equal(want))
				GinkgoWriter.Printf("ginkgo %q → %q\n", raw, got)
			},
			Entry("passed → passed", "passed", event.StatusPassed),
			Entry("failed → failed", "failed", event.StatusFailed),
			Entry("panicked → failed", "panicked", event.StatusFailed),
			Entry("skipped → skipped", "skipped", event.StatusSkipped),
			Entry("pending → skipped", "pending", event.StatusSkipped),
		)
	})

	Context("When framework is Pytest", func() {
		DescribeTable("should map raw status to canonical status",
			func(raw, want string) {
				got := event.NormalizeStatus(event.FrameworkPytest, raw)
				Expect(got).To(Equal(want))
				GinkgoWriter.Printf("pytest %q → %q\n", raw, got)
			},
			Entry("passed → passed", "passed", event.StatusPassed),
			Entry("failed → failed", "failed", event.StatusFailed),
			Entry("error → failed", "error", event.StatusFailed),
			Entry("skipped → skipped", "skipped", event.StatusSkipped),
			Entry("xfailed → skipped", "xfailed", event.StatusSkipped),
		)
	})
})

var _ = Describe("event.TruncateExcerpt", func() {
	Context("When the message is short", func() {
		It("should return it unchanged", func() {
			short := "hello"
			Expect(event.TruncateExcerpt(short)).To(Equal(short))
		})
	})

	Context("When the message exceeds 500 runes", func() {
		It("should truncate to exactly 500 runes", func() {
			long := strings.Repeat("a", 600)
			got := event.TruncateExcerpt(long)
			Expect([]rune(got)).To(HaveLen(500))
		})
	})
})
