package capture

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	name    string
	fn      func(tx *sql.Tx) error
}

// migrations is the ordered list of schema migrations.
// IMPORTANT: Only append to this slice. Never remove or reorder entries.
// Columns should only be added, never removed.
// JSON columns (board_json, buff_sources_json) provide flexibility for
// evolving nested structures without schema changes.
var migrations = []migration{
	{version: 1, name: "initial schema", fn: migrateV1},
}

func migrateV1(tx *sql.Tx) error {
	stmts := []string{
		`CREATE TABLE games (
			game_id      TEXT PRIMARY KEY,
			start_time   TEXT NOT NULL,
			end_time     TEXT,
			is_duos      BOOLEAN NOT NULL DEFAULT 0,
			placement    INTEGER DEFAULT 0,
			total_frames INTEGER DEFAULT 0
		)`,
		`CREATE TABLE frames (
			id                 INTEGER PRIMARY KEY AUTOINCREMENT,
			game_id            TEXT NOT NULL REFERENCES games(game_id),
			sequence           INTEGER NOT NULL,
			timestamp          TEXT NOT NULL,
			elapsed_ms         INTEGER NOT NULL,
			file_path          TEXT NOT NULL,
			file_size_bytes    INTEGER NOT NULL,
			capture_latency_ms INTEGER NOT NULL,
			turn               INTEGER NOT NULL,
			phase              TEXT NOT NULL,
			tavern_tier        INTEGER NOT NULL,
			health             INTEGER NOT NULL,
			armor              INTEGER NOT NULL,
			gold               INTEGER NOT NULL,
			placement          INTEGER NOT NULL DEFAULT 0,
			is_duos            BOOLEAN NOT NULL DEFAULT 0,
			partner_health     INTEGER,
			partner_tier       INTEGER,
			board_json         TEXT NOT NULL,
			buff_sources_json  TEXT NOT NULL,
			UNIQUE(game_id, sequence)
		)`,
		`CREATE INDEX idx_frames_game_turn ON frames(game_id, turn)`,
		`CREATE INDEX idx_frames_game_phase ON frames(game_id, phase)`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("exec %q: %w", s[:40], err)
		}
	}
	return nil
}

func ensureSchema(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("create schema_version: %w", err)
	}

	var current int
	row := db.QueryRow(`SELECT version FROM schema_version LIMIT 1`)
	if err := row.Scan(&current); err != nil {
		// No row = fresh DB, version 0.
		current = 0
	}

	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d (%s): %w", m.version, m.name, err)
		}
		if err := m.fn(tx); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d (%s): %w", m.version, m.name, err)
		}
		if current == 0 {
			_, err = tx.Exec(`INSERT INTO schema_version (version) VALUES (?)`, m.version)
		} else {
			_, err = tx.Exec(`UPDATE schema_version SET version = ?`, m.version)
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("update schema_version to %d: %w", m.version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d: %w", m.version, err)
		}
		current = m.version
	}
	return nil
}
