# nscale-quality-tooling

Test history ingestion pipeline for nscale CI: collects test results from GitHub Actions, stores them in Postgres, and exposes a query API for dashboards and trend analysis.

## Architecture

```
GitHub Actions CI
      │
      ▼
nscale-test-history CLI   (cmd/nscale-test-history)
  - Parses Playwright JSON, Ginkgo v2 JSON, JUnit XML
  - Writes spool file (.test-history/events.ndjson)
  - POSTs to REST API
      │
      ▼
test-history-api          (cmd/test-history-api)
  - Bearer token auth
  - POST /v1/runs/ingest  → idempotent upsert (SHA-256 event_id)
  - GET  /v1/tests/history → per-test stats over a time window
  - GET  /v1/tests/trends  → day-bucketed failure rate + p95 duration
      │
      ▼
PostgreSQL (test_case_attempts table)
```

## Prerequisites

- Go 1.22+
- Docker (for local Postgres and integration tests)

## Quickstart

```bash
# Build both binaries
make build

# Run unit tests (no Docker)
make test

# Start local dev environment (Postgres + API on :8080)
make dev

# Seed with fixture data (in a separate terminal)
make seed
```

Or use Claude Code slash commands: `/dev`, `/test`, `/seed`.

## Directory Structure

```
cmd/
  test-history-api/         API server binary + handler package
    handler/                HTTP handlers, router, auth middleware
  nscale-test-history/      CLI binary
    ingest/                 Parse, spool, and POST logic
internal/
  event/                    Core TestAttempt model + NewEventID
  parser/
    playwright/             Playwright JSON parser
    ginkgo/                 Ginkgo v2 JSON parser
    junit/                  JUnit XML parser (fallback for all frameworks)
  normalizer/               Convert parser output → []event.TestAttempt
  store/                    Postgres store (upsert, history, trends queries)
migrations/
  001_initial_schema.sql    test_case_attempts DDL + indexes
testdata/fixtures/          Sample result files for tests and seeding
```

## API Reference

All endpoints require `Authorization: Bearer <token>`.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/healthz` | Liveness + DB ping |
| POST | `/v1/runs/ingest` | Ingest a batch of test attempts |
| GET | `/v1/tests/history` | Aggregated stats for one test (`repo`, `suite`, `env`, `test_id`, `window`) |
| GET | `/v1/tests/trends` | Day-bucketed trends for a suite (`repo`, `suite`, `env`, `window`) |

`window` accepts: `7d`, `30d`, `90d`.

## CLI Usage

```bash
GITHUB_REPOSITORY=org/repo \
GITHUB_RUN_ID=12345 \
GITHUB_RUN_ATTEMPT=1 \
GITHUB_REF_NAME=main \
GITHUB_SHA=abc123 \
TEST_HISTORY_API_URL=https://your-api \
TEST_HISTORY_TOKEN=your-token \
./bin/nscale-test-history ingest \
  --suite my-suite \
  --framework playwright \
  --env staging \
  --json results.json \
  --junit results.xml
```

The CLI always exits 0. If the API is unreachable, events are preserved in `.test-history/events.ndjson` for later replay.

## Running Tests

```bash
# Unit tests only
make test

# Integration tests (requires Docker — spins up real Postgres via testcontainers)
make test-integration
```

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `DATABASE_URL` | API server | Postgres connection string |
| `TEST_HISTORY_TOKEN` | Both | Bearer token for auth |
| `PORT` | API server | HTTP listen port (default 8080) |
| `TEST_HISTORY_API_URL` | CLI | API base URL |
| `GITHUB_REPOSITORY` | CLI | `org/repo` |
| `GITHUB_RUN_ID` | CLI | Actions run ID |
| `GITHUB_RUN_ATTEMPT` | CLI | Retry attempt number |
| `GITHUB_REF_NAME` | CLI | Branch name |
| `GITHUB_SHA` | CLI | Commit SHA |

## Links

- [Test History Approach RFC (Notion)](https://www.notion.so/nscalecloud/Test-History-Approach-RFC-358caf6bfadc81f1a798e39ef28a97fc)
- [Linear Project (QUA team)](https://linear.app/nscale-workspace/team/QUA/projects/all)
