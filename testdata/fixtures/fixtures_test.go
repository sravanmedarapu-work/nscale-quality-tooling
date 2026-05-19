package fixtures_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
)

type fixtureSet struct {
	dir      string
	minSpecs int
}

func testFixtureSet(t *testing.T, fs fixtureSet) {
	t.Helper()
	t.Run("ginkgo", func(t *testing.T) {
		data, err := os.ReadFile(fmt.Sprintf("%s/ginkgo-results.json", fs.dir))
		if err != nil {
			t.Fatal(err)
		}
		results, err := ginkgo.Parse(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s ginkgo: %d results", fs.dir, len(results))
		if len(results) < fs.minSpecs {
			t.Fatalf("expected >= %d results, got %d", fs.minSpecs, len(results))
		}
	})
	t.Run("junit", func(t *testing.T) {
		data, err := os.ReadFile(fmt.Sprintf("%s/junit.xml", fs.dir))
		if err != nil {
			t.Fatal(err)
		}
		results, err := junit.Parse(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s junit: %d results", fs.dir, len(results))
		if len(results) < fs.minSpecs {
			t.Fatalf("expected >= %d results, got %d", fs.minSpecs, len(results))
		}
	})
}

func TestUniComputeFixtures(t *testing.T) {
	for _, env := range []string{"dev", "uat"} {
		env := env
		t.Run(env, func(t *testing.T) {
			testFixtureSet(t, fixtureSet{dir: "uni-compute-" + env, minSpecs: 40})
		})
	}
}

func TestUniRegionFixtures(t *testing.T) {
	t.Run("uat", func(t *testing.T) {
		testFixtureSet(t, fixtureSet{dir: "uni-region-uat", minSpecs: 13})
	})
}
