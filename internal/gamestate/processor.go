package gamestate

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"battlestream.fixates.io/internal/parser"
)

// entityInfo stores known properties of an entity for board tracking.
type entityInfo struct {
	CardID      string
	Name        string
	CardType    string
	Attack      int
	Health      int
	Zone        string
	CreatorID   int
	AttachedTo  int
	ScriptData1 int
	ScriptData2 int
}

// pendingStatChange buffers a stat change for batch analysis.
type pendingStatChange struct {
	entityID int
	name     string
	turn     int
	stat     string
	delta    int
}

// Processor consumes parser.GameEvents and updates a Machine.
type Processor struct {
	machine          *Machine
	gameSeq          int
	pendingPlacement int // tracks PLAYER_LEADERBOARD_PLACE before GAME_RESULT fires

	// Local player identity — determined from CREATE_GAME block.
	localPlayerID   int    // CONTROLLER value for the local player (e.g. 7)
	localPlayerName string // BattleTag (e.g. "Moch#1358")
	localHeroID     int    // entity ID of the local player's hero card

	// Entity registry — maps entity IDs to their controller PlayerIDs.
	entityController map[int]int
	// heroEntities tracks entity IDs known to be HERO card types.
	heroEntities     map[int]bool
	// entityProps tracks known properties of entities for zone transition handling.
	entityProps      map[int]*entityInfo

	// Buffered stat changes for board-wide buff detection.
	pendingStatChanges []pendingStatChange

	// Buff source state: category → [ATK, HP] for player-level tag tracking.
	buffSourceState map[string][2]int

	// Counter state for Dnt enchantments (HDT-style counters).
	// shopBuffPrev tracks previous SD values per entity for differential accumulation.
	shopBuffPrev map[int][2]int
	// nomiCounter tracks the running Nomi total [ATK, HP].
	nomiCounter [2]int
	// nomiAllCounter tracks the running Nomi (All) total [ATK, HP] for Timewarped Nomi.
	nomiAllCounter [2]int

	// Economy counters.
	goldNextTurnSure   int // from BACON_PLAYER_EXTRA_GOLD_NEXT_TURN player tag
	overconfidenceCount int // number of active Overconfidence Dnt enchantments in PLAY
}

// NewProcessor returns a Processor that updates the given Machine.
func NewProcessor(m *Machine) *Processor {
	return &Processor{
		machine:          m,
		entityController: make(map[int]int),
		heroEntities:     make(map[int]bool),
		entityProps:      make(map[int]*entityInfo),
		buffSourceState:  make(map[string][2]int),
		shopBuffPrev:     make(map[int][2]int),
	}
}

// Handle processes a single GameEvent and updates the game state.
func (p *Processor) Handle(e parser.GameEvent) {
	switch e.Type {
	case parser.EventGameStart:
		p.gameSeq++
		p.pendingPlacement = 0
		p.localPlayerID = 0
		p.localPlayerName = ""
		p.localHeroID = 0
		p.entityController = make(map[int]int)
		p.heroEntities = make(map[int]bool)
		p.entityProps = make(map[int]*entityInfo)
		p.buffSourceState = make(map[string][2]int)
		p.shopBuffPrev = make(map[int][2]int)
		p.nomiCounter = [2]int{}
		p.nomiAllCounter = [2]int{}
		p.goldNextTurnSure = 0
		p.overconfidenceCount = 0
		gameID := fmt.Sprintf("game-%d", p.gameSeq)
		p.machine.GameStart(gameID, e.Timestamp)

	case parser.EventPlayerDef:
		p.handlePlayerDef(e)

	case parser.EventPlayerName:
		p.handlePlayerName(e)

	case parser.EventGameEnd:
		p.flushPendingStatChanges()
		placement := p.pendingPlacement
		if pl, ok := e.Tags["PLAYER_LEADERBOARD_PLACE"]; ok {
			if v, err := strconv.Atoi(pl); err == nil {
				placement = v
			}
		}
		p.pendingPlacement = 0
		p.machine.GameEnd(placement, e.Timestamp)

	case parser.EventTurnStart:
		p.flushPendingStatChanges()
		if t, ok := e.Tags["TURN"]; ok {
			turn, _ := strconv.Atoi(t)
			p.machine.SetGameEntityTurn(turn)
		}

	case parser.EventTagChange:
		p.handleTagChange(e)

	case parser.EventEntityUpdate:
		p.handleEntityUpdate(e)
	}
}

