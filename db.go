package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

var (
	dbMu  sync.Mutex
	appDB *sql.DB
)

func dbPath() string {
	return filepath.Join(managerRoot(), "aigateway.db")
}

// openDB opens (or creates) the SQLite database and applies schema + JSON migration.
func openDB() (*sql.DB, error) {
	dbMu.Lock()
	defer dbMu.Unlock()
	if appDB != nil {
		return appDB, nil
	}
	if err := os.MkdirAll(managerRoot(), 0o755); err != nil {
		return nil, err
	}
	dsn := "file:" + dbPath() + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	if err := migrateSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := migrateJSONIfNeeded(db); err != nil {
		fmt.Fprintf(os.Stderr, "aigateway: json migrate: %v\n", err)
	}
	appDB = db
	return appDB, nil
}

// closeDB closes the global DB (tests / shutdown).
func closeDB() {
	dbMu.Lock()
	defer dbMu.Unlock()
	if appDB != nil {
		_ = appDB.Close()
		appDB = nil
	}
	clearActiveModelCache()
}

func migrateSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS providers (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL,
  base_url        TEXT NOT NULL DEFAULT '',
  api_key         TEXT NOT NULL DEFAULT '',
  color           TEXT NOT NULL DEFAULT '',
  use_proxy       INTEGER,
  format_standard TEXT NOT NULL DEFAULT 'openai',
  enabled         INTEGER NOT NULL DEFAULT 1,
  created_at      TEXT NOT NULL,
  updated_at      TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS provider_models (
  id           TEXT PRIMARY KEY,
  provider_id  TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  model_id     TEXT NOT NULL,
  name         TEXT NOT NULL DEFAULT '',
  enabled      INTEGER NOT NULL DEFAULT 1,
  is_default   INTEGER NOT NULL DEFAULT 0,
  owned_by     TEXT NOT NULL DEFAULT '',
  UNIQUE(provider_id, model_id)
);

CREATE TABLE IF NOT EXISTS token_packages (
  id            TEXT PRIMARY KEY,
  provider_id   TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  name          TEXT NOT NULL DEFAULT '',
  total_tokens  INTEGER NOT NULL DEFAULT 0,
  used_offset   INTEGER NOT NULL DEFAULT 0,
  price         REAL NOT NULL DEFAULT 0,
  currency      TEXT NOT NULL DEFAULT 'CNY',
  start_at      TEXT NOT NULL DEFAULT '',
  expire_at     TEXT NOT NULL DEFAULT '',
  note          TEXT NOT NULL DEFAULT '',
  active        INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS model_groups (
  id         TEXT PRIMARY KEY,
  name       TEXT NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 1,
  strategy   TEXT NOT NULL DEFAULT 'priority',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS model_group_routes (
  id                TEXT PRIMARY KEY,
  group_id          TEXT NOT NULL REFERENCES model_groups(id) ON DELETE CASCADE,
  provider_id       TEXT NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
  provider_model_id TEXT NOT NULL,
  priority          INTEGER NOT NULL DEFAULT 100,
  enabled           INTEGER NOT NULL DEFAULT 1,
  status            TEXT NOT NULL DEFAULT 'ok',
  used_tokens       INTEGER NOT NULL DEFAULT 0,
  UNIQUE(group_id, provider_id, provider_model_id)
);

CREATE TABLE IF NOT EXISTS usage_events (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  time          TEXT NOT NULL,
  provider_id   TEXT NOT NULL DEFAULT '',
  provider_name TEXT NOT NULL DEFAULT '',
  model         TEXT NOT NULL DEFAULT '',
  group_id      TEXT NOT NULL DEFAULT '',
  endpoint      TEXT NOT NULL DEFAULT '',
  status        INTEGER NOT NULL DEFAULT 0,
  input_tokens  INTEGER NOT NULL DEFAULT 0,
  output_tokens INTEGER NOT NULL DEFAULT 0,
  total_tokens  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_usage_time ON usage_events(time);
CREATE INDEX IF NOT EXISTS idx_usage_model ON usage_events(model);
CREATE INDEX IF NOT EXISTS idx_routes_group ON model_group_routes(group_id, priority);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	_, _ = db.Exec(`INSERT OR IGNORE INTO meta(key, value) VALUES ('schema_version', '1')`)
	_, _ = db.Exec(`INSERT OR IGNORE INTO meta(key, value) VALUES ('created_at', ?)`, time.Now().UTC().Format(time.RFC3339))
	return nil
}

func metaGet(db *sql.DB, key string) string {
	var v string
	_ = db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	return v
}

func metaSet(db *sql.DB, key, value string) error {
	_, err := db.Exec(`INSERT INTO meta(key, value) VALUES(?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func intToBool(v int) bool { return v != 0 }

func useProxyToSQL(p *bool) any {
	if p == nil {
		return nil
	}
	if *p {
		return 1
	}
	return 0
}

func useProxyFromSQL(v sql.NullInt64) *bool {
	if !v.Valid {
		return nil
	}
	b := v.Int64 != 0
	return &b
}
