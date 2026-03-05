package parser

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Parser parses raw Power.log lines into GameEvents.
// It maintains a small block state for FULL_ENTITY / SHOW_ENTITY entries whose
// ATK/HEALTH tags appear on subsequent indented lines.
type Parser struct {
	out     chan<- GameEvent
	inBlock bool
	pending GameEvent
}

// New creates a Parser that sends events to out.
func New(out chan<- GameEvent) *Parser {
	return &Parser{out: out}
}

// Power.log regex patterns
var (
	// Only process lines from GameState.DebugPrintPower / DebugPrintGame.
	// Skip PowerTaskList lines to avoid processing duplicate events.
	reGameStateSource = regexp.MustCompile(`GameState\.DebugPrint(?:Power|Game)\(\)`)

	// D 21:11:50.1234567 GameState.DebugPrintPower() - CREATE_GAME
	reCreateGame = regexp.MustCompile(`CREATE_GAME`)

	// Player EntityID=20 PlayerID=7 GameAccountId=[hi=144115193835963207 lo=30722021]
	rePlayerDef = regexp.MustCompile(`Player\s+EntityID=(\d+)\s+PlayerID=(\d+)\s+GameAccountId=\[hi=(\d+)\s+lo=(\d+)\]`)

	// GameState.DebugPrintGame() - PlayerID=7, PlayerName=Moch#1358
	rePlayerNameLine = regexp.MustCompile(`PlayerID=(\d+),\s+PlayerName=(.+)`)

	// TAG_CHANGE Entity=GameEntity tag=TURN value=7
	reTurnStart = regexp.MustCompile(`TAG_CHANGE\s+Entity=GameEntity\s+tag=TURN\s+value=(\d+)`)

	// TAG_CHANGE Entity=<PlayerName> tag=TURN value=3  (player-specific turn)
	rePlayerTurn = regexp.MustCompile(`TAG_CHANGE\s+Entity=(\S+?)\s+tag=TURN\s+value=(\d+)\s`)

	// TAG_CHANGE Entity=GameEntity tag=STATE value=COMPLETE
	reGameComplete = regexp.MustCompile(`TAG_CHANGE\s+Entity=GameEntity\s+tag=STATE\s+value=COMPLETE`)

	// FULL_ENTITY / SHOW_ENTITY block header
	// "FULL_ENTITY - Creating ID=75 CardID=TB_BaconShop_HERO_49"
	// "FULL_ENTITY - Creating Entity=Murloc Tidehunter CardID=EX1_506"
	reFullEntity = regexp.MustCompile(`(?:FULL_ENTITY|SHOW_ENTITY)\s+-\s+(?:Creating|Updating)\s+(?:ID=(\d+)\s+)?(?:Entity=(.+?)\s+)?CardID=(\S*)`)

	// entityID from bracketed notation: [entityName=... id=42 ...]
	reEntityID = regexp.MustCompile(`\bid=(\d+)\b`)

	// player= field from bracketed entity notation: [... player=7]
	rePlayerField = regexp.MustCompile(`\bplayer=(\d+)\b`)

	// TAG_CHANGE Entity=... tag=... value=...
	reTagChange = regexp.MustCompile(`TAG_CHANGE\s+Entity=(.+?)\s+tag=(\S+)\s+value=(\S+)`)

	// Timestamp prefix like "D 21:11:50.1234567 " or "W " etc.
	reTimestamp = regexp.MustCompile(`^[DWIE]\s+(\d{2}:\d{2}:\d{2}\.\d+)\s+`)

	// Indented block tag lines:
	//   "GameState.DebugPrintPower() -     tag=ATK value=2"
	// Requires at least 4 spaces after the " - " separator.
	reBlockTag = regexp.MustCompile(`-\s{4,}tag=(\S+)\s+value=(\S*)`)
)