// handlePlayerDef identifies the local player from the CREATE_GAME block.
// The local player has a real GameAccountId (hi≠0); the dummy/opponent has hi=0.
func (p *Processor) handlePlayerDef(e parser.GameEvent) {
	hi := e.Tags["hi"]
	if hi != "" && hi != "0" {
		// This is the local player.
		p.localPlayerID = e.PlayerID
		slog.Info("identified local player", "playerID", p.localPlayerID, "entityID", e.EntityID)
	}
}

// handlePlayerName maps PlayerID to a display name.
func (p *Processor) handlePlayerName(e parser.GameEvent) {
	if e.PlayerID == p.localPlayerID {
		p.localPlayerName = e.EntityName
		p.machine.UpdatePlayerName(e.EntityName)
		slog.Info("local player name", "name", e.EntityName)
	}
}

func (p *Processor) handleTagChange(e parser.GameEvent) {
	// Determine the controller for this entity.
	controllerID := p.resolveController(e)

	// Keep entity registry up-to-date with names from TAG_CHANGE events.
	if e.EntityID > 0 && e.EntityName != "" {
		info := p.entityProps[e.EntityID]
		if info == nil {
			info = &entityInfo{}
			p.entityProps[e.EntityID] = info
		}
		cleaned := cleanEntityName(e.EntityName)
		// Update name if empty or currently a bare number placeholder.
		if info.Name == "" || isBareNumber(info.Name) {
			info.Name = cleaned
		}
		if info.CardID == "" {
			info.CardID = extractCardID(e.EntityName)
		}
	}

	for tag, value := range e.Tags {
		switch tag {
		case "HEALTH":
			if p.isLocalHero(e, controllerID) {
				p.machine.UpdatePlayerTag(tag, value)
			} else if e.EntityID > 0 && controllerID == p.localPlayerID {
				p.updateMinionStat(e, "HEALTH", value)
			}

		case "ATK":
			if e.EntityID > 0 && controllerID == p.localPlayerID {
				p.updateMinionStat(e, "ATK", value)
			}

		case "ARMOR":
			if p.isLocalHero(e, controllerID) {
				p.machine.UpdatePlayerTag(tag, value)
			}

		case "SPELL_POWER":
			if controllerID == p.localPlayerID || controllerID == 0 {
				p.machine.UpdatePlayerTag(tag, value)
			}

		case "PLAYER_TECH_LEVEL", "TAVERN_TIER":
			// Only apply if this is the local player's entity.
			if controllerID == p.localPlayerID || p.isLocalPlayerEntity(e) {
				tier, _ := strconv.Atoi(value)
				if tier > 0 {
					p.machine.SetTavernTier(tier)
				}
			}

		case "PLAYER_TRIPLES":
			// PLAYER_TRIPLES is set on the hero entity with the cumulative count.
			if p.isLocalHero(e, controllerID) || p.isLocalPlayerEntity(e) {
				p.machine.UpdatePlayerTag(tag, value)
			}

		case "PLAYER_LEADERBOARD_PLACE":
			// Only track placement for the local player.
			if p.isLocalPlayerEntity(e) {
				if pl, err := strconv.Atoi(value); err == nil && pl > 0 {
					p.pendingPlacement = pl
				}
			}

		case "ZONE":
			if e.EntityID > 0 {
				// Update stored zone and track Overconfidence Dnt zone transitions.
				info := p.entityProps[e.EntityID]
				if info != nil {
					prevZone := info.Zone
					info.Zone = value
					p.handleOverconfidenceZone(info.CardID, value, prevZone)
				}
			}
			if value == "GRAVEYARD" || value == "REMOVEDFROMGAME" || value == "SETASIDE" {
				if e.EntityID > 0 && p.machine.Phase() != PhaseGameOver {
					p.machine.RemoveMinion(e.EntityID)
					p.machine.RemoveEnchantmentsForEntity(e.EntityID)
				}
			} else if value == "PLAY" && e.EntityID > 0 {
				// Minion moved to board — add if it's a local minion.
				// Allow zone transitions during any phase (BG refreshes board
				// entities from SETASIDE→PLAY between combat rounds).
				if p.machine.Phase() != PhaseGameOver {
					p.tryAddMinionFromRegistry(e.EntityID, controllerID)
				}
			}

		case "TURN":
			// Player-specific TURN tag (not GameEntity).
			// This gives us the actual BG turn number the player sees.
			if p.isLocalPlayerEntity(e) {
				turn, _ := strconv.Atoi(value)
				if turn > 0 {
					p.flushPendingStatChanges()
					p.machine.SetTurn(turn)
				}
			}

		case "HERO_ENTITY":
			// Track which hero entity belongs to the local player.
			if p.isLocalPlayerEntity(e) {
				heroID, _ := strconv.Atoi(value)
				if heroID > 0 {
					p.localHeroID = heroID
					slog.Info("local hero entity updated", "heroID", heroID)
				}
			}

		case "BACON_BLOODGEMBUFFATKVALUE", "BACON_BLOODGEMBUFFHEALTHVALUE",
			"BACON_ELEMENTAL_BUFFATKVALUE", "BACON_ELEMENTAL_BUFFHEALTHVALUE",
			"TAVERN_SPELL_ATTACK_INCREASE", "TAVERN_SPELL_HEALTH_INCREASE":
			if p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID) {
				p.updateBuffSourceFromPlayerTag(tag, value)
			}

		case "BACON_FREE_REFRESH_COUNT":
			if p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID) {
				raw, _ := strconv.Atoi(value)
				if raw > 0 {
					p.machine.SetAbilityCounter(CatFreeRefresh, raw, fmt.Sprintf("%d", raw))
				}
			}

		case "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN":
			if p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID) {
				raw, _ := strconv.Atoi(value)
				if raw < 0 {
					raw = 0
				}
				p.goldNextTurnSure = raw
				p.updateGoldNextTurnCounter()
			}

		case "TAG_SCRIPT_DATA_NUM_1", "TAG_SCRIPT_DATA_NUM_2":
			if e.EntityID > 0 {
				p.updateEnchantmentScriptData(e.EntityID, tag, value)
				p.handleDntTagChange(e.EntityID, tag, parseInt(value))
			}

		case "3809":
			// Spellcraft (Naga spell) stacks counter on local player.
			if p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID) {
				raw, _ := strconv.Atoi(value)
				stacks := 1 + (raw / 4)
				progress := raw % 4
				display := fmt.Sprintf("%d (%d/4)", stacks, progress)
				p.machine.SetAbilityCounter(CatSpellcraft, raw, display)
			}

		case "CONTROLLER":
			// Update entity controller registry.
			if e.EntityID > 0 {
				ctrl, _ := strconv.Atoi(value)
				p.entityController[e.EntityID] = ctrl
			}
		}
	}
}

