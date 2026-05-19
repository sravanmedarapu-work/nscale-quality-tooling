.PHONY: build test test-unit test-integration lint fmt dev down migrate seed clean install-hooks

MODULE := github.com/sravanmedarapu-work/nscale-quality-tooling
BIN     := bin

build:
	go build -o $(BIN)/test-history-api    ./cmd/test-history-api
	go build -o $(BIN)/nscale-test-history ./cmd/nscale-test-history

test-unit:
	go test -count=1 -timeout 60s ./internal/... ./cmd/...

test-integration:
	go test -count=1 -timeout 120s -tags integration ./...

test: test-unit

fmt:
	gofmt -w .

lint:
	go vet ./...

dev:
	docker compose up -d postgres
	@echo "Waiting for Postgres..." && sleep 2
	$(MAKE) migrate
	TEST_HISTORY_TOKEN=dev-token DATABASE_URL="postgres://test_history:test_history@localhost:5432/test_history?sslmode=disable" \
		go run ./cmd/test-history-api

down:
	docker compose down -v

migrate:
	@for f in migrations/*.sql; do \
		echo "Applying $$f"; \
		PGPASSWORD=test_history psql -h localhost -U test_history -d test_history -f "$$f"; \
	done

seed:
	go run ./cmd/nscale-test-history ingest \
		--suite console-e2e --framework playwright --env dev \
		--junit testdata/fixtures/playwright-junit.xml \
		--json  testdata/fixtures/playwright-results.json

install-hooks:
	cp scripts/hooks/pre-commit .git/hooks/pre-commit
	chmod +x .git/hooks/pre-commit
	@echo "pre-commit hook installed"

clean:
	rm -rf $(BIN)
