Run the test suite for nscale-quality-tooling.

Run unit tests (no Docker required):
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go test -count=1 -timeout 60s ./internal/... ./cmd/...
```

If any tests fail, show the full output with verbose flag:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go test -v -count=1 -timeout 60s ./internal/... ./cmd/...
```

To run integration tests (requires Docker — starts a real Postgres via testcontainers):
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go test -count=1 -timeout 120s -tags integration ./internal/store/...
```

To run a specific package:
```bash
# Parser tests
go test -v ./internal/parser/playwright/...
go test -v ./internal/parser/ginkgo/...
go test -v ./internal/parser/junit/...

# Handler tests
go test -v ./cmd/test-history-api/handler/...

# CLI ingest tests
go test -v ./cmd/nscale-test-history/ingest/...
```

Report the number of tests passed/failed and any failure details.