func (p *Processor) handleEntityUpdate(e parser.GameEvent) {
	if e.EntityID == 0 {
		return
	}

	// Register the controller from the CONTROLLER tag in the block.
	controllerID := e.PlayerID
	if ctrl, ok := e.Tags["CONTROLLER"]; ok {
		c, _ := strconv.Atoi(ctrl)
		if c > 0 {
			controllerID = c
		}
	}
	if controllerID > 0 {
		p.entityController[e.EntityID] = controllerID
	}

	// Determine card type from tags.
	cardType := e.Tags["CARDTYPE"]

	// Store/update entity properties for zone transition handling.
	info := p.entityProps[e.EntityID]
	if info == nil {
		info = &entityInfo{}
		p.entityProps[e.EntityID] = info
	}
	if e.CardID != "" {
		info.CardID = e.CardID
	}
	if e.EntityName != "" {
		info.Name = cleanEntityName(e.EntityName)
	}
	if cardType != "" {
		info.CardType = cardType
	}
	if atk, ok := e.Tags["ATK"]; ok {
		info.Attack = parseInt(atk)
	}
	if hp, ok := e.Tags["HEALTH"]; ok {
		info.Health = parseInt(hp)
	}
	if zone, ok := e.Tags["ZONE"]; ok {
		info.Zone = zone
	}
	if creator, ok := e.Tags["CREATOR"]; ok {
		info.CreatorID = parseInt(creator)
	}
	if attached, ok := e.Tags["ATTACHED"]; ok {
		info.AttachedTo = parseInt(attached)
	}
	if sd1, ok := e.Tags["TAG_SCRIPT_DATA_NUM_1"]; ok {
		info.ScriptData1 = parseInt(sd1)
	}
	if sd2, ok := e.Tags["TAG_SCRIPT_DATA_NUM_2"]; ok {
		info.ScriptData2 = parseInt(sd2)
	}

	// Register hero entity IDs.
	if cardType == "HERO" && e.EntityID > 0 {
		p.heroEntities[e.EntityID] = true
	}

	// Handle enchantment entities — track buff sources.
	if cardType == "ENCHANTMENT" {
		p.handleEnchantmentEntity(e, info)
		return
	}

	// If this is a HERO entity owned by the local player, track its stats.
	if cardType == "HERO" && controllerID == p.localPlayerID {
		if hp, ok := e.Tags["HEALTH"]; ok {
			p.machine.UpdatePlayerTag("HEALTH", hp)
		}
		if armor, ok := e.Tags["ARMOR"]; ok {
			p.machine.UpdatePlayerTag("ARMOR", armor)
		}
		if e.CardID != "" && !strings.HasPrefix(e.CardID, "TB_BaconShop_HERO_PH") {
			p.machine.UpdateHeroCardID(e.CardID)
		}
		return
	}

	// For minions: require ATK or HEALTH, and filter by controller.
	if info.Attack == 0 && info.Health == 0 {
		return
	}

	// Skip non-minion entities (heroes, enchantments, etc.)
	if info.CardType != "" && info.CardType != "MINION" {
		return
	}

	// Only add minions in PLAY zone to the board.
	if info.Zone != "" && info.Zone != "PLAY" {
		return
	}

	if controllerID == p.localPlayerID {
		if p.machine.Phase() != PhaseGameOver {
			p.machine.UpsertMinion(MinionState{
				EntityID: e.EntityID,
				CardID:   info.CardID,
				Name:     info.Name,
				Attack:   info.Attack,
				Health:   info.Health,
			})
		}
	}
}

