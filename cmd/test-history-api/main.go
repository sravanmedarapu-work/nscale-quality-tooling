package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/cmd/test-history-api/handler"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/store"
)

func main() {
	token := mustEnv("TEST_HISTORY_TOKEN")
	dbURL := mustEnv("DATABASE_URL")
	port := envOr("PORT", "8080")

	st, err := store.New(dbURL)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	mux := handler.New(st, token)
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      logMiddleware(mux),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("test-history-api listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-done
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// responseWriter captures the status code written by handlers.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// logMiddleware logs METHOD PATH status duration for every request.
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		log.Printf("%-6s %-40s %d  %s", r.Method, r.URL.RequestURI(), rw.status, time.Since(start).Round(time.Millisecond))
	})
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("required env var %s is not set", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
