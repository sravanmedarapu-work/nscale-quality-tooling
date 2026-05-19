package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/test-history-api/handler"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mock store ---

type mockStore struct {
	upsertErr  error
	upserted   []event.TestAttempt
	pingErr    error
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

// --- tests ---

func TestHealthz_ok(t *testing.T) {
	h := newHandler(&mockStore{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHealthz_db_down(t *testing.T) {
	h := newHandler(&mockStore{pingErr: assert.AnError})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestIngest_success(t *testing.T) {
	ms := &mockStore{}
	h := newHandler(ms)

	body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
	req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusAccepted, rec.Code)
	require.Len(t, ms.upserted, 1)
}

func TestIngest_no_auth(t *testing.T) {
	h := newHandler(&mockStore{})
	body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
	req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestIngest_wrong_token(t *testing.T) {
	h := newHandler(&mockStore{})
	body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
	req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestIngest_empty_events(t *testing.T) {
	h := newHandler(&mockStore{})
	body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{}})
	req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestIngest_missing_required_field(t *testing.T) {
	h := newHandler(&mockStore{})
	bad := validAttempt()
	bad.Repo = ""
	body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{bad}})
	req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestIngest_db_unavailable_returns_503(t *testing.T) {
	h := newHandler(&mockStore{upsertErr: assert.AnError})
	body, _ := json.Marshal(map[string]any{"events": []event.TestAttempt{validAttempt()}})
	req := httptest.NewRequest("POST", "/v1/runs/ingest", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "30", rec.Header().Get("Retry-After"))
}

func TestHistory_ok(t *testing.T) {
	h := newHandler(&mockStore{})
	req := httptest.NewRequest("GET", "/v1/tests/history?repo=org/repo&suite=s&env=dev&test_id=f::t", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestHistory_missing_params(t *testing.T) {
	h := newHandler(&mockStore{})
	req := httptest.NewRequest("GET", "/v1/tests/history?repo=org/repo", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTrends_ok(t *testing.T) {
	h := newHandler(&mockStore{})
	req := httptest.NewRequest("GET", "/v1/tests/trends?repo=org/repo&suite=s&env=dev", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusOK, rec.Code)
}