// resolveController returns the controller PlayerID for the entity in a
// GameEvent. It checks (in order): the event's PlayerID field (from player=N),
// the entity controller registry, and falls back to 0 (unknown).
func (p *Processor) resolveController(e parser.GameEvent) int {
	if e.PlayerID > 0 {
		return e.PlayerID
	}
	if e.EntityID > 0 {
		if ctrl, ok := p.entityController[e.EntityID]; ok {
			return ctrl
		}
	}
	return 0
}

// isLocalPlayerEntity checks whether the event's entity is the local player
// entity itself (not a hero/minion, but the Player entity).
func (p *Processor) isLocalPlayerEntity(e parser.GameEvent) bool {
	if p.localPlayerName != "" && e.EntityName == p.localPlayerName {
		return true
	}
	if p.localPlayerID > 0 && e.PlayerID == p.localPlayerID {
		return true
	}
	return false
}

// isLocalHero checks whether the entity in the event is the local player's
// hero card. Only matches entities known to be heroes (CARDTYPE=HERO) that
// are controlled by the local player.
func (p *Processor) isLocalHero(e parser.GameEvent, controllerID int) bool {
	if e.EntityID <= 0 || controllerID != p.localPlayerID {
		return false
	}
	// Best check: is this the currently assigned hero entity?
	if p.localHeroID > 0 && e.EntityID == p.localHeroID {
		return true
	}
	// Fallback: is this entity registered as a HERO type?
	return p.heroEntities[e.EntityID]
}

