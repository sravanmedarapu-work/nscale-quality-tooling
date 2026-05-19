package ingest

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/event"
	"github.com/sravanmedarapu-work/nscale-quality-tooling/internal/normalizer"
	ginkgoparser "github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/ginkgo"
	junitparser "github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/junit"
	pwparser "github.com/sravanmedarapu-work/nscale-quality-tooling/internal/parser/playwright"
)

// Run is the entry point for `nscale-test-history ingest`.
// It always exits 0 — failures are warnings, not CI failures.
func Run(args []string) {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	suite := fs.String("suite", "", "suite name (required)")
	framework := fs.String("framework", "", "playwright|ginkgo|pytest (required)")
	env := fs.String("env", "", "environment e.g. dev (required)")
	junitPath := fs.String("junit", "", "path to JUnit XML report")
	jsonPath := fs.String("json", "", "path to framework JSON report")

	if err := fs.Parse(args); err != nil {
		warn("flag parse error: %v", err)
		return
	}
	if *suite == "" || *framework == "" || *env == "" {
		warn("--suite, --framework, and --env are required")
		return
	}
	if *junitPath == "" && *jsonPath == "" {
		warn("at least one of --junit or --json must be provided")
		return
	}

	ctx := buildContext(*suite, *framework, *env)
	attempts, err := parseReports(*framework, *junitPath, *jsonPath, ctx)
	if err != nil {
		warn("parse error: %v", err)
		return
	}
	if len(attempts) == 0 {
		warn("no test events found in the provided reports")
		return
	}

	if err := writeSpool(attempts); err != nil {
		warn("spool write error: %v", err)
		// Continue — spool failure should not prevent API posting
	}

	apiURL := os.Getenv("TEST_HISTORY_API_URL")
	token := os.Getenv("TEST_HISTORY_TOKEN")
	if apiURL == "" || token == "" {
		warn("TEST_HISTORY_API_URL and TEST_HISTORY_TOKEN not set — skipping API ingest")
		return
	}

	if err := postToAPI(apiURL, token, attempts); err != nil {
		warn("API ingest failed (events saved to .test-history/events.ndjson for replay): %v", err)
		return
	}
	fmt.Printf("[test-history] ingested %d events to %s\n", len(attempts), apiURL)
}

func buildContext(suite, framework, env string) normalizer.Context {
	repo := envOr("GITHUB_REPOSITORY", "unknown/unknown")
	branch := envOr("GITHUB_REF_NAME", "")
	commit := envOr("GITHUB_SHA", "")
	runID := envOr("GITHUB_RUN_ID", fmt.Sprintf("local-%d", time.Now().UnixMilli()))
	runAttempt, _ := strconv.Atoi(envOr("GITHUB_RUN_ATTEMPT", "1"))
	if runAttempt == 0 {
		runAttempt = 1
	}
	return normalizer.Context{
		Repo:        repo,
		Suite:       suite,
		Framework:   framework,
		Env:         env,
		Branch:      branch,
		CommitSHA:   commit,
		RunID:       runID,
		RunAttempt:  runAttempt,
		ArtifactURL: normalizer.ArtifactURL(repo, runID),
	}
}

func parseReports(framework, junitPath, jsonPath string, ctx normalizer.Context) ([]event.TestAttempt, error) {
	// Prefer JSON when available (richer retry/error data)
	switch framework {
	case event.FrameworkPlaywright:
		if jsonPath != "" {
			f, err := os.Open(jsonPath)
			if err != nil {
				return nil, fmt.Errorf("open playwright json: %w", err)
			}
			defer f.Close()
			results, err := pwparser.Parse(f)
			if err != nil {
				return nil, fmt.Errorf("parse playwright json: %w", err)
			}
			return normalizer.FromPlaywright(results, ctx), nil
		}
	case event.FrameworkGinkgo:
		if jsonPath != "" {
			f, err := os.Open(jsonPath)
			if err != nil {
				return nil, fmt.Errorf("open ginkgo json: %w", err)
			}
			defer f.Close()
			results, err := ginkgoparser.Parse(f)
			if err != nil {
				return nil, fmt.Errorf("parse ginkgo json: %w", err)
			}
			return normalizer.FromGinkgo(results, ctx), nil
		}
	}

	// Fall back to JUnit XML
	if junitPath != "" {
		f, err := os.Open(junitPath)
		if err != nil {
			return nil, fmt.Errorf("open junit xml: %w", err)
		}
		defer f.Close()
		cases, err := junitparser.Parse(f)
		if err != nil {
			return nil, fmt.Errorf("parse junit xml: %w", err)
		}
		return normalizer.FromJUnit(cases, ctx), nil
	}
	return nil, fmt.Errorf("no parseable report found for framework %q", framework)
}

func writeSpool(attempts []event.TestAttempt) error {
	if err := os.MkdirAll(".test-history", 0o755); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(".test-history", "events.ndjson"))
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, a := range attempts {
		if err := enc.Encode(a); err != nil {
			return err
		}
	}
	return nil
}

func postToAPI(apiURL, token string, attempts []event.TestAttempt) error {
	body, err := json.Marshal(map[string]any{"events": attempts})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := apiURL + "/v1/runs/ingest"
	fmt.Printf("[test-history] POST %s  events=%d\n", url, len(attempts))

	client := &http.Client{Timeout: 30 * time.Second}

	doRequest := func() (*http.Response, error) {
		req, err := http.NewRequest("POST", url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		return client.Do(req)
	}

	resp, err := doRequest()
	if err != nil {
		fmt.Printf("[test-history] request failed (%v), retrying in 5s...\n", err)
		time.Sleep(5 * time.Second)
		resp, err = doRequest()
		if err != nil {
			return fmt.Errorf("post (after retry): %w", err)
		}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	fmt.Printf("[test-history] response  status=%d  body=%s\n", resp.StatusCode, string(respBody))

	if resp.StatusCode >= 300 {
		return fmt.Errorf("API returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func warn(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[test-history] WARNING: "+format+"\n", args...)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
