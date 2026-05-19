CREATE TABLE IF NOT EXISTS test_case_attempts (
  event_id                TEXT        PRIMARY KEY,
  repo                    TEXT        NOT NULL,
  suite                   TEXT        NOT NULL,
  framework               TEXT        NOT NULL,
  env                     TEXT        NOT NULL,
  branch                  TEXT,
  commit_sha              TEXT,
  run_id                  TEXT        NOT NULL,
  run_attempt             INTEGER     NOT NULL DEFAULT 1,
  test_id                 TEXT        NOT NULL,
  test_name               TEXT,
  status                  TEXT        NOT NULL,
  duration_ms             INTEGER,
  attempt_index           INTEGER     NOT NULL DEFAULT 0,
  failure_category        TEXT,
  failure_fingerprint     TEXT,
  failure_message_excerpt TEXT,
  artifact_url            TEXT,
  pr_number               INTEGER,
  started_at              TIMESTAMPTZ NOT NULL,
  inserted_at             TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS test_case_attempts_lookup_idx
  ON test_case_attempts (repo, suite, env, test_id, started_at DESC);

CREATE INDEX IF NOT EXISTS test_case_attempts_dashboard_idx
  ON test_case_attempts (repo, suite, env, started_at DESC, status);