// updateMinionStat updates a minion's stat on the board and in the entity
// registry. During recruit phase, buffers the delta for board-wide detection.
func (p *Processor) updateMinionStat(e parser.GameEvent, stat, value string) {
	newVal := parseInt(value)
	if e.EntityID <= 0 {
		return
	}

	// Update entity registry.
	info := p.entityProps[e.EntityID]
	if info == nil {
		info = &entityInfo{}
		p.entityProps[e.EntityID] = info
	}

	// Extract name from the event entity name if we don't have one yet
	// or only have a bare numeric placeholder.
	if e.EntityName != "" && (info.Name == "" || isBareNumber(info.Name)) {
		info.Name = cleanEntityName(e.EntityName)
	}

	var oldVal int
	switch stat {
	case "ATK":
		oldVal = info.Attack
		info.Attack = newVal
	case "HEALTH":
		oldVal = info.Health
		info.Health = newVal
	}

	// Skip hero entities — those are handled by UpdatePlayerTag.
	if p.heroEntities[e.EntityID] {
		return
	}

	// Update board minion stats if it's on the board.
	onBoard := p.machine.UpdateMinionStat(e.EntityID, stat, newVal)

	// Buffer stat changes during recruit phase for board-wide buff detection.
	// Skip during combat (simulation noise).
	phase := p.machine.Phase()
	if oldVal > 0 && newVal != oldVal && (onBoard || info.Zone == "PLAY") && phase != PhaseCombat {
		delta := newVal - oldVal
		name := info.Name
		if name == "" {
			name = info.CardID
		}
		p.pendingStatChanges = append(p.pendingStatChanges, pendingStatChange{
			entityID: e.EntityID,
			name:     name,
			turn:     p.machine.currentTurn(),
			stat:     stat,
			delta:    delta,
		})
	}
}

// flushPendingStatChanges groups buffered stat changes and emits only board-wide
// modifications (2+ minions affected with the same turn/stat/delta).
func (p *Processor) flushPendingStatChanges() {
	if len(p.pendingStatChanges) == 0 {
		return
	}

	type groupKey struct {
		turn  int
		stat  string
		delta int
	}

	groups := make(map[groupKey]int)
	for _, sc := range p.pendingStatChanges {
		groups[groupKey{sc.turn, sc.stat, sc.delta}]++
	}

	for key, count := range groups {
		if count >= 2 {
			p.machine.AddMod(StatMod{
				Turn:   key.turn,
				Target: fmt.Sprintf("Board (%dx)", count),
				Stat:   key.stat,
				Delta:  key.delta,
			})
		}
	}

	p.pendingStatChanges = p.pendingStatChanges[:0]
}

// tryAddMinionFromRegistry adds a minion to the board from the entity registry
// if it's a local player's minion with valid stats.
func (p *Processor) tryAddMinionFromRegistry(entityID, controllerID int) {
	if controllerID != p.localPlayerID {
		return
	}
	info := p.entityProps[entityID]
	if info == nil || (info.Attack == 0 && info.Health == 0) {
		return
	}
	if info.CardType != "" && info.CardType != "MINION" {
		return
	}
	if p.machine.Phase() == PhaseGameOver {
		return
	}
	p.machine.UpsertMinion(MinionState{
		EntityID: entityID,
		CardID:   info.CardID,
		Name:     info.Name,
		Attack:   info.Attack,
		Health:   info.Health,
	})
	// During combat, keep the snapshot in sync so GameEnd restores
	// the combat board with correct buffed stats.
	if p.machine.Phase() == PhaseCombat {
		p.machine.UpdateBoardSnapshot()
	}
}

