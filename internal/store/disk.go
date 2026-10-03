package store

import (
	"database/sql"
	"fmt"
	"time"
)

const scannedKey = "disk_scanned_at"

// DiskFacts are things read from the music folder, cached because a scan
// takes long on a network share.
type DiskFacts struct {
	Sizes  map[string]int64  // file path to bytes, 0 when the file is missing
	Covers map[string]string // folder to cover file name, "" when it has none
}

// DiskFacts returns the cached facts and when they were last fully scanned.
// The time is zero when no scan happened yet.
func (s *Store) DiskFacts() (DiskFacts, time.Time, error) {
	f := DiskFacts{Sizes: map[string]int64{}, Covers: map[string]string{}}

	rows, err := s.db.Query(`SELECT f.path, f.bytes FROM file_sizes f`)
	if err != nil {
		return f, time.Time{}, fmt.Errorf("query file sizes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var p string
		var n int64
		if err = rows.Scan(&p, &n); err != nil {
			return f, time.Time{}, fmt.Errorf("scan file size: %w", err)
		}
		f.Sizes[p] = n
	}
	if err = rows.Err(); err != nil {
		return f, time.Time{}, err
	}

	crows, err := s.db.Query(`SELECT c.dir, c.file FROM dir_covers c`)
	if err != nil {
		return f, time.Time{}, fmt.Errorf("query covers: %w", err)
	}
	defer crows.Close()
	for crows.Next() {
		var d, name string
		if err = crows.Scan(&d, &name); err != nil {
			return f, time.Time{}, fmt.Errorf("scan cover: %w", err)
		}
		f.Covers[d] = name
	}
	if err = crows.Err(); err != nil {
		return f, time.Time{}, err
	}

	var raw string
	err = s.db.QueryRow(`SELECT m.value FROM meta m WHERE m.key = ?`, scannedKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return f, time.Time{}, nil
	}
	if err != nil {
		return f, time.Time{}, fmt.Errorf("query scan time: %w", err)
	}
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return f, time.Time{}, fmt.Errorf("parse scan time: %w", err)
	}
	return f, at, nil
}

// SaveDiskFacts stores facts. With replace, the old facts are dropped and the
// scan time is set to now: use it after a full scan.
func (s *Store) SaveDiskFacts(f DiskFacts, replace bool, now time.Time) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback()

	if replace {
		for _, q := range []string{`DELETE FROM file_sizes`, `DELETE FROM dir_covers`} {
			if _, err = tx.Exec(q); err != nil {
				return fmt.Errorf("clear disk facts: %w", err)
			}
		}
		if _, err = tx.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, scannedKey, now.UTC().Format(time.RFC3339)); err != nil {
			return fmt.Errorf("save scan time: %w", err)
		}
	}
	for p, n := range f.Sizes {
		if _, err = tx.Exec(`INSERT OR REPLACE INTO file_sizes (path, bytes) VALUES (?, ?)`, p, n); err != nil {
			return fmt.Errorf("save file size: %w", err)
		}
	}
	for d, name := range f.Covers {
		if _, err = tx.Exec(`INSERT OR REPLACE INTO dir_covers (dir, file) VALUES (?, ?)`, d, name); err != nil {
			return fmt.Errorf("save cover: %w", err)
		}
	}
	return tx.Commit()
}
