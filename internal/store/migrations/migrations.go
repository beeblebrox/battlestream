// Package migrations provides forward-only BadgerDB schema versioning for battlestream.
//
// Each migration is a pure function that receives an open *badger.DB and performs
// whatever schema changes are needed. Migrations are keyed by version number and
// run in ascending order automatically when the store is opened.
//
// Adding a new migration:
//  1. Increment latestVersion.
//  2. Add a case for the new version in the registry below.
//  3. The function must be idempotent — partial data is acceptable; the version key
//     is only written after the function returns nil.
package migrations

import (
	"encoding/binary"
	"fmt"
	"log/slog"

	badger "github.com/dgraph-io/badger/v4"
)

const (
	keyDBVersion  = "meta:db_version"
	latestVersion = 1
)

// Run applies any pending forward migrations to db and returns the final version.
// It is safe to call on a brand-new database or one already at latestVersion.
func Run(db *badger.DB) error {
	current, err := readVersion(db)
	if err != nil {
		return fmt.Errorf("migrations: reading version: %w", err)
	}

	if current == latestVersion {
		return nil
	}

	for v := current + 1; v <= latestVersion; v++ {
		slog.Info("migrations: applying", "version", v)
		if err := apply(db, v); err != nil {
			return fmt.Errorf("migrations: applying v%d: %w", v, err)
		}
		if err := writeVersion(db, v); err != nil {
			return fmt.Errorf("migrations: writing version after v%d: %w", v, err)
		}
		slog.Info("migrations: applied", "version", v)
	}
	return nil
}

// apply dispatches migration functions by version number.
func apply(db *badger.DB, version int) error {
	switch version {
	case 1:
		return migrate1(db)
	default:
		return fmt.Errorf("unknown migration version %d", version)
	}
}

// migrate1 is a no-op that establishes the versioned schema baseline.
// All existing databases are considered to be at version 0 (un-versioned) and
// will be upgraded to version 1 on first open, which simply records the version.
func migrate1(_ *badger.DB) error {
	return nil
}

// readVersion returns the current schema version stored in the database.
// A missing key (fresh or pre-migration database) returns 0.
func readVersion(db *badger.DB) (int, error) {
	var version int
	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(keyDBVersion))
		if err == badger.ErrKeyNotFound {
			version = 0
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			if len(val) != 8 {
				return fmt.Errorf("unexpected version length %d", len(val))
			}
			version = int(binary.BigEndian.Uint64(val))
			return nil
		})
	})
	return version, err
}

// writeVersion persists the current schema version.
func writeVersion(db *badger.DB, version int) error {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(version))
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(keyDBVersion), buf)
	})
}