// isBareNumber returns true if s consists entirely of digits.
func isBareNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// cleanEntityName extracts a readable name from bracketed entity notation.
func cleanEntityName(s string) string {
	if strings.HasPrefix(s, "[entityName=") {
		end := strings.Index(s, " id=")
		if end > 12 {
			return s[12:end]
		}
	}
	return s
}

// handleEnchantmentEntity processes a FULL_ENTITY/SHOW_ENTITY with CARDTYPE=ENCHANTMENT.
// Tracks per-minion enchantments in Enchantments[]; Dnt buff sources are handled
// separately by handleDntTagChange via TAG_CHANGE events.
func (p *Processor) handleEnchantmentEntity(e parser.GameEvent, info *entityInfo) {
	if info.AttachedTo == 0 {
		return // no target — skip
	}

	// Resolve the CREATOR entity to get source card info.
	var sourceCardID, sourceName string
	if info.CreatorID > 0 {
		if creator := p.entityProps[info.CreatorID]; creator != nil {
			sourceCardID = creator.CardID
			sourceName = creator.Name
		}
	}

	// Classify the enchantment.
	category := ClassifyEnchantment(info.CardID)
	if category == CatGeneral && sourceCardID != "" {
		if cat, ok := ClassifyCreator(sourceCardID); ok {
			category = cat
		}
	}

	// Determine ATK/HP buff values from script data.
	atkBuff := info.ScriptData1
	hpBuff := info.ScriptData2
	if IsNomiSticker(info.CardID) {
		hpBuff = info.ScriptData1 // Nomi Sticker uses NUM_1 for both
	}

	// Check if the target is a local board minion.
	targetCtrl := p.entityController[info.AttachedTo]
	enchCtrl := p.entityController[e.EntityID]
	isOnBoardMinion := targetCtrl == p.localPlayerID

	if !isOnBoardMinion {
		if enchCtrl != p.localPlayerID {
			return
		}
		if category == CatGeneral {
			return
		}
	}

	ench := Enchantment{
		EntityID:     e.EntityID,
		CardID:       info.CardID,
		SourceCardID: sourceCardID,
		SourceName:   sourceName,
		TargetID:     info.AttachedTo,
		AttackBuff:   atkBuff,
		HealthBuff:   hpBuff,
		Category:     category,
	}
	p.machine.AddEnchantment(ench)

	// Process initial SD values from FULL_ENTITY/SHOW_ENTITY as counter updates.
	if info.ScriptData1 != 0 || info.ScriptData2 != 0 {
		if info.ScriptData1 != 0 {
			p.handleDntTagChange(e.EntityID, "TAG_SCRIPT_DATA_NUM_1", info.ScriptData1)
		}
		if info.ScriptData2 != 0 {
			p.handleDntTagChange(e.EntityID, "TAG_SCRIPT_DATA_NUM_2", info.ScriptData2)
		}
	}
}

