package ingest_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/nscale-test-history/ingest"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
)

func TestIngestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Ingest Suite")
}

// fixtureDir returns the absolute path to testdata/fixtures.
func fixtureDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "fixtures")
}

var _ = Describe("ingest.Run", func() {
	Describe("playwright JSON", func() {
		It("sends 4 attempts and writes a spool file", func() {
			fixtures := fixtureDir()
			var received []event.TestAttempt

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				Expect(r.Header.Get("Authorization")).To(Equal("Bearer test-token"))
				var body struct{ Events []event.TestAttempt }
				json.NewDecoder(r.Body).Decode(&body)
				received = body.Events
				w.WriteHeader(http.StatusAccepted)
			}))
			DeferCleanup(srv.Close)

			GinkgoT().Setenv("TEST_HISTORY_API_URL", srv.URL)
			GinkgoT().Setenv("TEST_HISTORY_TOKEN", "test-token")
			GinkgoT().Setenv("GITHUB_REPOSITORY", "org/repo")
			GinkgoT().Setenv("GITHUB_RUN_ID", "42")
			GinkgoT().Setenv("GITHUB_RUN_ATTEMPT", "1")

			dir := GinkgoT().TempDir()
			oldDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Chdir, oldDir)
			Expect(os.Chdir(dir)).To(Succeed())

			ingest.Run([]string{
				"--suite", "console-e2e",
				"--framework", "playwright",
				"--env", "dev",
				"--json", filepath.Join(fixtures, "playwright-results.json"),
			})

			Expect(received).To(HaveLen(4), "4 attempts from the playwright fixture")
			for _, a := range received {
				Expect(a.Repo).To(Equal("org/repo"))
				Expect(a.Suite).To(Equal("console-e2e"))
				Expect(a.EventID).To(HaveLen(64))
			}

			_, statErr := os.Stat(filepath.Join(dir, ".test-history", "events.ndjson"))
			Expect(statErr).NotTo(HaveOccurred(), "spool file must be written")
		})
	})

	Describe("ginkgo JSON", func() {
		It("sends 3 attempts with framework=ginkgo", func() {
			fixtures := fixtureDir()
			var received []event.TestAttempt

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Events []event.TestAttempt }
				json.NewDecoder(r.Body).Decode(&body)
				received = body.Events
				w.WriteHeader(http.StatusAccepted)
			}))
			DeferCleanup(srv.Close)

			GinkgoT().Setenv("TEST_HISTORY_API_URL", srv.URL)
			GinkgoT().Setenv("TEST_HISTORY_TOKEN", "test-token")
			GinkgoT().Setenv("GITHUB_REPOSITORY", "org/repo")
			GinkgoT().Setenv("GITHUB_RUN_ID", "99")

			dir := GinkgoT().TempDir()
			oldDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Chdir, oldDir)
			Expect(os.Chdir(dir)).To(Succeed())

			ingest.Run([]string{
				"--suite", "uni-region-api",
				"--framework", "ginkgo",
				"--env", "dev",
				"--json", filepath.Join(fixtures, "ginkgo-results.json"),
			})

			Expect(received).To(HaveLen(3))
			Expect(received[0].Framework).To(Equal("ginkgo"))
		})
	})

	Describe("JUnit fallback", func() {
		It("sends 3 attempts from junit XML", func() {
			fixtures := fixtureDir()
			var received []event.TestAttempt

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct{ Events []event.TestAttempt }
				json.NewDecoder(r.Body).Decode(&body)
				received = body.Events
				w.WriteHeader(http.StatusAccepted)
			}))
			DeferCleanup(srv.Close)

			GinkgoT().Setenv("TEST_HISTORY_API_URL", srv.URL)
			GinkgoT().Setenv("TEST_HISTORY_TOKEN", "test-token")
			GinkgoT().Setenv("GITHUB_REPOSITORY", "org/repo")
			GinkgoT().Setenv("GITHUB_RUN_ID", "77")

			dir := GinkgoT().TempDir()
			oldDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Chdir, oldDir)
			Expect(os.Chdir(dir)).To(Succeed())

			ingest.Run([]string{
				"--suite", "pytest-suite",
				"--framework", "pytest",
				"--env", "dev",
				"--junit", filepath.Join(fixtures, "playwright-junit.xml"),
			})

			Expect(received).To(HaveLen(3))
		})
	})

	Describe("API down", func() {
		It("exits gracefully and still writes the spool file", func() {
			fixtures := fixtureDir()

			GinkgoT().Setenv("TEST_HISTORY_API_URL", "http://127.0.0.1:19999") // nothing listening
			GinkgoT().Setenv("TEST_HISTORY_TOKEN", "test-token")
			GinkgoT().Setenv("GITHUB_REPOSITORY", "org/repo")
			GinkgoT().Setenv("GITHUB_RUN_ID", "55")

			dir := GinkgoT().TempDir()
			oldDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Chdir, oldDir)
			Expect(os.Chdir(dir)).To(Succeed())

			// Must not panic
			ingest.Run([]string{
				"--suite", "s", "--framework", "playwright", "--env", "dev",
				"--json", filepath.Join(fixtures, "playwright-results.json"),
			})

			_, statErr := os.Stat(filepath.Join(dir, ".test-history", "events.ndjson"))
			Expect(statErr).NotTo(HaveOccurred(), "spool must be written even when API is down")
		})
	})

	Describe("missing required flags", func() {
		It("does not panic", func() {
			// Should not panic — just warn and return
			Expect(func() {
				ingest.Run([]string{"--suite", "s"})
			}).NotTo(Panic())
		})
	})
})
