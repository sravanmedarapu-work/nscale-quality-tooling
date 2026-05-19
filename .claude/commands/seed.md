Ingest sample fixture data into the local API and verify it landed correctly.

Requires the local dev environment to be running (`/dev`).

1. Build the CLI binary first:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go build -o bin/nscale-test-history ./cmd/nscale-test-history
```

2. Ingest the Playwright fixture:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
GITHUB_REPOSITORY=org/nscale-ui \
GITHUB_RUN_ID=seed-$(date +%s) \
GITHUB_RUN_ATTEMPT=1 \
GITHUB_REF_NAME=main \
GITHUB_SHA=abc123 \
TEST_HISTORY_API_URL=http://localhost:8080 \
TEST_HISTORY_TOKEN=dev-token \
./bin/nscale-test-history ingest \
  --suite console-e2e \
  --framework playwright \
  --env dev \
  --json testdata/fixtures/playwright-results.json \
  --junit testdata/fixtures/playwright-junit.xml
```

3. Ingest the Ginkgo fixture:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
GITHUB_REPOSITORY=org/uni-region \
GITHUB_RUN_ID=seed-$(date +%s) \
GITHUB_RUN_ATTEMPT=1 \
GITHUB_REF_NAME=main \
GITHUB_SHA=def456 \
TEST_HISTORY_API_URL=http://localhost:8080 \
TEST_HISTORY_TOKEN=dev-token \
./bin/nscale-test-history ingest \
  --suite uni-region-api \
  --framework ginkgo \
  --env dev \
  --json testdata/fixtures/ginkgo-results.json
```

4. Verify the data landed — check row counts in Postgres:
```bash
docker exec nscale-quality-tooling-postgres-1 psql -U test_history -d test_history \
  -c "SELECT suite, framework, status, count(*) FROM test_case_attempts GROUP BY 1,2,3 ORDER BY 1,2,3;"
```

5. Query the API to confirm results are queryable:
```bash
# History for one test
curl -s "http://localhost:8080/v1/tests/history?repo=org/nscale-ui&suite=console-e2e&env=dev&test_id=tests/network/vpc.spec.ts::create%20and%20delete%20VPC&window=30d" \
  -H "Authorization: Bearer dev-token" | python3 -m json.tool

# Trends for the suite
curl -s "http://localhost:8080/v1/tests/trends?repo=org/nscale-ui&suite=console-e2e&env=dev&window=30d" \
  -H "Authorization: Bearer dev-token" | python3 -m json.tool
```

Report what was seeded (row counts) and whether the API queries returned data.