// handleDntTagChange dispatches TAG_SCRIPT_DATA changes on Dnt enchantment
// entities to the appropriate HDT-style counter handler.
func (p *Processor) handleDntTagChange(entityID int, tag string, value int) {
	info := p.entityProps[entityID]
	if info == nil {
		return
	}
	// Only process enchantments controlled by local player.
	ctrl := p.entityController[entityID]
	if ctrl != p.localPlayerID {
		return
	}

	cardID := info.CardID
	isSD1 := tag == "TAG_SCRIPT_DATA_NUM_1"

	switch cardID {
	case "BG_ShopBuff_Elemental":
		// Nomi: DIFFERENTIAL accumulation (HDT pattern).
		p.handleShopBuffDnt(entityID, isSD1, value)
	case "BG30_MagicItem_544pe":
		// Nomi Sticker: SD1 applies to BOTH atk and hp (differential).
		p.handleNomiStickerDnt(entityID, isSD1, value)
	case "BG34_855pe":
		// Timewarped Nomi (Kitchen Dream): DIFFERENTIAL, buffs ALL minions.
		p.handleNomiAllDnt(entityID, isSD1, value)
	case "BG31_808pe":
		// Beetle: ABSOLUTE, base stats 1/1.
		p.handleAbsoluteDnt(CatBeetle, isSD1, value, 1, 1)
	case "BG34_854pe":
		// Rightmost: ABSOLUTE.
		p.handleAbsoluteDnt(CatRightmost, isSD1, value, 0, 0)
	case "BG34_402pe":
		// Whelp: ABSOLUTE.
		p.handleAbsoluteDnt(CatWhelp, isSD1, value, 0, 0)
	case "BG25_011pe":
		// Undead: SD1 only (ATK only), ABSOLUTE.
		if isSD1 {
			p.machine.SetBuffSource(CatUndead, value, 0)
		}
	case "BG34_170e":
		// Volumizer: ABSOLUTE.
		p.handleAbsoluteDnt(CatVolumizer, isSD1, value, 0, 0)
	case "BG34_689e2":
		// BloodGem Barrage: ABSOLUTE.
		p.handleAbsoluteDnt(CatBloodgemBarrage, isSD1, value, 0, 0)
	}
}

// handleAbsoluteDnt sets a buff source from an absolute Dnt value plus base offset.
func (p *Processor) handleAbsoluteDnt(category string, isSD1 bool, value, baseAtk, baseHp int) {
	state := p.buffSourceState[category]
	if isSD1 {
		state[0] = baseAtk + value
	} else {
		state[1] = baseHp + value
	}
	p.buffSourceState[category] = state
	p.machine.SetBuffSource(category, state[0], state[1])
}

// handleShopBuffDnt handles BG_ShopBuff_Elemental with differential accumulation.
// The Dnt tracks cumulative totals; we compute delta = value - prevValue.
func (p *Processor) handleShopBuffDnt(entityID int, isSD1 bool, value int) {
	prev := p.shopBuffPrev[entityID]
	var delta int
	if isSD1 {
		delta = value - prev[0]
		prev[0] = value
	} else {
		delta = value - prev[1]
		prev[1] = value
	}
	p.shopBuffPrev[entityID] = prev

	if isSD1 {
		p.nomiCounter[0] += delta
	} else {
		p.nomiCounter[1] += delta
	}
	p.machine.SetBuffSource(CatNomi, p.nomiCounter[0], p.nomiCounter[1])
}

// handleNomiAllDnt handles BG34_855pe (Timewarped Nomi / Kitchen Dream) with differential
// accumulation. Same pattern as regular Nomi but tracked under CatNomiAll.
func (p *Processor) handleNomiAllDnt(entityID int, isSD1 bool, value int) {
	prev := p.shopBuffPrev[entityID]
	var delta int
	if isSD1 {
		delta = value - prev[0]
		prev[0] = value
	} else {
		delta = value - prev[1]
		prev[1] = value
	}
	p.shopBuffPrev[entityID] = prev

	if isSD1 {
		p.nomiAllCounter[0] += delta
	} else {
		p.nomiAllCounter[1] += delta
	}
	p.machine.SetBuffSource(CatNomiAll, p.nomiAllCounter[0], p.nomiAllCounter[1])
}

// handleNomiStickerDnt handles BG30_MagicItem_544pe where SD1 applies to BOTH atk and hp.
func (p *Processor) handleNomiStickerDnt(entityID int, isSD1 bool, value int) {
	prev := p.shopBuffPrev[entityID]
	if isSD1 {
		delta := value - prev[0]
		prev[0] = value
		p.shopBuffPrev[entityID] = prev
		// SD1 applies to both ATK and HP.
		p.nomiCounter[0] += delta
		p.nomiCounter[1] += delta
	} else {
		// SD2 not used for Nomi Sticker.
		prev[1] = value
		p.shopBuffPrev[entityID] = prev
	}
	p.machine.SetBuffSource(CatNomi, p.nomiCounter[0], p.nomiCounter[1])
}