// Feed processes a single raw log line.
func (p *Parser) Feed(line string) {
	// Only process GameState.DebugPrintPower/Game lines.
	// Skip PowerTaskList to avoid duplicate events.
	if !reGameStateSource.MatchString(line) {
		return
	}

	ts := extractTimestamp(line)
	stripped := stripTimestamp(line)

	// If we're inside a FULL_ENTITY / SHOW_ENTITY block, check for
	// continuation tag lines before processing any other patterns.
	if p.inBlock {
		if m := reBlockTag.FindStringSubmatch(stripped); m != nil {
			p.pending.Tags[m[1]] = m[2]
			return
		}
		// Non-continuation line: flush the accumulated entity event.
		p.flushPending()
	}

	switch {
	case reCreateGame.MatchString(stripped):
		p.emit(GameEvent{
			Type:      EventGameStart,
			Timestamp: ts,
			Tags:      map[string]string{},
		})

	case rePlayerDef.MatchString(stripped):
		m := rePlayerDef.FindStringSubmatch(stripped)
		entityID, _ := strconv.Atoi(m[1])
		playerID, _ := strconv.Atoi(m[2])
		hi := m[3]
		lo := m[4]
		p.emit(GameEvent{
			Type:      EventPlayerDef,
			Timestamp: ts,
			EntityID:  entityID,
			PlayerID:  playerID,
			Tags: map[string]string{
				"hi":        hi,
				"lo":        lo,
				"PLAYER_ID": m[2],
			},
		})

	case rePlayerNameLine.MatchString(stripped):
		m := rePlayerNameLine.FindStringSubmatch(stripped)
		playerID, _ := strconv.Atoi(m[1])
		name := strings.TrimSpace(m[2])
		p.emit(GameEvent{
			Type:       EventPlayerName,
			Timestamp:  ts,
			PlayerID:   playerID,
			EntityName: name,
			Tags:       map[string]string{},
		})

	case reGameComplete.MatchString(stripped):
		p.emit(GameEvent{
			Type:      EventGameEnd,
			Timestamp: ts,
			Tags:      map[string]string{},
		})

	case reFullEntity.MatchString(stripped):
		m := reFullEntity.FindStringSubmatch(stripped)
		var id int
		var entityDesc string
		if m[1] != "" {
			id, _ = strconv.Atoi(m[1])
		}
		if m[2] != "" {
			entityDesc = strings.TrimSpace(m[2])
			if id == 0 {
				id = extractEntityID(entityDesc)
			}
		}
		cardID := m[3]
		// Enter block mode: accumulate subsequent indented tag lines.
		p.inBlock = true
		p.pending = GameEvent{
			Type:       EventEntityUpdate,
			Timestamp:  ts,
			EntityID:   id,
			EntityName: entityDesc,
			CardID:     cardID,
			Tags:       map[string]string{},
		}

	case reTurnStart.MatchString(stripped):
		m := reTurnStart.FindStringSubmatch(stripped)
		turn, _ := strconv.Atoi(m[1])
		p.emit(GameEvent{
			Type:      EventTurnStart,
			Timestamp: ts,
			Tags:      map[string]string{"TURN": strconv.Itoa(turn)},
		})

	case reTagChange.MatchString(stripped):
		m := reTagChange.FindStringSubmatch(stripped)
		entity := strings.TrimSpace(m[1])
		tag := m[2]
		value := m[3]
		id := extractEntityID(entity)
		playerID := extractPlayerField(entity)
		p.emit(GameEvent{
			Type:       EventTagChange,
			Timestamp:  ts,
			EntityID:   id,
			PlayerID:   playerID,
			EntityName: entity,
			Tags:       map[string]string{tag: value},
		})
	}
}

// Flush emits any pending buffered block event. Call after the last line to
// ensure a trailing FULL_ENTITY / SHOW_ENTITY block is not lost.
func (p *Parser) Flush() {
	p.flushPending()
}

func (p *Parser) flushPending() {
	if p.inBlock {
		// Resolve the CONTROLLER from block tags if present.
		if ctrl, ok := p.pending.Tags["CONTROLLER"]; ok && p.pending.PlayerID == 0 {
			p.pending.PlayerID, _ = strconv.Atoi(ctrl)
		}
		p.emit(p.pending)
		p.inBlock = false
		p.pending = GameEvent{}
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
	if m != nil {
		id, _ := strconv.Atoi(m[1])
		return id
	}
	// Handle bare numeric entity IDs (e.g., "10181" from "Entity=10181").
	id, err := strconv.Atoi(s)
	if err == nil && id > 0 {
		return id
	}
	return 0
}

func extractPlayerField(s string) int {
	m := rePlayerField.FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	id, _ := strconv.Atoi(m[1])
	return id
}
