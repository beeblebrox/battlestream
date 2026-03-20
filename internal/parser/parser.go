package parser

import (
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// blockContext tracks the source entity of a BLOCK_START for attribution.
type blockContext struct {
	entityID int
	cardID   string
}

// Parser parses raw Power.log lines into GameEvents.
// It maintains a small block state for FULL_ENTITY / SHOW_ENTITY entries whose
// ATK/HEALTH tags appear on subsequent indented lines.
type Parser struct {
	out        chan<- GameEvent
	inBlock    bool
	pending    GameEvent
	blockStack []blockContext
	refDate    time.Time // reference date for timestamps (default: time.Now())
	lastTS     time.Time // last emitted timestamp for midnight wrap detection

	// CREATE_GAME GameEntity block capture
	inCreateGameEntity    bool
	createGameEntityTags  map[string]string
}

// New creates a Parser that sends events to out.
func New(out chan<- GameEvent) *Parser {
	return &Parser{out: out}
}

// SetReferenceDate sets the date used for timestamp construction.
// Use the log file's modification time for reparse, or time.Now() for live tailing.
// Also handles midnight wrap: if a timestamp appears earlier than the previous one,
// the reference date is advanced by one day.
func (p *Parser) SetReferenceDate(t time.Time) {
	p.refDate = t.Truncate(24 * time.Hour)
}

// blockTagMinIndent is the minimum number of leading spaces for block tag
// continuation lines. Update if Blizzard changes the Power.log indentation.
const blockTagMinIndent = 4

// Power.log regex patterns
var (
	// Only process lines from GameState.DebugPrintPower / DebugPrintGame.
	// Skip PowerTaskList lines to avoid processing duplicate events.
	reGameStateSource = regexp.MustCompile(`GameState\.DebugPrint(?:Power|Game)\(\)`)

	// D 21:11:50.1234567 GameState.DebugPrintPower() - CREATE_GAME
	reCreateGame = regexp.MustCompile(`CREATE_GAME`)

	// GameEntity EntityID=13  (inside CREATE_GAME block)
	reGameEntity = regexp.MustCompile(`GameEntity\s+EntityID=(\d+)`)

	// Player EntityID=20 PlayerID=7 GameAccountId=[hi=144115193835963207 lo=30722021]
	rePlayerDef = regexp.MustCompile(`Player\s+EntityID=(\d+)\s+PlayerID=(\d+)\s+GameAccountId=\[hi=(\d+)\s+lo=(\d+)\]`)

	// GameState.DebugPrintGame() - PlayerID=7, PlayerName=Moch#1358
	rePlayerNameLine = regexp.MustCompile(`PlayerID=(\d+),\s+PlayerName=(.+)`)

	// TAG_CHANGE Entity=GameEntity tag=TURN value=7
	reTurnStart = regexp.MustCompile(`TAG_CHANGE\s+Entity=GameEntity\s+tag=TURN\s+value=(\d+)`)

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
	// Requires at least blockTagMinIndent spaces after the " - " separator.
	// If Blizzard changes the log indentation, update this constant.
	reBlockTag = regexp.MustCompile(`-\s{` + strconv.Itoa(blockTagMinIndent) + `,}tag=(\S+)\s+value=(\S*)`)

	// BLOCK_START BlockType=PLAY Entity=[entityName=... cardId=BG_LOE_077 player=8] EffectCardId=...
	reBlockStart = regexp.MustCompile(`BLOCK_START\s+BlockType=\w+\s+Entity=(.+?)\s+EffectCardId=`)

	// BLOCK_END
	reBlockEnd = regexp.MustCompile(`BLOCK_END`)

	// cardId= field from bracketed entity notation
	reCardIDField = regexp.MustCompile(`\bcardId=(\S+?)[\s\]]`)
)

// Feed processes a single raw log line.
func (p *Parser) Feed(line string) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("parser panic recovered", "error", fmt.Sprint(r), "line", line)
		}
	}()

	// Only process GameState.DebugPrintPower/Game lines.
	// Skip PowerTaskList to avoid duplicate events.
	if !reGameStateSource.MatchString(line) {
		return
	}

	ts := p.extractTimestamp(line)
	stripped := stripTimestamp(line)

	// Capture GameEntity block tags during CREATE_GAME.
	if p.inCreateGameEntity {
		if m := reBlockTag.FindStringSubmatch(stripped); m != nil {
			p.createGameEntityTags[m[1]] = m[2]
			return
		}
		// Non-tag line ends the GameEntity block — emit accumulated tags.
		if len(p.createGameEntityTags) > 0 {
			p.emit(GameEvent{
				Type: EventGameEntityTags,
				Timestamp: ts,
				Tags: p.createGameEntityTags,
			})
		}
		p.inCreateGameEntity = false
		p.createGameEntityTags = nil
	}

	// If we're inside a FULL_ENTITY / SHOW_ENTITY block, check for
	// continuation tag lines before processing any other patterns.
	if p.inBlock {
		if m := reBlockTag.FindStringSubmatch(stripped); m != nil {
			p.pending.Tags[m[1]] = m[2]
			return
		}
		// A new game trumps any pending block state — discard the stale
		// partial event rather than flushing it as garbage.
		if reCreateGame.MatchString(stripped) {
			p.inBlock = false
			p.pending = GameEvent{}
			p.blockStack = p.blockStack[:0]
		} else {
			// Non-continuation line: flush the accumulated entity event.
			p.flushPending()
		}
	}

	// Handle BLOCK_START/BLOCK_END for context tracking.
	if reBlockEnd.MatchString(stripped) {
		if len(p.blockStack) > 0 {
			p.blockStack = p.blockStack[:len(p.blockStack)-1]
		}
		return
	}
	if m := reBlockStart.FindStringSubmatch(stripped); m != nil {
		entity := strings.TrimSpace(m[1])
		bc := blockContext{
			entityID: extractEntityID(entity),
			cardID:   extractCardIDField(entity),
		}
		p.blockStack = append(p.blockStack, bc)
		return
	}

	switch {
	case reCreateGame.MatchString(stripped):
		// Reset any leftover block state from the previous game.
		p.inBlock = false
		p.pending = GameEvent{}
		p.blockStack = p.blockStack[:0]
		p.inCreateGameEntity = false
		p.createGameEntityTags = nil
		p.emit(GameEvent{
			Type:      EventGameStart,
			Timestamp: ts,
			Tags:      map[string]string{},
		})

	case reGameEntity.MatchString(stripped):
		// GameEntity block inside CREATE_GAME — capture subsequent tag lines.
		p.inCreateGameEntity = true
		p.createGameEntityTags = make(map[string]string)

	case rePlayerDef.MatchString(stripped):
		m := rePlayerDef.FindStringSubmatch(stripped)
		entityID, _ := strconv.Atoi(m[1])
		playerID, _ := strconv.Atoi(m[2])
		hi := m[3]
		lo := m[4]
		// Use block mode so that subsequent indented tag= lines (e.g. HERO_ENTITY)
		// are captured in p.pending.Tags before the event is emitted.
		p.pending = GameEvent{
			Type:     EventPlayerDef,
			Timestamp: ts,
			EntityID: entityID,
			PlayerID: playerID,
			Tags: map[string]string{
				"hi":        hi,
				"lo":        lo,
				"PLAYER_ID": m[2],
			},
		}
		p.inBlock = true

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
		p.applyBlockContext(&p.pending)

	// IMPORTANT: reTurnStart must be checked BEFORE reTagChange because both
	// match TAG_CHANGE lines. reTurnStart matches the specific pattern
	// "tag=TURN value=N" and emits EventTurnStart; reTagChange is the general
	// catch-all for all TAG_CHANGE lines. If reordered, turn changes would be
	// emitted as generic EventTagChange instead of EventTurnStart.
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
		ev := GameEvent{
			Type:       EventTagChange,
			Timestamp:  ts,
			EntityID:   id,
			PlayerID:   playerID,
			EntityName: entity,
			Tags:       map[string]string{tag: value},
		}
		p.applyBlockContext(&ev)
		p.emit(ev)
	}
}

