package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
)

type storer interface {
	UpsertAttempts(ctx context.Context, attempts []event.TestAttempt) error
	QueryHistory(ctx context.Context, repo, suite, env, testID, window string) (*store.HistorySummary, error)
	QueryTrends(ctx context.Context, repo, suite, env, window string) ([]store.TrendBucket, error)
	Ping(ctx context.Context) error
}

// New builds the HTTP mux with all routes registered.
func New(st storer, token string) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{st: st, token: token}

	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("POST /v1/runs/ingest", h.ingest)
	mux.HandleFunc("GET /v1/tests/history", h.history)
	mux.HandleFunc("GET /v1/tests/trends", h.trends)

	return mux
}

type handlers struct {
	st    storer
	token string
}

// healthz returns 200 if the DB is reachable.
func (h *handlers) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	dbStatus := "ok"
	if err := h.st.Ping(ctx); err != nil {
		dbStatus = "error"
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	writeJSON(w, map[string]string{"status": "ok", "db": dbStatus})
}

// ingest accepts a batch of test attempt events.
func (h *handlers) ingest(w http.ResponseWriter, r *http.Request) {
	if !h.auth(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing token")
		return
	}

	var body struct {
		Events []event.TestAttempt `json:"events"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "invalid JSON: "+err.Error())
		return
	}
	if len(body.Events) == 0 {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "events array is empty")
		return
	}

	if err := validateAttempts(body.Events); err != nil {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	if err := h.st.UpsertAttempts(ctx, body.Events); err != nil {
		log.Printf("ingest: store error: %v", err)
		w.Header().Set("Retry-After", "30")
		writeError(w, http.StatusServiceUnavailable, "DB_UNAVAILABLE", "storage unavailable, retry later")
		return
	}

	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]any{"accepted": len(body.Events)})
}

// history returns aggregated history for one test.
func (h *handlers) history(w http.ResponseWriter, r *http.Request) {
	if !h.auth(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing token")
		return
	}
	q := r.URL.Query()
	repo := q.Get("repo")
	suite := q.Get("suite")
	env := q.Get("env")
	testID := q.Get("test_id")
	window := q.Get("window")
	if window == "" {
		window = "30d"
	}
	if repo == "" || suite == "" || env == "" || testID == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "repo, suite, env, and test_id are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	hs, err := h.st.QueryHistory(ctx, repo, suite, env, testID, window)
	if err != nil {
		log.Printf("history query: %v", err)
		writeError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	writeJSON(w, hs)
}

// trends returns day-bucketed failure rate and duration.
func (h *handlers) trends(w http.ResponseWriter, r *http.Request) {
	if !h.auth(r) {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid or missing token")
		return
	}
	q := r.URL.Query()
	repo := q.Get("repo")
	suite := q.Get("suite")
	env := q.Get("env")
	window := q.Get("window")
	if window == "" {
		window = "30d"
	}
	if repo == "" || suite == "" || env == "" {
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", "repo, suite, and env are required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	buckets, err := h.st.QueryTrends(ctx, repo, suite, env, window)
	if err != nil {
		log.Printf("trends query: %v", err)
		writeError(w, http.StatusInternalServerError, "DB_ERROR", "query failed")
		return
	}
	writeJSON(w, map[string]any{
		"repo": repo, "suite": suite, "env": env, "window": window,
		"buckets": buckets,
	})
}

func (h *handlers) auth(r *http.Request) bool {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return false
	}
	return strings.TrimPrefix(authHeader, "Bearer ") == h.token
}

func validateAttempts(attempts []event.TestAttempt) error {
	for i, a := range attempts {
		if a.EventID == "" {
			return fmt.Errorf("events[%d]: event_id is required", i)
		}
		if a.Repo == "" {
			return fmt.Errorf("events[%d]: repo is required", i)
		}
		if a.Suite == "" {
			return fmt.Errorf("events[%d]: suite is required", i)
		}
		if a.Framework == "" {
			return fmt.Errorf("events[%d]: framework is required", i)
		}
		if a.Env == "" {
			return fmt.Errorf("events[%d]: env is required", i)
		}
		if a.RunID == "" {
			return fmt.Errorf("events[%d]: run_id is required", i)
		}
		if a.TestID == "" {
			return fmt.Errorf("events[%d]: test_id is required", i)
		}
		if a.Status == "" {
			return fmt.Errorf("events[%d]: status is required", i)
		}
		if a.StartedAt.IsZero() {
			return fmt.Errorf("events[%d]: started_at is required", i)
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": code})
}
