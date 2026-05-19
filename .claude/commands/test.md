# /test — Run all tests

Runs the full unit test suite (no Docker required).

```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go test -count=1 -timeout 60s ./internal/... ./cmd/...
```

To run integration tests (requires Docker — starts a real Postgres via testcontainers):

```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go test -count=1 -timeout 120s -tags integration ./internal/store/...
```

To run a specific package:

```bash
go test -v ./internal/parser/playwright/...
go test -v ./cmd/test-history-api/handler/...
go test -v ./cmd/nscale-test-history/ingest/...
```