// updateBuffSourceFromPlayerTag handles player-level buff tags like
// BACON_BLOODGEMBUFFATKVALUE, TAVERN_SPELL_ATTACK_INCREASE, etc.
func (p *Processor) updateBuffSourceFromPlayerTag(tag, value string) {
	category, isATK, ok := ClassifyPlayerTag(tag)
	if !ok {
		return
	}

	rawVal := parseInt(value)

	// Apply category-specific value computation.
	var computedVal int
	switch category {
	case CatBloodgem:
		computedVal = ComputeBloodgemValue(rawVal)
	case CatElemental:
		computedVal = ComputeElementalValue(rawVal)
	default:
		computedVal = rawVal
	}

	state := p.buffSourceState[category]
	if isATK {
		state[0] = computedVal
	} else {
		state[1] = computedVal
	}
	p.buffSourceState[category] = state

	p.machine.SetBuffSource(category, state[0], state[1])
}

// updateEnchantmentScriptData handles TAG_CHANGE for TAG_SCRIPT_DATA_NUM_1/2
// on existing enchantment entities. Updates stored values only; counter-based
// buff source tracking is handled by handleDntTagChange.
func (p *Processor) updateEnchantmentScriptData(entityID int, tag, value string) {
	info := p.entityProps[entityID]
	if info == nil || info.CardType != "ENCHANTMENT" {
		return
	}

	val := parseInt(value)
	switch tag {
	case "TAG_SCRIPT_DATA_NUM_1":
		info.ScriptData1 = val
	case "TAG_SCRIPT_DATA_NUM_2":
		info.ScriptData2 = val
	}
}

// overconfidenceCardID is the Dnt enchantment for Overconfidence (BG28_884e).
const overconfidenceCardID = "BG28_884e"

// handleOverconfidenceZone tracks Overconfidence Dnt enchantments entering/leaving PLAY.
// Each active Overconfidence contributes +3 potential gold next turn.
func (p *Processor) handleOverconfidenceZone(cardID, newZone, prevZone string) {
	if cardID != overconfidenceCardID {
		return
	}
	if newZone == "PLAY" && prevZone != "PLAY" {
		p.overconfidenceCount++
		p.updateGoldNextTurnCounter()
	} else if newZone != "PLAY" && prevZone == "PLAY" {
		p.overconfidenceCount--
		if p.overconfidenceCount < 0 {
			p.overconfidenceCount = 0
		}
		p.updateGoldNextTurnCounter()
	}
}

// updateGoldNextTurnCounter updates the GoldNextTurn ability counter display.
func (p *Processor) updateGoldNextTurnCounter() {
	sure := p.goldNextTurnSure
	bonus := p.overconfidenceCount * 3
	if sure == 0 && p.overconfidenceCount == 0 {
		return
	}
	var display string
	if bonus > 0 {
		display = fmt.Sprintf("%d (%d)", sure, sure+bonus)
	} else {
		display = fmt.Sprintf("%d", sure)
	}
	p.machine.SetAbilityCounter(CatGoldNextTurn, sure, display)
}

// extractCardID extracts the cardId from bracketed entity notation.
// e.g. "[entityName=Acid Rainfall id=9596 zone=PLAY zonePos=5 cardId=BG34_857_G player=8]"
func extractCardID(s string) string {
	const prefix = "cardId="
	idx := strings.Index(s, prefix)
	if idx < 0 {
		return ""
	}
	rest := s[idx+len(prefix):]
	end := strings.IndexAny(rest, " ]")
	if end < 0 {
		return rest
	}
	return rest[:end]
}
