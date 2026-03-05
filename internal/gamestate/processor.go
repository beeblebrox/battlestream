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
	CardID   string
	Name     string
	CardType string
	Attack   int
	Health   int
	Zone     string
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
}

// NewProcessor returns a Processor that updates the given Machine.
func NewProcessor(m *Machine) *Processor {
	return &Processor{
		machine:          m,
		entityController: make(map[int]int),
		heroEntities:     make(map[int]bool),
		entityProps:      make(map[int]*entityInfo),
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
		gameID := fmt.Sprintf("game-%d", p.gameSeq)
		p.machine.GameStart(gameID, e.Timestamp)

	case parser.EventPlayerDef:
		p.handlePlayerDef(e)

	case parser.EventPlayerName:
		p.handlePlayerName(e)

	case parser.EventGameEnd:
		placement := p.pendingPlacement
		if pl, ok := e.Tags["PLAYER_LEADERBOARD_PLACE"]; ok {
			if v, err := strconv.Atoi(pl); err == nil {
				placement = v
			}
		}
		p.pendingPlacement = 0
		p.machine.GameEnd(placement, e.Timestamp)

	case parser.EventTurnStart:
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

	for tag, value := range e.Tags {
		switch tag {
		case "HEALTH", "ARMOR":
			// Only apply to the local player's hero entity.
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
				// Update stored zone.
				if info := p.entityProps[e.EntityID]; info != nil {
					info.Zone = value
				}
			}
			if value == "GRAVEYARD" || value == "REMOVEDFROMGAME" {
				if e.EntityID > 0 && p.machine.Phase() != PhaseGameOver {
					p.machine.RemoveMinion(e.EntityID)
				}
			} else if value == "PLAY" && e.EntityID > 0 {
				// Minion moved to board — add if it's a local minion.
				p.tryAddMinionFromRegistry(e.EntityID, controllerID)
			}

		case "TURN":
			// Player-specific TURN tag (not GameEntity).
			// This gives us the actual BG turn number the player sees.
			if p.isLocalPlayerEntity(e) {
				turn, _ := strconv.Atoi(value)
				if turn > 0 {
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

	// Register hero entity IDs.
	if cardType == "HERO" && e.EntityID > 0 {
		p.heroEntities[e.EntityID] = true
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
		p.machine.UpsertMinion(MinionState{
			EntityID: e.EntityID,
			CardID:   info.CardID,
			Name:     info.Name,
			Attack:   info.Attack,
			Health:   info.Health,
		})
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
	p.machine.UpsertMinion(MinionState{
		EntityID: entityID,
		CardID:   info.CardID,
		Name:     info.Name,
		Attack:   info.Attack,
		Health:   info.Health,
	})
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
