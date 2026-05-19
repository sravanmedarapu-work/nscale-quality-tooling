Start the local development environment for nscale-quality-tooling.

Run these steps in order:

1. Start Postgres in the background:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
docker compose up -d postgres
```

2. Wait for Postgres to be ready (check health):
```bash
until docker exec nscale-quality-tooling-postgres-1 pg_isready -U test_history; do sleep 1; done
```

3. Apply migrations:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
PGPASSWORD=test_history psql -h localhost -U test_history -d test_history -f migrations/001_initial_schema.sql
```

4. Build the API binary:
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
go build -o bin/test-history-api ./cmd/test-history-api
```

5. Start the API server (this will run in the foreground — open a separate terminal or background it):
```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling
DATABASE_URL="postgres://test_history:test_history@localhost:5432/test_history?sslmode=disable" \
TEST_HISTORY_TOKEN=dev-token \
PORT=8080 \
./bin/test-history-api
```

6. Verify it's up:
```bash
curl -s http://localhost:8080/healthz
```

Once running:
- API base URL: http://localhost:8080
- Auth token: `dev-token`

To stop: run `docker compose down` in the project directory and kill the API process.
