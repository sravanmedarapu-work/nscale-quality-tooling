package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/test-history-api/handler"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
)

func TestHandlerSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	suiteConfig, reporterConfig := GinkgoConfiguration()
	reporterConfig.Verbose = true
	RunSpecs(t, "Handler Suite", suiteConfig, reporterConfig)
}

// --- mock store ---

type mockStore struct {
	upsertErr error
	upserted  []event.TestAttempt
	pingErr   error
}

func (m *mockStore) UpsertAttempts(_ context.Context, a []event.TestAttempt) error {
	m.upserted = append(m.upserted, a...)
	return m.upsertErr
}
func (m *mockStore) QueryHistory(_ context.Context, _, _, _, _, _ string) (*store.HistorySummary, error) {
	return &store.HistorySummary{Attempts: 5, Passed: 4, Failed: 1, FailureRate: 20.0}, nil
}
func (m *mockStore) QueryTrends(_ context.Context, _, _, _, _ string) ([]store.TrendBucket, error) {
	return []store.TrendBucket{{Attempts: 3}}, nil
}
func (m *mockStore) Ping(_ context.Context) error { return m.pingErr }

// errSentinel is a non-nil error for DB-down scenarios.
var errSentinel = &sentinelError{}

type sentinelError struct{}

func (e *sentinelError) Error() string { return "sentinel error" }

// --- helpers ---

func newHandler(ms *mockStore) http.Handler {
	return handler.New(ms, "secret-token")
}

func validAttempt() event.TestAttempt {
	return event.TestAttempt{
		EventID: "evt-1", Repo: "org/repo", Suite: "s", Framework: "playwright",
		Env: "dev", RunID: "1", RunAttempt: 1, TestID: "f::t",
		Status: event.StatusPassed, DurationMS: 500, StartedAt: time.Now().UTC(),
	}
}

// --- specs ---

var _ = Describe("HTTP handler", func() {
	Describe("GET /healthz", func() {
		Context("When the database is healthy", func() {
			It("should return 200 OK", func() {
				h := newHandler(&mockStore{})
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
				Expect(rec.Code).To(Equal(http.StatusOK))
				GinkgoWriter.Printf("healthz response: %d\n", rec.Code)
			})
		})

		Context("When the database is down", func() {
			It("should return 503 Service Unavailable", func() {
				h := newHandler(&mockStore{pingErr: errSentinel})
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
				Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
			})
		})
	})

	Describe("POST /v1/runs/ingest", func() {
		Context("When given a valid batch with correct auth", func() {
			It("should accept the batch and return 202 Accepted", func() {
				ms := &mockStore{}
				h := newHandler(ms)

				body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
				req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer secret-token")
				req.Header.Set("Content-Type", "application/json")

				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusAccepted))
				Expect(ms.upserted).To(HaveLen(1))
				GinkgoWriter.Printf("upserted %d events\n", len(ms.upserted))
			})
		})

		Context("When auth is missing or wrong", func() {
			It("should return 401 when Authorization header is absent", func() {
				h := newHandler(&mockStore{})
				body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
				req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
			})

			It("should return 401 for a wrong token", func() {
				h := newHandler(&mockStore{})
				body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
				req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer wrong")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusUnauthorized))
			})
		})

		Context("When the request body is invalid", func() {
			It("should return 400 for an empty events array", func() {
				h := newHandler(&mockStore{})
				body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{}})
				req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer secret-token")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			})

			type requiredField struct {
				name  string
				blank func(a *event.TestAttempt)
			}
			fields := []requiredField{
				{"event_id", func(a *event.TestAttempt) { a.EventID = "" }},
				{"repo", func(a *event.TestAttempt) { a.Repo = "" }},
				{"suite", func(a *event.TestAttempt) { a.Suite = "" }},
				{"run_id", func(a *event.TestAttempt) { a.RunID = "" }},
				{"test_id", func(a *event.TestAttempt) { a.TestID = "" }},
				{"status", func(a *event.TestAttempt) { a.Status = "" }},
			}
			for _, f := range fields {
				f := f
				It("should return 400 when "+f.name+" is blank", func() {
					h := newHandler(&mockStore{})
					bad := validAttempt()
					f.blank(&bad)
					body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{bad}})
					req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
					req.Header.Set("Authorization", "Bearer secret-token")
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, req)
					Expect(rec.Code).To(Equal(http.StatusBadRequest), "missing %s must be 400", f.name)
					GinkgoWriter.Printf("blank %s → %d\n", f.name, rec.Code)
				})
			}
		})

		Context("When optional metadata fields are omitted", func() {
			It("should apply defaults and return 202 Accepted", func() {
				ms := &mockStore{}
				h := newHandler(ms)

				minimal := event.TestAttempt{
					EventID: "evt-min", Repo: "org/repo", Suite: "s",
					RunID: "1", TestID: "f::t", Status: event.StatusPassed,
				}
				body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{minimal}})
				req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer secret-token")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)

				Expect(rec.Code).To(Equal(http.StatusAccepted))
				Expect(ms.upserted).To(HaveLen(1))
				stored := ms.upserted[0]
				Expect(stored.Framework).To(Equal("unknown"), "framework defaults to unknown")
				Expect(stored.Env).To(Equal("unknown"), "env defaults to unknown")
				Expect(stored.RunAttempt).To(Equal(1), "run_attempt defaults to 1")
				Expect(stored.StartedAt.IsZero()).To(BeFalse(), "started_at defaults to now")
				GinkgoWriter.Printf("defaults applied: framework=%s env=%s\n", stored.Framework, stored.Env)
			})
		})

		Context("When the database is unavailable during ingest", func() {
			It("should return 503 with Retry-After header", func() {
				h := newHandler(&mockStore{upsertErr: errSentinel})
				body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
				req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
				req.Header.Set("Authorization", "Bearer secret-token")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
				Expect(rec.Header().Get("Retry-After")).To(Equal("30"))
			})
		})
	})

	Describe("GET /v1/tests/history", func() {
		Context("When all required params are present", func() {
			It("should return 200 OK", func() {
				h := newHandler(&mockStore{})
				req := httptest.NewRequest("GET", "/v1/tests/history?repo=org/repo&suite=s&env=dev&test_id=f::t", nil)
				req.Header.Set("Authorization", "Bearer secret-token")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK))
			})
		})

		Context("When required params are missing", func() {
			It("should return 400 Bad Request", func() {
				h := newHandler(&mockStore{})
				req := httptest.NewRequest("GET", "/v1/tests/history?repo=org/repo", nil)
				req.Header.Set("Authorization", "Bearer secret-token")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusBadRequest))
			})
		})
	})

	Describe("GET /v1/tests/trends", func() {
		Context("When all required params are present", func() {
			It("should return 200 OK", func() {
				h := newHandler(&mockStore{})
				req := httptest.NewRequest("GET", "/v1/tests/trends?repo=org/repo&suite=s&env=dev", nil)
				req.Header.Set("Authorization", "Bearer secret-token")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				Expect(rec.Code).To(Equal(http.StatusOK))
			})
		})
	})
})
