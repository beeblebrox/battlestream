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
	Armor       int // cached for retroactive hero identification
	Zone        string
	CreatorID   int
	AttachedTo  int
	ScriptData1 int
	ScriptData2 int
	Subsets     int // count of BACON_SUBSET_* tags seen (for multi-tribe detection)
}

// maxPendingStatChanges caps the pending stat-change buffer to prevent unbounded
// growth when a turn-boundary event is missed. Flush is triggered early with a warning.
const maxPendingStatChanges = 200

// pendingStatChange buffers a stat change for batch analysis.
type pendingStatChange struct {
	entityID int
	name     string
	turn     int
	stat     string
	delta    int
}

// buffTracker holds buff source tracking state for the local player.
// Encapsulates buff source state, Dnt counters, and economy counters.
type buffTracker struct {
	buffSourceState map[string][2]int
	shopBuffPrev    map[int][2]int
	nomiCounter     [2]int
	nomiAllCounter  [2]int
	goldNextTurnSure    int
	overconfidenceCount int
}

func newBuffTracker() buffTracker {
	return buffTracker{
		buffSourceState: make(map[string][2]int),
		shopBuffPrev:    make(map[int][2]int),
	}
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

	// Duos partner identity.
	partnerPlayerID   int    // CONTROLLER value for the partner (from BACON_DUO_TEAMMATE_PLAYER_ID)
	partnerPlayerName string
	partnerHeroID     int    // entity ID of the partner's hero card
	isDuos            bool

	// Entity registry — maps entity IDs to their controller PlayerIDs.
	entityController map[int]int
	// heroEntities tracks entity IDs known to be HERO card types.
	heroEntities     map[int]bool
	// entityProps tracks known properties of entities for zone transition handling.
	entityProps      map[int]*entityInfo

	// Buffered stat changes for board-wide buff detection.
	pendingStatChanges []pendingStatChange

	// Buff tracking for local player.
	localBuffs buffTracker

	// Win/loss streak tracking.
	// In BG, the winning side's hero attacks the losing hero at end of combat.
	// We track both attacker and defender to filter for the local hero's combat
	// (critical in Duos where partner's combat also fires hero attacks).
	// localCombatResult: 1=win, -1=loss, 0=unknown/tie.
	localCombatResult int
	// pendingHeroAttackerID/DefenderID capture the current PROPOSED_ATTACKER/DEFENDER
	// pair. When both are hero entities, we check if the local hero is involved.
	pendingHeroAttackerID int
	bgTurnsStarted        int // counts how many BG turns have started (skip recording for turn 1)

	// Available tribes detected from BACON_SUBSET_* tags.
	seenTribes        map[string]bool
	entityTribeReg    map[int]string  // entityID → tribe provisionally registered via TAG_CHANGE
	tribeConfirmCount map[string]int  // tribe → count of single-tribe entities confirming it

	// playerEntityIDs maps player entity IDs (e.g. 2,3 for Duos with 4 players)
	// to their PlayerID values. Used for deferred partner resolution.
	playerEntityIDs map[int]int // entityID → PlayerID
	realPlayerIDs   map[int]int // PlayerID → entityID, for all players with real GameAccountIds
}

// NewProcessor returns a Processor that updates the given Machine.
func NewProcessor(m *Machine) *Processor {
	return &Processor{
		machine:          m,
		entityController: make(map[int]int),
		heroEntities:     make(map[int]bool),
		entityProps:      make(map[int]*entityInfo),
		localBuffs:       newBuffTracker(),
		playerEntityIDs:  make(map[int]int),
		realPlayerIDs:    make(map[int]int),
	}
}

