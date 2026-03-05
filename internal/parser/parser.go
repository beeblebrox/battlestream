package parser

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Parser parses raw Power.log lines into GameEvents.
type Parser struct {
	out chan<- GameEvent

	// state for block parsing
	inBlock    bool
	blockType  EventType
	entityID   int
	entityName string
	cardID     string
	blockTags  map[string]string
	blockTime  time.Time
}

// New creates a Parser that sends events to out.
func New(out chan<- GameEvent) *Parser {
	return &Parser{out: out}
}

// Power.log regex patterns
var (
	// D 21:11:50.1234567 GameState.DebugPrintPower() - CREATE_GAME
	reCreateGame = regexp.MustCompile(`CREATE_GAME`)
	// GameState.DebugPrintGame() - ... TURN=7
	reTurnStart = regexp.MustCompile(`TAG_CHANGE\s+Entity=GameEntity\s+tag=TURN\s+value=(\d+)`)
	// TAG_CHANGE Entity=... tag=GAME_RESULT value=LOSS
	reGameResult = regexp.MustCompile(`TAG_CHANGE\s+Entity=(\S+)\s+tag=GAME_RESULT\s+value=(\S+)`)
	// FULL_ENTITY - Updating Entity=... CardID=...
	reFullEntity = regexp.MustCompile(`(?:FULL_ENTITY|SHOW_ENTITY)\s+-\s+(?:Creating|Updating)\s+Entity=(.+?)\s+CardID=(\S*)`)
	// entityID from bracketed notation: [entityName=... id=42 ...]
	reEntityID = regexp.MustCompile(`\bid=(\d+)\b`)
	// TAG_CHANGE Entity=... tag=... value=...
	reTagChange = regexp.MustCompile(`TAG_CHANGE\s+Entity=(.+?)\s+tag=(\S+)\s+value=(\S+)`)
	// tag=X value=Y within a block
	reBlockTag = regexp.MustCompile(`\s+tag=(\S+)\s+value=(\S+)`)
	// Timestamp prefix like "D 21:11:50.1234567 " or "W " etc.
	reTimestamp = regexp.MustCompile(`^[DWIE]\s+(\d{2}:\d{2}:\d{2}\.\d+)\s+`)
)

// Feed processes a single raw log line.
func (p *Parser) Feed(line string) {
	ts := extractTimestamp(line)
	line = stripTimestamp(line)

	switch {
	case reCreateGame.MatchString(line):
		p.emit(GameEvent{
			Type:      EventGameStart,
			Timestamp: ts,
			Tags:      map[string]string{},
		})

	case reTurnStart.MatchString(line):
		m := reTurnStart.FindStringSubmatch(line)
		turn, _ := strconv.Atoi(m[1])
		p.emit(GameEvent{
			Type:      EventTurnStart,
			Timestamp: ts,
			Tags:      map[string]string{"TURN": strconv.Itoa(turn)},
		})

	case reGameResult.MatchString(line):
		m := reGameResult.FindStringSubmatch(line)
		p.emit(GameEvent{
			Type:      EventGameEnd,
			Timestamp: ts,
			EntityName: m[1],
			Tags:      map[string]string{"GAME_RESULT": m[2]},
		})

	case reFullEntity.MatchString(line):
		m := reFullEntity.FindStringSubmatch(line)
		entityDesc := m[1]
		cardID := m[2]
		id := extractEntityID(entityDesc)
		p.emit(GameEvent{
			Type:      EventEntityUpdate,
			Timestamp: ts,
			EntityID:  id,
			EntityName: strings.TrimSpace(entityDesc),
			CardID:    cardID,
			Tags:      map[string]string{},
		})

	case reTagChange.MatchString(line):
		m := reTagChange.FindStringSubmatch(line)
		entity := strings.TrimSpace(m[1])
		tag := m[2]
		value := m[3]
		id := extractEntityID(entity)
		p.emit(GameEvent{
			Type:      EventTagChange,
			Timestamp: ts,
			EntityID:  id,
			EntityName: entity,
			Tags:      map[string]string{tag: value},
		})
	}
}

func (p *Parser) emit(e GameEvent) {
	if p.out != nil {
		p.out <- e
	}
}

func extractTimestamp(line string) time.Time {
	m := reTimestamp.FindStringSubmatch(line)
	if m == nil {
		return time.Now()
	}
	// Parse HH:MM:SS.nnnnnnn
	now := time.Now()
	t, err := time.Parse("15:04:05.9999999", m[1])
	if err != nil {
		return now
	}
	return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), now.Location())
}

func stripTimestamp(line string) string {
	loc := reTimestamp.FindStringIndex(line)
	if loc == nil {
		return line
	}
	return line[loc[1]:]
}

func extractEntityID(s string) int {
	m := reEntityID.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	id, _ := strconv.Atoi(m[1])
	return id
}
