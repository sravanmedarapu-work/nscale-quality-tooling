package fixtures_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
)

func TestFixtures(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Fixtures Suite", suiteConfig, reporterConfig)
}

type fixtureSet struct {
	dir      string
	minSpecs int
}

func testFixtureSet(fs fixtureSet) {
	Context("ginkgo", func() {
		It("should have at least the expected number of results", func() {
			data, err := os.ReadFile(fmt.Sprintf("%s/ginkgo-results.json", fs.dir))
			Expect(err).NotTo(HaveOccurred())
			results, err := ginkgo.Parse(bytes.NewReader(data))
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("%s ginkgo: %d results\n", fs.dir, len(results))
			Expect(len(results)).To(BeNumerically(">=", fs.minSpecs))
		})
	})

	Context("junit", func() {
		It("should have at least the expected number of results", func() {
			data, err := os.ReadFile(fmt.Sprintf("%s/junit.xml", fs.dir))
			Expect(err).NotTo(HaveOccurred())
			results, err := junit.Parse(bytes.NewReader(data))
			Expect(err).NotTo(HaveOccurred())
			GinkgoWriter.Printf("%s junit: %d results\n", fs.dir, len(results))
			Expect(len(results)).To(BeNumerically(">=", fs.minSpecs))
		})
	})
}

var _ = Describe("UniComputeFixtures", func() {
	for _, env := range []string{"dev", "uat"} {
		env := env
		Context(env, func() {
			testFixtureSet(fixtureSet{dir: "uni-compute-" + env, minSpecs: 40})
		})
	}
})

var _ = Describe("UniRegionFixtures", func() {
	Context("uat", func() {
		testFixtureSet(fixtureSet{dir: "uni-region-uat", minSpecs: 13})
	})
})