// Handle processes a single GameEvent and updates the game state.
func (p *Processor) Handle(e parser.GameEvent) {
	switch e.Type {
	case parser.EventGameStart:
		p.flushPendingStatChanges()
		p.gameSeq++
		p.pendingPlacement = 0
		p.localPlayerID = 0
		p.localPlayerName = ""
		p.localHeroID = 0
		p.partnerPlayerID = 0
		p.partnerPlayerName = ""
		p.partnerHeroID = 0
		p.isDuos = false
		p.entityController = make(map[int]int)
		p.heroEntities = make(map[int]bool)
		p.entityProps = make(map[int]*entityInfo)
		p.localBuffs = newBuffTracker()
		p.localCombatResult = 0
		p.pendingHeroAttackerID = 0
		p.bgTurnsStarted = 0
		p.seenTribes = make(map[string]bool)
		p.entityTribeReg = make(map[int]string)
		p.tribeConfirmCount = make(map[string]int)
		p.playerEntityIDs = make(map[int]int)
		p.realPlayerIDs = make(map[int]int)
		// Derive game ID from CREATE_GAME timestamp for stability across
		// daemon restarts and reparse (plans 23+24). Falls back to gameSeq
		// if timestamp is zero.
		var gameID string
		if !e.Timestamp.IsZero() {
			gameID = fmt.Sprintf("game-%d", e.Timestamp.UnixMilli())
		} else {
			gameID = fmt.Sprintf("game-%d", p.gameSeq)
		}
		p.machine.GameStart(gameID, e.Timestamp)

	case parser.EventPlayerDef:
		p.handlePlayerDef(e)

	case parser.EventPlayerName:
		p.handlePlayerName(e)

	case parser.EventGameEnd:
		p.flushPendingStatChanges()
		// Record the final combat result before ending the game.
		// The TURN-based streak update won't fire after the last combat.
		if p.localCombatResult > 0 {
			p.machine.RecordRoundWin()
		} else if p.localCombatResult < 0 {
			p.machine.RecordRoundLoss()
		}
		p.localCombatResult = 0
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
// In Duos, there are 4 Player entities; 2 have real GameAccountIds.
func (p *Processor) handlePlayerDef(e parser.GameEvent) {
	hi := e.Tags["hi"]
	isReal := hi != "" && hi != "0"

	// Track all player entity IDs for deferred partner resolution.
	if e.EntityID > 0 {
		p.playerEntityIDs[e.EntityID] = e.PlayerID
	}
	if isReal {
		p.realPlayerIDs[e.PlayerID] = e.EntityID
	}

	if isReal && p.localPlayerID == 0 {
		// First real player — this is the local player.
		p.localPlayerID = e.PlayerID
		slog.Info("identified local player", "playerID", p.localPlayerID, "entityID", e.EntityID)
		if heroStr := e.Tags["HERO_ENTITY"]; heroStr != "" {
			if heroID, err := strconv.Atoi(heroStr); err == nil && heroID > 0 {
				p.localHeroID = heroID
				slog.Info("local hero entity set (tentative) from player def", "heroID", heroID)
			}
		}
		// Check for Duos tag in the Player block.
		if duoStr := e.Tags["BACON_DUO_TEAMMATE_PLAYER_ID"]; duoStr != "" {
			if partnerID, err := strconv.Atoi(duoStr); err == nil && partnerID > 0 {
				p.isDuos = true
				p.partnerPlayerID = partnerID
				p.machine.SetDuosMode(true)
				slog.Info("Duos detected from player def", "partnerPlayerID", partnerID)
			}
		}
		// Capture initial state from Player entity tags (critical for reconnects
		// where the entity carries mid-game TURN, RESOURCES, etc.).
		if turn := e.Tags["TURN"]; turn != "" {
			if t, _ := strconv.Atoi(turn); t > 0 {
				p.machine.SetTurn(t)
			}
		}
		if res := e.Tags["RESOURCES"]; res != "" {
			p.machine.UpdateGold("RESOURCES", parseInt(res))
		}
		if used := e.Tags["RESOURCES_USED"]; used != "" {
			p.machine.UpdateGold("RESOURCES_USED", parseInt(used))
		}
	} else if isReal && p.localPlayerID != 0 && e.PlayerID != p.localPlayerID {
		// Second real player in Duos — check if this is the partner.
		if p.isDuos && e.PlayerID == p.partnerPlayerID {
			slog.Info("identified partner player from def", "playerID", e.PlayerID, "entityID", e.EntityID)
			if heroStr := e.Tags["HERO_ENTITY"]; heroStr != "" {
				if heroID, err := strconv.Atoi(heroStr); err == nil && heroID > 0 {
					p.partnerHeroID = heroID
					slog.Info("partner hero entity set (tentative) from player def", "heroID", heroID)
				}
			}
		}
	}
}

// handlePlayerName maps PlayerID to a display name.
func (p *Processor) handlePlayerName(e parser.GameEvent) {
	if e.PlayerID == p.localPlayerID {
		p.localPlayerName = e.EntityName
		p.machine.UpdatePlayerName(e.EntityName)
		slog.Info("local player name", "name", e.EntityName)
	} else if p.isDuos && e.PlayerID == p.partnerPlayerID {
		p.partnerPlayerName = e.EntityName
		p.machine.UpdatePartnerName(e.EntityName)
		slog.Info("partner player name", "name", e.EntityName)
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
		case "BACON_DUO_TEAMMATE_PLAYER_ID":
			// Duos detection: this tag on the local player entity identifies the partner.
			if p.isLocalPlayerEntity(e) {
				partnerID, _ := strconv.Atoi(value)
				if partnerID > 0 && !p.isDuos {
					p.isDuos = true
					p.partnerPlayerID = partnerID
					p.machine.SetDuosMode(true)
					slog.Info("Duos detected", "partnerPlayerID", partnerID)
					// Try to resolve partner name/hero from already-seen player defs.
					if entityID, ok := p.realPlayerIDs[partnerID]; ok {
						if info := p.entityProps[entityID]; info != nil && info.Name != "" {
							p.partnerPlayerName = info.Name
							p.machine.UpdatePartnerName(info.Name)
						}
					}
				}
			}

		case "HEALTH":
			if p.isLocalHero(e, controllerID) {
				p.machine.UpdatePlayerTag(tag, value)
			} else if p.isPartnerHero(e, controllerID) {
				p.machine.UpdatePartnerTag(tag, value)
			} else if e.EntityID > 0 && controllerID == p.localPlayerID {
				p.updateMinionStat(e, "HEALTH", value)
			}

		case "ATK":
			if e.EntityID > 0 && controllerID == p.localPlayerID {
				p.updateMinionStat(e, "ATK", value)
			}

		case "PROPOSED_ATTACKER":
			// GameEntity's PROPOSED_ATTACKER tag fires for every attack during combat.
			// Buffer the hero attacker ID; we resolve the result when PROPOSED_DEFENDER arrives.
			if e.EntityName == "GameEntity" {
				attackerID := parseInt(value)
				if attackerID > 0 && p.heroEntities[attackerID] {
					p.pendingHeroAttackerID = attackerID
				} else {
					p.pendingHeroAttackerID = 0
				}
			}

		case "PROPOSED_DEFENDER":
			// PROPOSED_DEFENDER fires right after PROPOSED_ATTACKER for each attack.
			// When both attacker and defender are heroes, this is the end-of-combat
			// hero attack. In Duos, multiple combats happen per round — only record
			// the result for the combat involving the local hero.
			if e.EntityName == "GameEntity" && p.pendingHeroAttackerID > 0 {
				defenderID := parseInt(value)
				if defenderID > 0 && p.heroEntities[defenderID] {
					// Both attacker and defender are heroes — end-of-combat attack.
					// Winner's hero attacks the loser's hero.
					if p.pendingHeroAttackerID == p.localHeroID {
						// Local hero is the attacker → local won this combat.
						p.localCombatResult = 1
					} else if defenderID == p.localHeroID {
						// Local hero is the defender → local lost this combat.
						p.localCombatResult = -1
					}
					// If neither is the local hero, it's the partner's or another combat — ignore.
				}
				p.pendingHeroAttackerID = 0
			}

		case "DAMAGE":
			if p.isLocalHero(e, controllerID) {
				p.machine.UpdatePlayerTag(tag, value)
			} else if p.isPartnerHero(e, controllerID) {
				p.machine.UpdatePartnerTag(tag, value)
			}

		case "ARMOR":
			// Cache armor for all known hero entities so we can retroactively
			// apply it when the hero is later identified as the local hero.
			if e.EntityID > 0 && p.heroEntities[e.EntityID] {
				if info := p.entityProps[e.EntityID]; info != nil {
					info.Armor = parseInt(value)
				}
			}
			if p.isLocalHero(e, controllerID) {
				p.machine.UpdatePlayerTag(tag, value)
			} else if p.isPartnerHero(e, controllerID) {
				p.machine.UpdatePartnerTag(tag, value)
			}

		case "SPELL_POWER":
			if controllerID == p.localPlayerID || controllerID == 0 {
				p.machine.UpdatePlayerTag(tag, value)
			}

		case "PLAYER_TECH_LEVEL", "TAVERN_TIER":
			// Guard: only accept tier from a positively-identified local entity.
			// Require controllerID to be known (non-zero) and match localPlayerID,
			// OR fall back to isLocalPlayerEntity only when controllerID is unknown.
			// This prevents controllerID==0==localPlayerID from matching when
			// localPlayerID has not yet been set (i.e. before CREATE_GAME resolves).
			isLocal := (controllerID != 0 && controllerID == p.localPlayerID) ||
				(controllerID == 0 && p.isLocalPlayerEntity(e))
			if isLocal {
				tier, _ := strconv.Atoi(value)
				if tier > 0 {
					p.machine.SetTavernTier(tier)
				}
			} else if p.isPartnerHero(e, controllerID) || p.isPartnerPlayerEntity(e) {
				tier, _ := strconv.Atoi(value)
				if tier > 0 {
					p.machine.UpdatePartnerTag(tag, value)
				}
			}

		case "PLAYER_TRIPLES":
			// PLAYER_TRIPLES is set on the hero entity with the cumulative count.
			if p.isLocalHero(e, controllerID) || p.isLocalPlayerEntity(e) {
				p.machine.UpdatePlayerTag(tag, value)
			} else if p.isPartnerHero(e, controllerID) || p.isPartnerPlayerEntity(e) {
				p.machine.UpdatePartnerTag(tag, value)
			}

		case "RESOURCES", "RESOURCES_USED":
			// Gold tracking: RESOURCES = total gold, RESOURCES_USED = spent gold.
			// These fire on the player entity (not hero), so use isLocalPlayerEntity.
			if p.isLocalPlayerEntity(e) {
				p.machine.UpdateGold(tag, parseInt(value))
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
					p.handleOverconfidenceZone(info.CardID, value, prevZone, controllerID)
				}
			}
			if value == "PLAY" && e.EntityID > 0 {
				if p.machine.Phase() != PhaseGameOver {
					p.tryAddMinionFromRegistry(e.EntityID, controllerID)
				}
			} else if e.EntityID > 0 && p.machine.Phase() != PhaseGameOver {
				// Remove from board on any non-PLAY zone transition (GRAVEYARD,
				// REMOVEDFROMGAME, SETASIDE, HAND, DECK, etc.). This catches
				// sold minions (PLAY->HAND) that were previously missed.
				p.machine.RemoveMinion(e.EntityID)
				p.machine.RemoveEnchantmentsForEntity(e.EntityID)
				if p.machine.Phase() == PhaseRecruit {
					p.machine.UpdateBoardSnapshot()
				}
			}

		case "TURN":
			// Player-specific TURN tag (not GameEntity).
			// This gives us the actual BG turn number the player sees.
			if p.isLocalPlayerEntity(e) {
				turn, _ := strconv.Atoi(value)
				if turn > 0 {
					// Record outcome of the combat that just resolved (turns 2+).
					// localCombatResult is set by PROPOSED_ATTACKER/DEFENDER hero pairs.
					if p.bgTurnsStarted > 0 {
						if p.localCombatResult > 0 {
							p.machine.RecordRoundWin()
						} else if p.localCombatResult < 0 {
							p.machine.RecordRoundLoss()
						}
						// localCombatResult == 0 → tie or no hero attack — no streak change.
					}
					p.localCombatResult = 0
					p.bgTurnsStarted++
					p.flushPendingStatChanges()

					// Reset Overconfidence count at turn boundary.
					if p.localBuffs.overconfidenceCount > 0 {
						p.localBuffs.overconfidenceCount = 0
						p.updateGoldNextTurnCounter()
					}

					p.machine.SetTurn(turn)
				}
			}

		case "HERO_ENTITY":
			// Track which hero entity belongs to the local player.
			if p.isLocalPlayerEntity(e) {
				heroID, _ := strconv.Atoi(value)
				if heroID > 0 && heroID != p.localHeroID {
					shouldUpdate := p.localHeroID == 0
					if !shouldUpdate {
						if info := p.entityProps[p.localHeroID]; info != nil {
							shouldUpdate = strings.HasPrefix(info.CardID, "TB_BaconShop_HERO_PH")
						}
					}
					if shouldUpdate {
						p.localHeroID = heroID
						slog.Info("local hero entity updated", "heroID", heroID)
						if info := p.entityProps[heroID]; info != nil {
							if info.Health > 0 {
								p.machine.UpdatePlayerTag("HEALTH", strconv.Itoa(info.Health))
							}
							if info.Armor > 0 {
								p.machine.UpdatePlayerTag("ARMOR", strconv.Itoa(info.Armor))
							}
							if info.CardID != "" && !strings.HasPrefix(info.CardID, "TB_BaconShop_HERO_PH") {
								p.machine.UpdateHeroCardID(info.CardID)
							}
						}
					}
				}
			} else if p.isPartnerPlayerEntity(e) {
				heroID, _ := strconv.Atoi(value)
				if heroID > 0 && heroID != p.partnerHeroID {
					shouldUpdate := p.partnerHeroID == 0
					if !shouldUpdate {
						if info := p.entityProps[p.partnerHeroID]; info != nil {
							shouldUpdate = strings.HasPrefix(info.CardID, "TB_BaconShop_HERO_PH")
						}
					}
					if shouldUpdate {
						p.partnerHeroID = heroID
						slog.Info("partner hero entity updated", "heroID", heroID)
						if info := p.entityProps[heroID]; info != nil {
							if info.Health > 0 {
								p.machine.UpdatePartnerTag("HEALTH", strconv.Itoa(info.Health))
							}
							if info.Armor > 0 {
								p.machine.UpdatePartnerTag("ARMOR", strconv.Itoa(info.Armor))
							}
							if info.CardID != "" && !strings.HasPrefix(info.CardID, "TB_BaconShop_HERO_PH") {
								p.machine.UpdatePartnerHeroCardID(info.CardID)
							}
						}
					}
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
				p.localBuffs.goldNextTurnSure = raw
				p.updateGoldNextTurnCounter()
			}

		case "TAG_SCRIPT_DATA_NUM_1", "TAG_SCRIPT_DATA_NUM_2":
			if e.EntityID > 0 {
				p.updateEnchantmentScriptData(e.EntityID, tag, value)
				// Only process enchantments controlled by local player.
				ctrl := p.entityController[e.EntityID]
				if ctrl == p.localPlayerID {
					p.handleDntTagChange(e.EntityID, tag, parseInt(value))
				}
			}

		case "3809":
			// SpellsPlayedForNagasCounter (HDT) — total spells played this game for
			// Thaumaturgist/ArcaneCannoneer/ShowyCyclist/Groundbreaker synergy.
			// Only show when one of those minions is on the board (mirrors HDT ShouldShow).
			if p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID) {
				raw, _ := strconv.Atoi(value)
				snap := p.machine.State()
				if HasNagaSynergyMinion(snap.Board) {
					stacks := 1 + (raw / 4)
					progress := raw % 4
					display := fmt.Sprintf("Tier %d · %d/4", stacks, progress)
					p.machine.SetAbilityCounter(CatNagaSpells, raw, display)
				} else {
					p.machine.RemoveAbilityCounter(CatNagaSpells)
				}
			}

		case "CONTROLLER":
			// Update entity controller registry.
			if e.EntityID > 0 {
				ctrl, _ := strconv.Atoi(value)
				p.entityController[e.EntityID] = ctrl
			}

		default:
			// Handle BACON_SUBSET_* TAG_CHANGE events for tribe discovery.
			// TAG_CHANGEs arrive individually, so multi-tribe minions fire
			// separate events. We track per-entity subset counts: provisionally
			// register on the first subset, revoke if a second arrives.
			if strings.HasPrefix(tag, baconSubsetPrefix) && value == "1" && e.EntityID > 0 {
				suffix := tag[len(baconSubsetPrefix):]
				if tribe, ok := baconSubsetToTribe[suffix]; ok {
					info := p.entityProps[e.EntityID]
					if info == nil {
						info = &entityInfo{}
						p.entityProps[e.EntityID] = info
					}
					info.Subsets++
					if info.Subsets == 1 {
						// First subset — provisionally register.
						if p.entityTribeReg == nil {
							p.entityTribeReg = make(map[int]string)
						}
						p.entityTribeReg[e.EntityID] = tribe
						p.registerTribeConfirmation(tribe)
					} else if info.Subsets == 2 {
						// Second subset — entity is multi-tribe; revoke.
						if prevTribe, ok := p.entityTribeReg[e.EntityID]; ok {
							delete(p.entityTribeReg, e.EntityID)
							p.revokeTribeConfirmation(prevTribe)
						}
					}
				}
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

	// Detect available tribes from BACON_SUBSET_* tags in FULL_ENTITY blocks.
	// Only count entities with exactly ONE BACON_SUBSET tag (single-tribe minions).
	// Multi-tribe minions (2+ subset tags) appear via any of their tribes, so
	// they can bleed banned tribes into the detected set.
	p.trackTribesFromEntity(e.EntityID, e.Tags)

	// Register hero entity IDs.
	if cardType == "HERO" && e.EntityID > 0 {
		p.heroEntities[e.EntityID] = true
	}

	// Detect anomaly from FULL_ENTITY with CARDTYPE=BATTLEGROUND_ANOMALY.
	if cardType == "BATTLEGROUND_ANOMALY" && info.CardID != "" {
		name := CardName(info.CardID)
		if name == "" {
			name = info.CardID
		}
		desc := CardDescription(info.CardID)
		p.machine.SetAnomaly(info.CardID, name, desc)
		return
	}

	// Handle enchantment entities — track buff sources.
	if cardType == "ENCHANTMENT" {
		p.handleEnchantmentEntity(e, info)
		return
	}

	// If this is a HERO entity owned by the local player, track its stats.
	if cardType == "HERO" && controllerID == p.localPlayerID {
		if p.localHeroID > 0 && e.EntityID != p.localHeroID {
			return
		}
		if hp, ok := e.Tags["HEALTH"]; ok {
			p.machine.UpdatePlayerTag("HEALTH", hp)
		}
		if dmg, ok := e.Tags["DAMAGE"]; ok {
			p.machine.UpdatePlayerTag("DAMAGE", dmg)
		}
		if armor, ok := e.Tags["ARMOR"]; ok {
			p.machine.UpdatePlayerTag("ARMOR", armor)
		}
		if tier, ok := e.Tags["PLAYER_TECH_LEVEL"]; ok {
			if t, _ := strconv.Atoi(tier); t > 0 {
				p.machine.SetTavernTier(t)
			}
		}
		if triples, ok := e.Tags["PLAYER_TRIPLES"]; ok {
			p.machine.UpdatePlayerTag("PLAYER_TRIPLES", triples)
		}
		if e.CardID != "" && !strings.HasPrefix(e.CardID, "TB_BaconShop_HERO_PH") {
			p.machine.UpdateHeroCardID(e.CardID)
		}
		return
	}
	// Partner hero entity — identified by PLAYER_ID tag matching partner.
	// In BG Duos, the partner hero has CONTROLLER=<botID> but PLAYER_ID=<partnerPlayerID>.
	if cardType == "HERO" && p.isDuos && p.partnerPlayerID > 0 {
		if pidStr, ok := e.Tags["PLAYER_ID"]; ok {
			pid, _ := strconv.Atoi(pidStr)
			if pid == p.partnerPlayerID {
				if p.partnerHeroID == 0 || e.EntityID == p.partnerHeroID {
					p.partnerHeroID = e.EntityID
					p.heroEntities[e.EntityID] = true
					slog.Info("partner hero identified via PLAYER_ID tag",
						"entityID", e.EntityID, "cardID", e.CardID, "playerID", pid)
					if hp, ok := e.Tags["HEALTH"]; ok {
						p.machine.UpdatePartnerTag("HEALTH", hp)
					}
					if dmg, ok := e.Tags["DAMAGE"]; ok {
						p.machine.UpdatePartnerTag("DAMAGE", dmg)
					}
					if armor, ok := e.Tags["ARMOR"]; ok {
						p.machine.UpdatePartnerTag("ARMOR", armor)
					}
					if tier, ok := e.Tags["PLAYER_TECH_LEVEL"]; ok {
						p.machine.UpdatePartnerTag("PLAYER_TECH_LEVEL", tier)
					}
					if triples, ok := e.Tags["PLAYER_TRIPLES"]; ok {
						p.machine.UpdatePartnerTag("PLAYER_TRIPLES", triples)
					}
					if e.CardID != "" && !strings.HasPrefix(e.CardID, "TB_BaconShop_HERO_PH") {
						p.machine.UpdatePartnerHeroCardID(e.CardID)
					}
					if e.EntityName != "" && p.partnerPlayerName == "" {
						p.partnerPlayerName = cleanEntityName(e.EntityName)
						p.machine.UpdatePartnerName(p.partnerPlayerName)
					}
					return
				}
			}
		}
	}
	// Partner hero entity — fallback via controller match.
	if cardType == "HERO" && p.isDuos && controllerID == p.partnerPlayerID {
		if p.partnerHeroID > 0 && e.EntityID != p.partnerHeroID {
			return
		}
		if hp, ok := e.Tags["HEALTH"]; ok {
			p.machine.UpdatePartnerTag("HEALTH", hp)
		}
		if dmg, ok := e.Tags["DAMAGE"]; ok {
			p.machine.UpdatePartnerTag("DAMAGE", dmg)
		}
		if armor, ok := e.Tags["ARMOR"]; ok {
			p.machine.UpdatePartnerTag("ARMOR", armor)
		}
		if tier, ok := e.Tags["PLAYER_TECH_LEVEL"]; ok {
			p.machine.UpdatePartnerTag("PLAYER_TECH_LEVEL", tier)
		}
		if triples, ok := e.Tags["PLAYER_TRIPLES"]; ok {
			p.machine.UpdatePartnerTag("PLAYER_TRIPLES", triples)
		}
		if e.CardID != "" && !strings.HasPrefix(e.CardID, "TB_BaconShop_HERO_PH") {
			p.machine.UpdatePartnerHeroCardID(e.CardID)
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

	if p.machine.Phase() == PhaseGameOver {
		return
	}
	mn := MinionState{
		EntityID: e.EntityID,
		CardID:   info.CardID,
		Name:     info.Name,
		Attack:   info.Attack,
		Health:   info.Health,
	}
	if mn.Name == "" && mn.CardID != "" {
		mn.Name = CardName(mn.CardID)
	}
	if controllerID == p.localPlayerID {
		p.machine.UpsertMinion(mn)
		if p.machine.Phase() == PhaseRecruit {
			p.machine.UpdateBoardSnapshot()
		}
	}
}

// resolveController returns the controller PlayerID for the entity in a
// GameEvent. It checks (in order): the entity controller registry (updated by
// CONTROLLER TAG_CHANGE events), then the event's PlayerID field (from player=N
// in the log's entity notation), and falls back to 0 (unknown).
// The registry is checked first because CONTROLLER TAG_CHANGE events update
// ownership, while the player= annotation in the log is stale after ownership changes.
func (p *Processor) resolveController(e parser.GameEvent) int {
	if e.EntityID > 0 {
		if ctrl, ok := p.entityController[e.EntityID]; ok {
			return ctrl
		}
	}
	if e.PlayerID > 0 {
		return e.PlayerID
	}
	return 0
}

// isLocalPlayerEntity checks whether the event's entity is the local player
// entity itself (not a hero/minion, but the Player entity).
func (p *Processor) isLocalPlayerEntity(e parser.GameEvent) bool {
	// Prefer PlayerID match — most reliable.
	if p.localPlayerID > 0 && e.PlayerID == p.localPlayerID {
		return true
	}
	// If we have a positive localPlayerID, trust it over the name.
	// Only use name as a last resort when localPlayerID is not yet known.
	if p.localPlayerID == 0 && p.localPlayerName != "" && e.EntityName == p.localPlayerName {
		slog.Warn("isLocalPlayerEntity: using name fallback — localPlayerID not yet set",
			"name", p.localPlayerName, "entityID", e.EntityID)
		return true
	}
	// Name fallback for bare-name entity references (no player= field in the log line).
	// TAG_CHANGE Entity=Alice has PlayerID=0; TAG_CHANGE Entity=[... player=15] has PlayerID=15.
	// So if localPlayerID is known but e.PlayerID is 0, a name match is still safe.
	if p.localPlayerID > 0 && e.PlayerID == 0 && p.localPlayerName != "" && e.EntityName == p.localPlayerName {
		return true
	}
	return false
}

// isPartnerPlayerEntity checks whether the event's entity is the partner player entity.
func (p *Processor) isPartnerPlayerEntity(e parser.GameEvent) bool {
	if !p.isDuos || p.partnerPlayerID == 0 {
		return false
	}
	if e.PlayerID == p.partnerPlayerID {
		return true
	}
	if e.PlayerID == 0 && p.partnerPlayerName != "" && e.EntityName == p.partnerPlayerName {
		return true
	}
	return false
}

// isPartnerHero checks whether the entity is the partner's hero card.
func (p *Processor) isPartnerHero(e parser.GameEvent, controllerID int) bool {
	if !p.isDuos || e.EntityID <= 0 {
		return false
	}
	// Direct entity ID match — set during handleEntityUpdate via PLAYER_ID tag.
	if p.partnerHeroID > 0 && e.EntityID == p.partnerHeroID {
		return true
	}
	// Fallback: controller-based match.
	if controllerID != p.partnerPlayerID {
		return false
	}
	return p.heroEntities[e.EntityID]
}

// isLocalHero checks whether the entity in the event is the local player's
// hero card. Only matches entities known to be heroes (CARDTYPE=HERO) that
// are controlled by the local player.
func (p *Processor) isLocalHero(e parser.GameEvent, controllerID int) bool {
	if e.EntityID <= 0 || controllerID != p.localPlayerID {
		return false
	}
	// Once localHeroID is known, only ever match the exact entity.
	// This prevents combat-copy heroes (opponent heroes with player=localPlayerID)
	// from being treated as the local hero.
	if p.localHeroID > 0 {
		return e.EntityID == p.localHeroID
	}
	// Before the hero entity is identified, fall back to the HERO-type registry.
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
	if onBoard && p.machine.Phase() == PhaseRecruit {
		p.machine.UpdateBoardSnapshot()
	}

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
		if len(p.pendingStatChanges) > maxPendingStatChanges {
			slog.Warn("pendingStatChanges cap reached, flushing early",
				"count", len(p.pendingStatChanges))
			p.flushPendingStatChanges()
		}
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
	mn := MinionState{
		EntityID: entityID,
		CardID:   info.CardID,
		Name:     info.Name,
		Attack:   info.Attack,
		Health:   info.Health,
	}
	if mn.Name == "" && mn.CardID != "" {
		mn.Name = CardName(mn.CardID)
	}
	p.machine.UpsertMinion(mn)
	if p.machine.Phase() == PhaseRecruit {
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

	// Check if the target is a local player's board minion.
	targetCtrl := p.entityController[info.AttachedTo]
	enchCtrl := p.entityController[e.EntityID]
	isLocalMinion := targetCtrl == p.localPlayerID

	if !isLocalMinion {
		isLocalEnch := enchCtrl == p.localPlayerID
		if !isLocalEnch {
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
		if enchCtrl == p.localPlayerID {
			if info.ScriptData1 != 0 {
				p.handleDntTagChange(e.EntityID, "TAG_SCRIPT_DATA_NUM_1", info.ScriptData1)
			}
			if info.ScriptData2 != 0 {
				p.handleDntTagChange(e.EntityID, "TAG_SCRIPT_DATA_NUM_2", info.ScriptData2)
			}
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
		p.handleShopBuffDnt(entityID, isSD1, value)
	case "BG30_MagicItem_544pe":
		p.handleNomiStickerDnt(entityID, isSD1, value)
	case "BG34_855pe":
		p.handleNomiAllDnt(entityID, isSD1, value)
	case "BG31_808pe":
		p.handleAbsoluteDnt(CatBeetle, isSD1, value, 1, 1)
	case "BG34_854pe":
		p.handleAbsoluteDnt(CatRightmost, isSD1, value, 0, 0)
	case "BG34_402pe":
		p.handleAbsoluteDnt(CatWhelp, isSD1, value, 0, 0)
	case "BG25_011pe":
		if isSD1 {
			p.machine.SetBuffSource(CatUndead, value, 0)
		}
	case "BG34_170e":
		p.handleAbsoluteDnt(CatVolumizer, isSD1, value, 0, 0)
	case "BG34_689e2":
		p.handleAbsoluteDnt(CatBloodgemBarrage, isSD1, value, 0, 0)
	default:
		if cardID != "" && value != 0 {
			slog.Debug("untracked Dnt enchantment", "cardID", cardID, "tag", tag, "value", value, "entityID", entityID)
		}
	}
}

// handleAbsoluteDnt sets a buff source from an absolute Dnt value plus base offset.
func (p *Processor) handleAbsoluteDnt(category string, isSD1 bool, value, baseAtk, baseHp int) {
	bt := &p.localBuffs
	state := bt.buffSourceState[category]
	if isSD1 {
		state[0] = baseAtk + value
	} else {
		state[1] = baseHp + value
	}
	bt.buffSourceState[category] = state
	p.machine.SetBuffSource(category, state[0], state[1])
}

// handleShopBuffDnt handles BG_ShopBuff_Elemental with differential accumulation.
func (p *Processor) handleShopBuffDnt(entityID int, isSD1 bool, value int) {
	bt := &p.localBuffs
	prev := bt.shopBuffPrev[entityID]
	var delta int
	if isSD1 {
		delta = value - prev[0]
		prev[0] = value
	} else {
		delta = value - prev[1]
		prev[1] = value
	}
	bt.shopBuffPrev[entityID] = prev

	if isSD1 {
		bt.nomiCounter[0] += delta
	} else {
		bt.nomiCounter[1] += delta
	}
	p.machine.SetBuffSource(CatNomi, bt.nomiCounter[0], bt.nomiCounter[1])
}

// handleNomiAllDnt handles BG34_855pe (Timewarped Nomi / Kitchen Dream) with differential
// accumulation. Same pattern as regular Nomi but tracked under CatNomiAll.
func (p *Processor) handleNomiAllDnt(entityID int, isSD1 bool, value int) {
	bt := &p.localBuffs
	prev := bt.shopBuffPrev[entityID]
	var delta int
	if isSD1 {
		delta = value - prev[0]
		prev[0] = value
	} else {
		delta = value - prev[1]
		prev[1] = value
	}
	bt.shopBuffPrev[entityID] = prev

	if isSD1 {
		bt.nomiAllCounter[0] += delta
	} else {
		bt.nomiAllCounter[1] += delta
	}
	p.machine.SetBuffSource(CatNomiAll, bt.nomiAllCounter[0], bt.nomiAllCounter[1])
}

// handleNomiStickerDnt handles BG30_MagicItem_544pe where SD1 applies to BOTH atk and hp.
func (p *Processor) handleNomiStickerDnt(entityID int, isSD1 bool, value int) {
	bt := &p.localBuffs
	prev := bt.shopBuffPrev[entityID]
	if isSD1 {
		delta := value - prev[0]
		prev[0] = value
		bt.shopBuffPrev[entityID] = prev
		bt.nomiCounter[0] += delta
		bt.nomiCounter[1] += delta
	} else {
		prev[1] = value
		bt.shopBuffPrev[entityID] = prev
	}
	p.machine.SetBuffSource(CatNomi, bt.nomiCounter[0], bt.nomiCounter[1])
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

	bt := &p.localBuffs
	state := bt.buffSourceState[category]
	if isATK {
		state[0] = computedVal
	} else {
		state[1] = computedVal
	}
	bt.buffSourceState[category] = state
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

// baconSubsetPrefix is the tag prefix for tribe membership on pool minions.
const baconSubsetPrefix = "BACON_SUBSET_"

// baconSubsetToTribe maps the suffix of BACON_SUBSET_* tags to display tribe names.
var baconSubsetToTribe = map[string]string{
	"BEAST":      "Beast",
	"DEMON":      "Demon",
	"DRAGON":     "Dragon",
	"ELEMENTALS": "Elemental",
	"MECH":       "Mech",
	"MURLOC":     "Murloc",
	"NAGA":       "Naga",
	"PIRATE":     "Pirate",
	"QUILLBOAR":  "Quilboar",
	"UNDEAD":     "Undead",
}

// trackTribesFromEntity examines all BACON_SUBSET_* tags on an entity and
// registers the tribe as available ONLY if the entity has exactly one subset
// tag (single-tribe minion). Multi-tribe minions (2+ subset tags) can appear
// in the pool via any of their tribes, so they would incorrectly mark banned
// tribes as available.
func (p *Processor) trackTribesFromEntity(entityID int, tags map[string]string) {
	var tribe string
	count := 0
	for tag, value := range tags {
		if !strings.HasPrefix(tag, baconSubsetPrefix) || value != "1" {
			continue
		}
		suffix := tag[len(baconSubsetPrefix):]
		if t, ok := baconSubsetToTribe[suffix]; ok {
			tribe = t
			count++
		}
	}
	if count == 0 {
		return
	}
	// Update entity subset count for TAG_CHANGE multi-tribe detection.
	if info := p.entityProps[entityID]; info != nil {
		info.Subsets = count
	}
	if count != 1 {
		return // multi-tribe entity — skip
	}
	p.registerTribeConfirmation(tribe)
}

// registerTribeConfirmation increments the confirmation count for a tribe
// and adds it to available tribes if this is the first confirmation.
func (p *Processor) registerTribeConfirmation(tribe string) {
	if p.tribeConfirmCount == nil {
		p.tribeConfirmCount = make(map[string]int)
	}
	p.tribeConfirmCount[tribe]++
	if p.seenTribes == nil {
		p.seenTribes = make(map[string]bool)
	}
	if !p.seenTribes[tribe] {
		p.seenTribes[tribe] = true
		p.machine.AddAvailableTribe(tribe)
	}
}

// revokeTribeConfirmation decrements the confirmation count for a tribe
// and removes it from available tribes if no single-tribe entities remain.
func (p *Processor) revokeTribeConfirmation(tribe string) {
	if p.tribeConfirmCount == nil {
		return
	}
	p.tribeConfirmCount[tribe]--
	if p.tribeConfirmCount[tribe] <= 0 {
		delete(p.tribeConfirmCount, tribe)
		if p.seenTribes[tribe] {
			delete(p.seenTribes, tribe)
			p.machine.RemoveAvailableTribe(tribe)
		}
	}
}

// overconfidenceCardID is the Dnt enchantment for Overconfidence (BG28_884e).
const overconfidenceCardID = "BG28_884e"

// handleOverconfidenceZone tracks Overconfidence Dnt enchantments entering/leaving PLAY.
// Each active Overconfidence contributes +3 potential gold next turn.
func (p *Processor) handleOverconfidenceZone(cardID, newZone, prevZone string, controllerID int) {
	if cardID != overconfidenceCardID {
		return
	}
	// Only track overconfidence for the local player.
	if controllerID != p.localPlayerID {
		return
	}
	bt := &p.localBuffs
	if newZone == "PLAY" && prevZone != "PLAY" {
		bt.overconfidenceCount++
		p.updateGoldNextTurnCounter()
	} else if newZone != "PLAY" && prevZone == "PLAY" {
		bt.overconfidenceCount--
		if bt.overconfidenceCount < 0 {
			bt.overconfidenceCount = 0
		}
		p.updateGoldNextTurnCounter()
	}
}

// updateGoldNextTurnCounter updates the GoldNextTurn ability counter display.
func (p *Processor) updateGoldNextTurnCounter() {
	bt := &p.localBuffs
	sure := bt.goldNextTurnSure
	bonus := bt.overconfidenceCount * 3
	if sure == 0 && bt.overconfidenceCount == 0 {
		return
	}
	var display string
	if bonus > 0 {
		display = fmt.Sprintf("%d (+%d if win)", sure, bonus)
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
