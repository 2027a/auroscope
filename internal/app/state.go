package app

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type stateStore struct {
	db *sql.DB
}

type packageBaseline struct {
	Commit         string
	ManifestDigest string
}

func openState(path string) (*stateStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	store := &stateStore{db: db}
	if err := store.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *stateStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *stateStore) migrate() error {
	statements := []string{
		`PRAGMA journal_mode=WAL;`,
		`CREATE TABLE IF NOT EXISTS packages (
			pkgbase TEXT PRIMARY KEY,
			last_successful_commit TEXT NOT NULL,
			last_manifest_digest TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS audits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pkgbase TEXT NOT NULL,
			"commit" TEXT NOT NULL,
			previous_commit TEXT,
			codex_json TEXT NOT NULL,
			decision TEXT NOT NULL,
			created_at TEXT NOT NULL
		);`,
	}
	for _, stmt := range statements {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (s *stateStore) baseline(pkgbase string) (*packageBaseline, error) {
	row := s.db.QueryRow(`SELECT last_successful_commit, last_manifest_digest FROM packages WHERE pkgbase = ?`, pkgbase)
	var baseline packageBaseline
	if err := row.Scan(&baseline.Commit, &baseline.ManifestDigest); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &baseline, nil
}

func (s *stateStore) recordAudit(pkgbase string, identity recipeIdentity, previousCommit string, report auditReport, decision string) error {
	raw, err := json.Marshal(report)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(
		`INSERT INTO audits(pkgbase, "commit", previous_commit, codex_json, decision, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		pkgbase, identity.Commit, nullEmpty(previousCommit), string(raw), decision, time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

func (s *stateStore) advanceBaselines(identities []recipeIdentity) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, identity := range identities {
		if _, err := tx.Exec(
			`INSERT INTO packages(pkgbase, last_successful_commit, last_manifest_digest, updated_at)
			 VALUES (?, ?, ?, ?)
			 ON CONFLICT(pkgbase) DO UPDATE SET
			 last_successful_commit=excluded.last_successful_commit,
			 last_manifest_digest=excluded.last_manifest_digest,
			 updated_at=excluded.updated_at`,
			identity.Pkgbase, identity.Commit, identity.ManifestDigest, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *stateStore) tableNames() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return names, nil
}

func nullEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func validateStateIdentity(identity recipeIdentity) error {
	if identity.Pkgbase == "" || !isHex(identity.Commit, 40) || !isHex(identity.ManifestDigest, 64) {
		return fmt.Errorf("invalid identity for state")
	}
	return nil
}
