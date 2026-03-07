// Package parser converts raw Hearthstone log lines into typed game events.
package parser

import "time"

// EventType identifies the kind of game event.
type EventType string

const (
	EventGameStart    EventType = "GAME_START"
	EventGameEnd      EventType = "GAME_END"
	EventTurnStart    EventType = "TURN_START"
	EventEntityUpdate EventType = "ENTITY_UPDATE"
	EventTagChange    EventType = "TAG_CHANGE"
	EventPlayerUpdate EventType = "PLAYER_UPDATE"
	EventZoneChange   EventType = "ZONE_CHANGE"
	EventPlayerDef    EventType = "PLAYER_DEF"  // Player entity definition from CREATE_GAME
	EventPlayerName   EventType = "PLAYER_NAME" // PlayerID → PlayerName mapping
)

// GameEvent is a single parsed event from the Power.log stream.
type GameEvent struct {
	Type        EventType         `json:"type"`
	Timestamp   time.Time         `json:"timestamp"`
	EntityID    int               `json:"entity_id,omitempty"`
	PlayerID    int               `json:"player_id,omitempty"` // CONTROLLER / player= field
	Tags        map[string]string `json:"tags,omitempty"`      // TAG -> VALUE
	EntityName  string            `json:"entity_name,omitempty"`
	CardID      string            `json:"card_id,omitempty"`
	BlockSource int               `json:"block_source,omitempty"` // entity ID from enclosing BLOCK_START
	BlockCardID string            `json:"block_card_id,omitempty"` // card ID from enclosing BLOCK_START
}
