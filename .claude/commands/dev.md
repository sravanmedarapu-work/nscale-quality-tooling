# /dev — Start local development environment

Starts Postgres via Docker Compose, applies migrations, and runs the API server.

```bash
cd /Users/fqw4m72pvl/src/nscale/qa/nscale-quality-tooling

# Start Postgres
docker compose up -d postgres
echo "Waiting for Postgres to be ready..."
sleep 3

# Apply migrations
docker exec -i nscale-quality-tooling-postgres-1 psql -U test_history -d test_history < migrations/001_initial_schema.sql

# Build the API binary
go build -o bin/test-history-api ./cmd/test-history-api

# Start API server
DATABASE_URL="postgres://test_history:test_history@localhost:5432/test_history?sslmode=disable" \
TEST_HISTORY_TOKEN=dev-token \
PORT=8080 \
./bin/test-history-api
```

Once running:
- API: http://localhost:8080
- Health: http://localhost:8080/healthz
- Token: `dev-token`

To stop: `docker compose down` and kill the API process.
