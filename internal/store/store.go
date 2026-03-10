// Package store provides BadgerDB persistence for battlestream.
package store

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	"battlestream.fixates.io/internal/gamestate"
	"battlestream.fixates.io/internal/stats"
)

const (
	prefixGameMeta  = "game:meta:"
	prefixGameState = "game:state:"
	prefixGameList  = "game:list"
	keyAggWins      = "stat:aggregate:wins"
	keyAggLosses    = "stat:aggregate:losses"
	keyAggPlacement = "stat:aggregate:placements"
)

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
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// SaveGame persists a game state and updates aggregate stats.
func (s *Store) SaveGame(meta GameMeta, placement int) error {
	return s.db.Update(func(txn *badger.Txn) error {
		// Save meta
		metaBytes, err := json.Marshal(meta)
		if err != nil {
			return err
		}
		if err := txn.Set([]byte(prefixGameMeta+meta.GameID), metaBytes); err != nil {
			return err
		}

		// Append gameID to list
		listKey := []byte(prefixGameList)
		var ids []string
		item, err := txn.Get(listKey)
		if err == nil {
			if err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &ids)
			}); err != nil {
				return err
			}
		}
		ids = append(ids, meta.GameID)
		listBytes, err := json.Marshal(ids)
		if err != nil {
			return err
		}
		return txn.Set(listKey, listBytes)
	})
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

// HasGame checks if a game with the given ID already exists in the store.
func (s *Store) HasGame(gameID string) bool {
	err := s.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get([]byte(prefixGameMeta + gameID))
		return err
	})
	return err == nil
}

// DropAll deletes all data in the database.
func (s *Store) DropAll() error {
	return s.db.DropAll()
}

// SaveFullGame persists the complete game state snapshot.
func (s *Store) SaveFullGame(gs gamestate.BGGameState) error {
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
	if err := s.SaveGame(meta, gs.Placement); err != nil {
		return err
	}
	// Also store the full state JSON
	return s.db.Update(func(txn *badger.Txn) error {
		data, err := json.Marshal(gs)
		if err != nil {
			return err
		}
		return txn.Set([]byte(prefixGameState+gs.GameID), data)
	})
}

// GetGame retrieves a full game state by ID.
func (s *Store) GetGame(id string) (*gamestate.BGGameState, error) {
	var gs gamestate.BGGameState
	err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(prefixGameState + id))
		if err == badger.ErrKeyNotFound {
			return fmt.Errorf("game %q not found", id)
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

// badgerLogger adapts slog to badger's Logger interface.
type badgerLogger struct{}

func (badgerLogger) Errorf(fmt string, args ...interface{}) {
	slog.Error("badger: "+fmt, args...)
}
func (badgerLogger) Warningf(fmt string, args ...interface{}) {
	slog.Warn("badger: "+fmt, args...)
}
func (badgerLogger) Infof(fmt string, args ...interface{}) {
	slog.Debug("badger: "+fmt, args...)
}
func (badgerLogger) Debugf(fmt string, args ...interface{}) {
	slog.Debug("badger: "+fmt, args...)
}