// Flush emits any pending buffered block event. Call after the last line to
// ensure a trailing FULL_ENTITY / SHOW_ENTITY block is not lost.
func (p *Parser) Flush() {
	p.flushPending()
}

func (p *Parser) flushPending() {
	if p.inBlock {
		if len(p.pending.Tags) == 0 {
			slog.Warn("flushing entity block with empty Tags — indent regex may need update",
				"entity", p.pending.EntityName, "entityID", p.pending.EntityID)
		}
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
	if p.out == nil {
		return
	}
	select {
	case p.out <- e:
	default:
		// Channel full — drop event rather than blocking the log tail reader.
		// This should be extremely rare if the channel is adequately buffered.
		slog.Warn("parser event channel full, dropping event", "type", e.Type)
	}
}

func (p *Parser) extractTimestamp(line string) time.Time {
	m := reTimestamp.FindStringSubmatch(line)
	if m == nil {
		return time.Now()
	}
	t, err := time.Parse("15:04:05.9999999", m[1])
	if err != nil {
		return time.Now()
	}
	ref := p.refDate
	if ref.IsZero() {
		ref = time.Now().Truncate(24 * time.Hour)
		p.refDate = ref
	}
	ts := time.Date(ref.Year(), ref.Month(), ref.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), ref.Location())
	// Midnight wrap detection: if this timestamp is before the last one by more
	// than 12 hours, assume we crossed midnight and advance the reference date.
	if !p.lastTS.IsZero() && ts.Before(p.lastTS) && p.lastTS.Sub(ts) > 12*time.Hour {
		p.refDate = p.refDate.Add(24 * time.Hour)
		ts = ts.Add(24 * time.Hour)
	}
	p.lastTS = ts
	return ts
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

func extractCardIDField(s string) string {
	m := reCardIDField.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// applyBlockContext sets the block source fields from the current top of stack.
func (p *Parser) applyBlockContext(e *GameEvent) {
	if len(p.blockStack) > 0 {
		top := p.blockStack[len(p.blockStack)-1]
		e.BlockSource = top.entityID
		e.BlockCardID = top.cardID
	}
}
