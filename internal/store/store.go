// Package store provides BadgerDB persistence for battlestream.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/stats"
	"battlestream.fixates.io/internal/store/migrations"
)

const (
	prefixGameMeta  = "game:meta:"
	prefixGameState = "game:state:"
	prefixGameList  = "game:list"
	prefixGameTurns = "game:turns:"
)

// ErrEmptyGameID is returned when a save is attempted with an empty GameID,
// which would otherwise write records under bare prefix keys.
var ErrEmptyGameID = errors.New("store: empty GameID")

// ErrGameNotFound is returned (wrapped) by GetGame when no record exists for
// the requested GameID. Callers must use errors.Is to distinguish a missing
// game from a real database failure (e.g. corruption, closed DB).
var ErrGameNotFound = errors.New("store: game not found")

// Store wraps a BadgerDB instance.
type Store struct {
	db *badger.DB
}

// GameMeta holds lightweight game metadata for list views.
type GameMeta struct {
	GameID    string `json:"game_id"`
	StartTime int64  `json:"start_time_unix"`
	EndTime   int64  `json:"end_time_unix,omitempty"`
	Placement int    `json:"placement"`
	IsDuos    bool   `json:"is_duos,omitempty"`
}

// Open opens (or creates) the BadgerDB at the given path.
func Open(path string) (*Store, error) {
	opts := badger.DefaultOptions(path).
		WithLogger(badgerLogger{}).
		WithCompactL0OnClose(true)

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("opening badger at %s: %w", path, err)
	}
	if err := migrations.Run(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveGame persists game metadata and appends the GameID to the game list.
// The save is idempotent per GameID: if the ID is already in the list, the
// metadata is overwritten in place and the list is left unchanged, so
// duplicate saves never double-count a game. An empty GameID is rejected
// with ErrEmptyGameID.
func (s *Store) SaveGame(meta GameMeta) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return saveGameTxn(txn, meta)
	})
}

// saveGameTxn writes the game metadata and updates the game list inside an
// existing transaction.
//
// Any failure reading or decoding the existing game list aborts the
// transaction with an error. A corrupt or unreadable list must never be
// silently replaced with a fresh one-element list — that would orphan all
// prior game history. Only badger.ErrKeyNotFound is treated as "no list
// exists yet".
func saveGameTxn(txn *badger.Txn, meta GameMeta) error {
	if meta.GameID == "" {
		return ErrEmptyGameID
	}

	// Save meta (idempotent overwrite).
	metaBytes, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	if err := txn.Set([]byte(prefixGameMeta+meta.GameID), metaBytes); err != nil {
		return err
	}

	// Load the existing game list.
	listKey := []byte(prefixGameList)
	var ids []string
	item, err := txn.Get(listKey)
	switch {
	case err == nil:
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &ids)
		}); err != nil {
			return fmt.Errorf("decoding game list (refusing to overwrite): %w", err)
		}
	case errors.Is(err, badger.ErrKeyNotFound):
		// First game: start a new list.
	default:
		return fmt.Errorf("reading game list: %w", err)
	}

	// In-transaction dedup: never append the same GameID twice. This is the
	// authoritative duplicate guard; HasGame checks at call sites are only an
	// optimization.
	for _, id := range ids {
		if id == meta.GameID {
			return nil
		}
	}
	ids = append(ids, meta.GameID)
	listBytes, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return txn.Set(listKey, listBytes)
}

// GetAggregate loads all game metadata and delegates computation to the stats package.
func (s *Store) GetAggregate() (stats.AggregateStats, error) {
	metas, err := s.loadAllMetas()
	if err != nil {
		return stats.AggregateStats{}, err
	}
	results := make([]stats.GameResult, len(metas))
	for i, m := range metas {
		results[i] = stats.GameResult{
			Placement: m.Placement,
			EndTime:   time.Unix(m.EndTime, 0),
			IsDuos:    m.IsDuos,
		}
	}
	return stats.Compute(results), nil
}

// loadAllMetas returns all stored GameMeta records in insertion order.
func (s *Store) loadAllMetas() ([]GameMeta, error) {
	var metas []GameMeta
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixGameList))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		var ids []string
		if err := item.Value(func(val []byte) error {
			return json.Unmarshal(val, &ids)
		}); err != nil {
			return err
		}
		for _, id := range ids {
			metaItem, err := txn.Get([]byte(prefixGameMeta + id))
			if err != nil {
				continue
			}
			var meta GameMeta
			if err := metaItem.Value(func(val []byte) error {
				return json.Unmarshal(val, &meta)
			}); err != nil {
				continue
			}
			metas = append(metas, meta)
		}
		return nil
	})
	return metas, err
}

