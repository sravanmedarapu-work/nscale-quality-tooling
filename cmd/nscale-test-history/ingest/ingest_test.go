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
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Ingest Suite", suiteConfig, reporterConfig)
}

func fixtureDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "fixtures")
}

var _ = Describe("ingest.Run", func() {
	Context("When ingesting a Playwright JSON report", func() {
		It("should send 4 attempts and write a spool file", func() {
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
			GinkgoWriter.Printf("ingested %d playwright attempts\n", len(received))

			_, statErr := os.Stat(filepath.Join(dir, ".test-history", "events.ndjson"))
			Expect(statErr).NotTo(HaveOccurred(), "spool file must be written")
		})
	})

	Context("When ingesting a Ginkgo JSON report", func() {
		It("should send 3 attempts with framework=ginkgo", func() {
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
			GinkgoWriter.Printf("ingested %d ginkgo attempts\n", len(received))
		})
	})

	Context("When ingesting a JUnit XML report (pytest fallback)", func() {
		It("should send 3 attempts from the junit XML", func() {
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
			GinkgoWriter.Printf("ingested %d junit attempts\n", len(received))
		})
	})

	Context("When the API is unavailable", func() {
		It("should exit gracefully and still write the spool file", func() {
			fixtures := fixtureDir()

			GinkgoT().Setenv("TEST_HISTORY_API_URL", "http://127.0.0.1:19999")
			GinkgoT().Setenv("TEST_HISTORY_TOKEN", "test-token")
			GinkgoT().Setenv("GITHUB_REPOSITORY", "org/repo")
			GinkgoT().Setenv("GITHUB_RUN_ID", "55")

			dir := GinkgoT().TempDir()
			oldDir, err := os.Getwd()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(os.Chdir, oldDir)
			Expect(os.Chdir(dir)).To(Succeed())

			Expect(func() {
				ingest.Run([]string{
					"--suite", "s", "--framework", "playwright", "--env", "dev",
					"--json", filepath.Join(fixtures, "playwright-results.json"),
				})
			}).NotTo(Panic())

			_, statErr := os.Stat(filepath.Join(dir, ".test-history", "events.ndjson"))
			Expect(statErr).NotTo(HaveOccurred(), "spool must be written even when API is down")
			GinkgoWriter.Printf("spool file written despite API being down\n")
		})
	})

	Context("When required flags are missing", func() {
		It("should not panic", func() {
			Expect(func() {
				ingest.Run([]string{"--suite", "s"})
			}).NotTo(Panic())
		})
	})
})
