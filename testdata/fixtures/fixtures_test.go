package fixtures_test

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
)

func TestUniComputeFixtures(t *testing.T) {
	for _, env := range []string{"dev", "uat"} {
		env := env
		t.Run("ginkgo_"+env, func(t *testing.T) {
			data, err := os.ReadFile(fmt.Sprintf("uni-compute-%s/ginkgo-results.json", env))
			if err != nil {
				t.Fatal(err)
			}
			results, err := ginkgo.Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("ginkgo %s: %d results", env, len(results))
			if len(results) == 0 {
				t.Fatal("expected results, got 0")
			}
		})
		t.Run("junit_"+env, func(t *testing.T) {
			data, err := os.ReadFile(fmt.Sprintf("uni-compute-%s/junit.xml", env))
			if err != nil {
				t.Fatal(err)
			}
			results, err := junit.Parse(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("junit %s: %d results", env, len(results))
			if len(results) == 0 {
				t.Fatal("expected results, got 0")
			}
		})
	}
}