// ListGames returns game metadata newest-first with optional pagination.
// limit=0 returns all records.
func (s *Store) ListGames(limit, offset int) ([]GameMeta, error) {
	all, err := s.loadAllMetas()
	if err != nil {
		return nil, err
	}
	// Reverse: newest first.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if offset >= len(all) {
		return nil, nil
	}
	all = all[offset:]
	if limit > 0 && limit < len(all) {
		all = all[:limit]
	}
	return all, nil
}

// FilterCompleteGames returns only games with a recorded placement
// (placement > 0), excluding stale/incomplete games. Shared by the REST and
// gRPC list handlers so both APIs return the same game set.
func FilterCompleteGames(metas []GameMeta) []GameMeta {
	out := make([]GameMeta, 0, len(metas))
	for _, m := range metas {
		if m.Placement > 0 {
			out = append(out, m)
		}
	}
	return out
}

// HasGame reports whether a game with the given ID already exists in the
// store. The error is non-nil only for real database failures (not for a
// missing key); callers may treat (false, non-nil) as "unknown" — duplicate
// prevention is enforced inside SaveGame/SaveFullGame regardless, so HasGame
// is only an optimization at call sites.
func (s *Store) HasGame(gameID string) (bool, error) {
	if gameID == "" {
		return false, nil
	}
	var found bool
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(prefixGameMeta + gameID))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		found = true
		return nil
	})
	return found, err
}

// DropAll deletes all data in the database.
func (s *Store) DropAll() error {
	return s.db.DropAll()
}

// SaveFullGame persists the game metadata, game-list entry, and full state
// snapshot in a single transaction, so a crash can never leave a listed game
// without a retrievable state. Like SaveGame, it is idempotent per GameID and
// rejects an empty GameID with ErrEmptyGameID.
func (s *Store) SaveFullGame(gs gamestate.BGGameState) error {
	if gs.GameID == "" {
		return ErrEmptyGameID
	}
	endTime := int64(0)
	if gs.EndTime != nil {
		endTime = gs.EndTime.Unix()
	}
	meta := GameMeta{
		GameID:    gs.GameID,
		StartTime: gs.StartTime.Unix(),
		EndTime:   endTime,
		Placement: gs.Placement,
		IsDuos:    gs.IsDuos,
	}
	stateBytes, err := json.Marshal(gs)
	if err != nil {
		return err
	}
	return s.db.Update(func(txn *badger.Txn) error {
		if err := saveGameTxn(txn, meta); err != nil {
			return err
		}
		return txn.Set([]byte(prefixGameState+gs.GameID), stateBytes)
	})
}

// GetGame retrieves a full game state by ID.
func (s *Store) GetGame(id string) (*gamestate.BGGameState, error) {
	var gs gamestate.BGGameState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixGameState + id))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("game %q: %w", id, ErrGameNotFound)
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &gs)
		})
	})
	if err != nil {
		return nil, err
	}
	return &gs, nil
}

// SaveTurnSnapshots persists per-turn snapshots for a game.
func (s *Store) SaveTurnSnapshots(gameID string, snapshots []gamestate.TurnSnapshot) error {
	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(snapshots)
		if err != nil {
			return err
		}
		return txn.Set([]byte(prefixGameTurns+gameID), data)
	})
}

// GetTurnSnapshots retrieves per-turn snapshots for a game.
func (s *Store) GetTurnSnapshots(gameID string) ([]gamestate.TurnSnapshot, error) {
	var snapshots []gamestate.TurnSnapshot
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixGameTurns + gameID))
		if err == badger.ErrKeyNotFound {
			return nil
		}
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &snapshots)
		})
	})
	return snapshots, err
}

// badgerLogger adapts slog to badger's Logger interface.
type badgerLogger struct{}

func (badgerLogger) Errorf(format string, args ...interface{}) {
	slog.Error("badger: " + fmt.Sprintf(format, args...))
}
func (badgerLogger) Warningf(format string, args ...interface{}) {
	slog.Warn("badger: " + fmt.Sprintf(format, args...))
}
func (badgerLogger) Infof(format string, args ...interface{}) {
	slog.Debug("badger: " + fmt.Sprintf(format, args...))
}
func (badgerLogger) Debugf(format string, args ...interface{}) {
	slog.Debug("badger: " + fmt.Sprintf(format, args...))
}
