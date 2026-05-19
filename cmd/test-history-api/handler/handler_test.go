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
	RunSpecs(t, "Handler Suite")
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

var _ = Describe("Handler", func() {
	Describe("GET /healthz", func() {
		It("returns 200 when DB is up", func() {
			h := newHandler(&mockStore{})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
			Expect(rec.Code).To(Equal(http.StatusOK))
		})

		It("returns 503 when DB is down", func() {
			h := newHandler(&mockStore{pingErr: errSentinel})
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
			Expect(rec.Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	Describe("POST /v1/runs/ingest", func() {
		It("accepts a valid batch and returns 202", func() {
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
		})

		It("returns 401 when Authorization header is absent", func() {
			h := newHandler(&mockStore{})
			body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
			req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})

		It("returns 401 for a wrong token", func() {
			h := newHandler(&mockStore{})
			body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
			req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer wrong")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusUnauthorized))
		})

		It("returns 400 for an empty events array", func() {
			h := newHandler(&mockStore{})
			body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{}})
			req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer secret-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})

		Describe("missing required fields", func() {
			// Only identity/analytics fields are required; metadata fields get defaults.
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
				It("returns 400 when "+f.name+" is blank", func() {
					h := newHandler(&mockStore{})
					bad := validAttempt()
					f.blank(&bad)
					body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{bad}})
					req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
					req.Header.Set("Authorization", "Bearer secret-token")
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, req)
					Expect(rec.Code).To(Equal(http.StatusBadRequest), "missing %s must be 400", f.name)
				})
			}
		})

		It("applies defaults for optional metadata fields", func() {
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
		})

		It("returns 503 with Retry-After when DB is unavailable", func() {
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

	Describe("GET /v1/tests/history", func() {
		It("returns 200 for a valid request", func() {
			h := newHandler(&mockStore{})
			req := httptest.NewRequest("GET", "/v1/tests/history?repo=org/repo&suite=s&env=dev&test_id=f::t", nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})

		It("returns 400 when required params are missing", func() {
			h := newHandler(&mockStore{})
			req := httptest.NewRequest("GET", "/v1/tests/history?repo=org/repo", nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusBadRequest))
		})
	})

	Describe("GET /v1/tests/trends", func() {
		It("returns 200 for a valid request", func() {
			h := newHandler(&mockStore{})
			req := httptest.NewRequest("GET", "/v1/tests/trends?repo=org/repo&suite=s&env=dev", nil)
			req.Header.Set("Authorization", "Bearer secret-token")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			Expect(rec.Code).To(Equal(http.StatusOK))
		})
	})
})

// errSentinel is a simple non-nil error used where assert.AnError was previously used.
var errSentinel = &sentinelError{}

type sentinelError struct{}

func (e *sentinelError) Error() string { return "sentinel error" }
