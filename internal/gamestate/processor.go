package gamestate

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
)

// entityInfo stores known properties of an entity for board tracking.
type entityInfo struct {
	CardID       string
	Name         string
	CardType     string
	Race         string // CARDRACE tag (e.g. "BEAST", "DRAGON", "MECHANICAL")
	Attack       int
	Health       int
	Armor        int // cached for retroactive hero identification
	Zone         string
	ZonePosition int // ZONE_POSITION tag (>0 for initial board minions)
	CreatorID    int
	AttachedTo   int
	ScriptData1  int
	ScriptData2  int
	Subsets      int // count of BACON_SUBSET_* tags seen (for multi-tribe detection)
	PlayerID     int // PLAYER_ID tag value (for hero entities in duos)
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

// combatCopyPeak tracks peak ATK/HEALTH observed on a combat copy.
// During combat setup, copies briefly show real (enchantment-inclusive) stats
// before being stripped to base. The peak values represent the recruit board stats.
type combatCopyPeak struct {
	CardID string
	ATK    int
	Health int
}

// buffTracker holds buff source tracking state for the local player.
// Encapsulates buff source state, Dnt counters, and economy counters.
type buffTracker struct {
	buffSourceState     map[string][2]int
	shopBuffPrev        map[int][2]int
	shopBuffGlobal      map[string][2]int // last absolute value per cumulative-Dnt category (BG_ShopBuff)
	nomiCounter         [2]int
	nomiAllCounter      [2]int
	goldNextTurnSure    int
	overconfidenceCount int
}

func newBuffTracker() buffTracker {
	return buffTracker{
		buffSourceState: make(map[string][2]int),
		shopBuffPrev:    make(map[int][2]int),
		shopBuffGlobal:  make(map[string][2]int),
	}
}

// reconnectStash holds game-level state saved before an EventGameStart reset.
// If the subsequent EventGameEntityTags confirms a reconnect (STATE=RUNNING,
// TURN > 1), this state is restored. Otherwise it is discarded.
type reconnectStash struct {
	gameID             string
	startTime          time.Time
	turn               int
	tavernTier         int
	isDuos             bool
	duosFromTeammate   bool
	partnerPlayerID    int
	partnerPlayerName  string
	heroCardID         string
	partnerHeroCardID  string
	turnSnapshots      []TurnSnapshot
	buffSources        []BuffSource
	abilityCounters    []AbilityCounter
	partnerBuffSources []BuffSource
	partnerAbilityCtrs []AbilityCounter
	modifications      []StatMod
	prevBuffSources    []BuffSource
	prevAbilityCtrs    []AbilityCounter
	prevModCount       int
	anomalyCardID      string
	anomalyName        string
	anomalyDescription string
	availableTribes    []string
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
	partnerPlayerID     int // CONTROLLER value for the partner (from BACON_DUO_TEAMMATE_PLAYER_ID)
	partnerPlayerName   string
	partnerHeroID       int // entity ID of the partner's hero card
	isDuos              bool
	punishLeaversActive bool // BACON_DUOS_PUNISH_LEAVERS=1 seen (not sufficient alone)
	duosFromTeammate    bool // duos confirmed via BACON_DUO_TEAMMATE_PLAYER_ID (authoritative)

	// Partner combat tracking
	partnerCombatActive   bool          // true while partner's combat is in progress
	partnerCombatHeroCtrl int           // CONTROLLER of partner's hero copy in combat
	partnerCombatMinions  []MinionState // collected partner minions during combat
	opponentCombatMinions []MinionState // collected opponent minions during partner's combat
	partnerBoardSetupDone bool          // true after first combat action (PROPOSED_ATTACKER) — stops collection
	combatPhaseActive     bool          // true during the combat phase (BACON_CURRENT_COMBAT_PLAYER_ID > 0)
	combatPhaseEntityIDs  []int         // entity IDs created during current combat phase (for retroactive scan)

	// Duos absolute Dnt split tracking — separates local vs partner contributions.
	// dntTeamTotal tracks the last-seen raw SD value per category (no base offset).
	// dntPartnerAccum tracks the accumulated partner delta per category.
	dntTeamTotal    map[string][2]int // category → [sd1, sd2] raw values
	dntPartnerAccum map[string][2]int // category → [sd1, sd2] partner deltas

	// Entity registry — maps entity IDs to their controller PlayerIDs.
	entityController map[int]int
	// heroEntities tracks entity IDs known to be HERO card types.
	heroEntities map[int]bool
	// entityProps tracks known properties of entities for zone transition handling.
	entityProps map[int]*entityInfo

	// Buffered stat changes for board-wide buff detection.
	pendingStatChanges []pendingStatChange

	// Buff tracking for local player.
	localBuffs   buffTracker
	partnerBuffs buffTracker // tracks partner buff sources from combat enchantments

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

	// Staleness tracking
	lastEventTime time.Time

	// Reconnect state preservation
	reconnectStash *reconnectStash
	isReconnect    bool

	// Combat copy peak stats — tracks the max ATK/HEALTH seen on each
	// local-player combat copy (SETASIDE minions during combat). Used to
	// recover real stats for minions whose recruit-phase buffs are purely
	// enchantment-based and never appear as direct TAG_CHANGE events.
	combatCopies map[int]*combatCopyPeak // entityID → peak stats

	// Available tribes detected from BACON_SUBSET_* tags.
	seenTribes        map[string]bool
	entityTribeReg    map[int]string // entityID → tribe provisionally registered via TAG_CHANGE
	tribeConfirmCount map[string]int // tribe → count of single-tribe entities confirming it

	// playerEntityIDs maps player entity IDs (e.g. 2,3 for Duos with 4 players)
	// to their PlayerID values. Used for deferred partner resolution.
	playerEntityIDs map[int]int // entityID → PlayerID
	realPlayerIDs   map[int]int // PlayerID → entityID, for all players with real GameAccountIds

	// tripleFormationActive is set when TB_BaconShop_3ofKindChecke goes GRAVEYARD and
	// cleared after PLAYER_TRIPLES increments. While active, MINIONS_SOLD increments are
	// suppressed so that triple-consumed board minions are not counted as sells regardless
	// of whether they came from the board or hand.
	tripleFormationActive bool

	// spellsPlayedTotal is the last-seen absolute value of NUM_SPELLS_PLAYED_THIS_GAME
	// on the local player entity. Used to compute the delta when the tag increments.
	spellsPlayedTotal int

	// prevSpellcraftValue is the last-seen value of tag 3809 on the local player entity.
	// A 0→positive transition during PhaseCombat indicates a spellcraft trigger fired.
	prevSpellcraftValue int
}

// NewProcessor returns a Processor that updates the given Machine.
func NewProcessor(m *Machine) *Processor {
	return &Processor{
		machine:          m,
		entityController: make(map[int]int),
		heroEntities:     make(map[int]bool),
		entityProps:      make(map[int]*entityInfo),
		localBuffs:       newBuffTracker(),
		partnerBuffs:     newBuffTracker(),
		dntTeamTotal:     make(map[string][2]int),
		dntPartnerAccum:  make(map[string][2]int),
		playerEntityIDs:  make(map[int]int),
		realPlayerIDs:    make(map[int]int),
	}
}

// Handle processes a single GameEvent and updates the game state.
func (p *Processor) Handle(e parser.GameEvent) {
	p.lastEventTime = time.Now()

	// For migrated event types, Handle() is a thin adapter: it constructs the typed
	// action and calls the visitor method. Direct callers (tests, legacy code) continue
	// to work. The dispatcher path calls the visitor methods directly (no double-call).

	switch e.Type {
	case parser.EventGameStart:
		var gameID string
		if !e.Timestamp.IsZero() {
			gameID = "game-" + strconv.FormatInt(e.Timestamp.UnixMilli(), 10)
		}
		_ = p.OnGameStart(&action.GameStartAction{
			ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
			GameID:     gameID,
			Timestamp:  e.Timestamp,
		})

	case parser.EventPlayerDef:
		hi, _ := strconv.ParseUint(e.Tags["hi"], 10, 64)
		tag := e.EntityName
		if tag == "" {
			tag = e.Tags["BATTLETAG"]
		}
		_ = p.OnPlayerDef(&action.PlayerDefAction{
			ActionBase:    action.ActionBase{Entity: action.EntityID(e.EntityID)},
			PlayerID:      e.PlayerID,
			BattleTag:     tag,
			AccountHi:     hi,
			HeroEntityID:  parseInt(e.Tags["HERO_ENTITY"]),
			DuoTeammateID: parseInt(e.Tags["BACON_DUO_TEAMMATE_PLAYER_ID"]),
			InitialTurn:   parseInt(e.Tags["TURN"]),
			Resources:     parseInt(e.Tags["RESOURCES"]),
			ResourcesUsed: parseInt(e.Tags["RESOURCES_USED"]),
		})

	case parser.EventPlayerName:
		_ = p.OnPlayerName(&action.PlayerNameAction{
			ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
			PlayerID:   e.PlayerID,
			BattleTag:  e.EntityName,
		})

	case parser.EventTurnStart:
		turn, _ := strconv.Atoi(e.Tags["TURN"])
		newPhase := action.PhaseRecruit
		if turn%2 == 0 {
			newPhase = action.PhaseCombat
		}
		_ = p.OnTurnTransition(&action.TurnTransitionAction{
			ActionBase:     action.ActionBase{Entity: action.EntityID(e.EntityID)},
			NewPhase:       newPhase,
			GameEntityTurn: turn,
		})

	case parser.EventGameEnd:
		placement := p.pendingPlacement
		if pl := e.Tags["PLAYER_LEADERBOARD_PLACE"]; pl != "" {
			if v, err := strconv.Atoi(pl); err == nil {
				placement = v
			}
		}
		_ = p.OnGameEnd(&action.GameEndAction{
			ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
			Placement:  placement,
			Timestamp:  e.Timestamp,
		})

	case parser.EventGameEntityTags:
		// Thin adapter: delegate to OnReconnect. PunishLeaversActive is read from tags
		// so the backup duos detection path works for both the dispatcher and direct Handle() callers.
		state := e.Tags["STATE"]
		turn, _ := strconv.Atoi(e.Tags["TURN"])
		_ = p.OnReconnect(&action.ReconnectAction{
			ActionBase:          action.ActionBase{Entity: action.EntityID(e.EntityID)},
			IsRunning:           state == "RUNNING",
			Turn:                turn,
			PunishLeaversActive: e.Tags["BACON_DUOS_PUNISH_LEAVERS"] == "1",
		})

	case parser.EventTagChange:
		p.handleTagChange(e)

	case parser.EventEntityUpdate:
		p.handleEntityUpdate(e)
	}
}

const staleGameTimeout = 3 * time.Minute

// CheckStaleness marks an active game as over if no events have been received
// for staleGameTimeout. Called periodically from the daemon.
func (p *Processor) CheckStaleness() {
	phase := p.machine.Phase()
	if phase == PhaseIdle || phase == PhaseGameOver {
		return
	}
	if p.lastEventTime.IsZero() {
		return
	}
	if time.Since(p.lastEventTime) > staleGameTimeout {
		slog.Warn("game stale, forcing game over", "lastEvent", p.lastEventTime)
		p.machine.GameEnd(0, time.Now())
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
		case "BACON_DUO_PASSABLE":
			// Thin adapter: delegate to OnDuosPassableChanged for the direct Handle() path.
			val, _ := strconv.Atoi(value)
			_ = p.OnDuosPassableChanged(&action.DuosPassableChangedAction{
				ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Value:      val,
			})

		case "BACON_DUOS_PUNISH_LEAVERS":
			// Thin adapter: delegate to OnDuosPunishLeaversChanged for the direct Handle() path.
			val, _ := strconv.Atoi(value)
			_ = p.OnDuosPunishLeaversChanged(&action.DuosPunishLeaversChangedAction{
				ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Value:      val,
			})

		case "BACON_CURRENT_COMBAT_PLAYER_ID":
			combatPlayerID, _ := strconv.Atoi(value)
			_ = p.OnCombatPlayerChanged(&action.CombatPlayerChangedAction{
				ActionBase:     action.ActionBase{Entity: action.EntityID(e.EntityID)},
				CombatPlayerID: combatPlayerID,
				ControllerID:   controllerID,
				EntityName:     e.EntityName,
			})

		case "BACON_DUO_TEAMMATE_PLAYER_ID":
			if p.isLocalPlayerEntity(e) {
				pid, _ := strconv.Atoi(value)
				if pid > 0 {
					_ = p.OnDuosTeammate(&action.DuosTeammateAction{
						ActionBase:      action.ActionBase{Entity: action.EntityID(e.EntityID)},
						PartnerPlayerID: pid,
						ControllerID:    controllerID,
						EntityName:      e.EntityName,
					})
				}
			}

		case "PLAYER_ID":
			// Update entity registry with PLAYER_ID from TAG_CHANGE.
			// In duos combat, hero copies receive PLAYER_ID via TAG_CHANGE
			// after FULL_ENTITY creation. Only update existing entries to
			// avoid creating spurious registry entries.
			if e.EntityID > 0 {
				if info := p.entityProps[e.EntityID]; info != nil {
					info.PlayerID = parseInt(value)
				}
			}

		case "HEALTH":
			// Always update entity registry — tavern/shop minions receive
			// buffed stats (e.g. from BG_ShopBuff enchantments) while
			// CONTROLLER is still the tavern player. Without this, buying
			// a buffed minion would show base stats on the board.
			if e.EntityID > 0 {
				if info := p.entityProps[e.EntityID]; info != nil {
					info.Health = parseInt(value)
				}
			}
			if p.isLocalHero(e, controllerID) || p.isPartnerHero(e, controllerID) {
				_ = p.OnHeroStatChanged(&action.HeroStatChangedAction{
					ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
					Tag:          tag,
					NewValue:     parseInt(value),
					ControllerID: controllerID,
				})
			} else if e.EntityID > 0 && controllerID == p.localPlayerID {
				phase := p.machine.Phase()
				if phase == PhaseCombat {
					_ = p.OnMinionTempStatChanged(&action.MinionTempStatChangedAction{
						ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
						Stat:         "HEALTH",
						NewValue:     parseInt(value),
						ControllerID: controllerID,
					})
				} else {
					_ = p.OnMinionPermStatChanged(&action.MinionPermStatChangedAction{
						ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
						Stat:         "HEALTH",
						NewValue:     parseInt(value),
						EntityName:   e.EntityName,
						ControllerID: controllerID,
					})
				}
				p.updatePartnerCombatMinion(e.EntityID, "HEALTH", parseInt(value))
				p.updateCombatCopyPeak(e.EntityID, "HEALTH", parseInt(value))
			}

		case "ATK":
			// Always update entity registry (see HEALTH comment above).
			if e.EntityID > 0 {
				if info := p.entityProps[e.EntityID]; info != nil {
					info.Attack = parseInt(value)
				}
			}
			if e.EntityID > 0 && controllerID == p.localPlayerID {
				phase := p.machine.Phase()
				if phase == PhaseCombat {
					_ = p.OnMinionTempStatChanged(&action.MinionTempStatChangedAction{
						ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
						Stat:         "ATK",
						NewValue:     parseInt(value),
						ControllerID: controllerID,
					})
				} else {
					_ = p.OnMinionPermStatChanged(&action.MinionPermStatChangedAction{
						ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
						Stat:         "ATK",
						NewValue:     parseInt(value),
						EntityName:   e.EntityName,
						ControllerID: controllerID,
					})
				}
				p.updatePartnerCombatMinion(e.EntityID, "ATK", parseInt(value))
				p.updateCombatCopyPeak(e.EntityID, "ATK", parseInt(value))
			}

		case "PROPOSED_ATTACKER":
			// Thin adapter: delegate to OnCombatAttacker for the direct Handle() path.
			// The dispatcher path is handled via CombatAttackerAction from the builder.
			if e.EntityName == "GameEntity" {
				attackerID := parseInt(value)
				_ = p.OnCombatAttacker(&action.CombatAttackerAction{
					ActionBase:     action.ActionBase{Entity: action.EntityID(e.EntityID)},
					AttackerID:     attackerID,
					IsHeroAttacker: attackerID > 0 && p.heroEntities[attackerID],
				})
			}

		case "PROPOSED_DEFENDER":
			// Thin adapter: delegate to OnCombatDefender for the direct Handle() path.
			// The dispatcher path is handled via CombatDefenderAction from the builder.
			if e.EntityName == "GameEntity" {
				defenderID := parseInt(value)
				if defenderID > 0 {
					_ = p.OnCombatDefender(&action.CombatDefenderAction{
						ActionBase:     action.ActionBase{Entity: action.EntityID(e.EntityID)},
						DefenderID:     defenderID,
						IsHeroDefender: p.heroEntities[defenderID],
					})
				}
			}

		case "DAMAGE":
			_ = p.OnHeroStatChanged(&action.HeroStatChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Tag:          tag,
				NewValue:     parseInt(value),
				ControllerID: controllerID,
			})

		case "ARMOR":
			// Cache armor for all known hero entities so we can retroactively
			// apply it when the hero is later identified as the local hero.
			if e.EntityID > 0 && p.heroEntities[e.EntityID] {
				if info := p.entityProps[e.EntityID]; info != nil {
					info.Armor = parseInt(value)
				}
			}
			_ = p.OnHeroStatChanged(&action.HeroStatChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Tag:          tag,
				NewValue:     parseInt(value),
				ControllerID: controllerID,
			})

		case "SPELL_POWER":
			_ = p.OnHeroStatChanged(&action.HeroStatChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Tag:          tag,
				NewValue:     parseInt(value),
				ControllerID: controllerID,
			})

		case "PLAYER_TECH_LEVEL", "TAVERN_TIER":
			tier, _ := strconv.Atoi(value)
			_ = p.OnTavernTierChanged(&action.TavernTierChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Tag:          tag,
				Tier:         tier,
				ControllerID: controllerID,
				EntityName:   e.EntityName,
			})

		case "PLAYER_TRIPLES":
			val, _ := strconv.Atoi(value)
			_ = p.OnPlayerTriplesChanged(&action.PlayerTriplesChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Value:        val,
				ControllerID: controllerID,
				EntityName:   e.EntityName,
			})

		case "NUM_SPELLS_PLAYED_THIS_GAME":
			// Authoritative spell-play counter from the game engine — treats BG tavern
			// spells as MINIONs in the log, so the old ZONE=PLAY/CardType==SPELL path
			// was fundamentally unreliable.  Use the absolute value and compute deltas.
			val, _ := strconv.Atoi(value)
			if p.actionIsLocalPlayerEntity(e.EntityID, e.EntityName) && val > p.spellsPlayedTotal {
				delta := val - p.spellsPlayedTotal
				p.spellsPlayedTotal = val
				for i := 0; i < delta; i++ {
					_ = p.OnTavernSpellPlayed(&action.TavernSpellPlayedAction{
						ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
					})
				}
			}

		case "RESOURCES", "RESOURCES_USED":
			val, _ := strconv.Atoi(value)
			_ = p.OnGoldChanged(&action.GoldChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Tag:          tag,
				Value:        val,
				ControllerID: e.PlayerID,
				EntityName:   e.EntityName,
			})

		case "PLAYER_LEADERBOARD_PLACE":
			if pl, err := strconv.Atoi(value); err == nil {
				_ = p.OnPlacementChanged(&action.PlacementChangedAction{
					ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
					Value:        pl,
					ControllerID: controllerID,
					EntityName:   e.EntityName,
				})
			}

		case "ZONE":
			if e.EntityID > 0 {
				info := p.entityProps[e.EntityID]
				prevZone := ""
				if info != nil {
					prevZone = info.Zone
					info.Zone = value
					p.handleOverconfidenceZone(info.CardID, value, prevZone, controllerID)
				}
				if value == "PLAY" && p.machine.Phase() != PhaseGameOver {
					if info != nil && info.CardType == "SPELL" {
						// SPELLS_CAST (recruit phase) is now tracked via NUM_SPELLS_PLAYED_THIS_GAME
						// player tag — BG tavern spells appear as CARDTYPE=MINION in the log, so
						// the ZONE=PLAY / CardType==SPELL path only fires for system / combat spells.
						// Keep the combat path for SPELLCRAFT_CAST (rally-triggered combat spells
						// that do have CARDTYPE=SPELL in the log).
						if prevZone == "HAND" && controllerID == p.localPlayerID &&
							p.machine.Phase() == PhaseCombat {
							_ = p.OnCombatTavernSpell(&action.CombatTavernSpellAction{
								ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
							})
						}
						// SPELL entities must never route to OnMinionBought (they are not minions).
					} else {
						_ = p.OnMinionBought(&action.MinionBoughtAction{
							ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
							ControllerID: controllerID,
						})
					}
				}
			}
			if e.EntityID > 0 && value != "PLAY" && p.machine.Phase() != PhaseGameOver {
				_ = p.OnMinionSold(&action.MinionSoldAction{
					ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
				})
			}

		case "TURN":
			// Player-specific TURN tag (not GameEntity).
			// This gives us the actual BG turn number the player sees.
			turn, _ := strconv.Atoi(value)
			_ = p.OnPlayerBGTurnChanged(&action.PlayerBGTurnChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Value:        turn,
				ControllerID: controllerID,
				EntityName:   e.EntityName,
			})

		case "HERO_ENTITY":
			heroID, _ := strconv.Atoi(value)
			if heroID > 0 {
				_ = p.OnHeroEntityAssigned(&action.HeroEntityAssignedAction{
					ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
					HeroEntityID: heroID,
					ControllerID: controllerID,
					EntityName:   e.EntityName,
				})
			}

		case "BACON_BLOODGEMBUFFATKVALUE", "BACON_BLOODGEMBUFFHEALTHVALUE",
			"BACON_ELEMENTAL_BUFFATKVALUE", "BACON_ELEMENTAL_BUFFHEALTHVALUE",
			"TAVERN_SPELL_ATTACK_INCREASE", "TAVERN_SPELL_HEALTH_INCREASE":
			// Only accept player entities and heroes — not enchantments like
			// Bacon_TagTransferPlayerE which mirror player tags with stale values
			// and would overwrite the real buff source counters.
			if p.isPlayerOrHeroEntity(e, controllerID) {
				p.updateBuffSourceFromPlayerTag(tag, value)
			}

		case "BACON_FREE_REFRESH_COUNT":
			raw, _ := strconv.Atoi(value)
			_ = p.OnEconomyChanged(&action.EconomyChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Tag:          tag,
				Value:        raw,
				ControllerID: controllerID,
				EntityName:   e.EntityName,
			})

		case "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN":
			// Thin adapter: delegate to OnEconomyChanged (recruit) or OnCombatEconomyEffect
			// (combat) for the direct Handle() path. The builder emits the phase-appropriate
			// action; Handle() mirrors that logic here.
			raw, _ := strconv.Atoi(value)
			if raw < 0 {
				raw = 0
			}
			if p.machine.Phase() == PhaseCombat {
				_ = p.OnCombatEconomyEffect(&action.CombatEconomyEffectAction{
					ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
					Tag:          tag,
					Value:        raw,
					ControllerID: e.PlayerID,
				})
			} else {
				_ = p.OnEconomyChanged(&action.EconomyChangedAction{
					ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
					Tag:          tag,
					Value:        raw,
					ControllerID: e.PlayerID,
					EntityName:   e.EntityName,
				})
			}

		case "TAG_SCRIPT_DATA_NUM_1", "TAG_SCRIPT_DATA_NUM_2":
			if e.EntityID > 0 {
				p.updateEnchantmentScriptData(e.EntityID, tag, value)
				ctrl := p.entityController[e.EntityID]
				if ctrl == p.localPlayerID || p.isLocalDntTarget(e.EntityID) {
					_ = p.OnDntEnchantment(&action.DntEnchantmentAction{
						ActionBase: action.ActionBase{Entity: action.EntityID(e.EntityID)},
						Tag:        tag,
						Value:      parseInt(value),
					})
				}
			}

		case "3809":
			raw, _ := strconv.Atoi(value)
			_ = p.OnSpellcraftChanged(&action.SpellcraftChangedAction{
				ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
				Value:        raw,
				ControllerID: controllerID,
				EntityName:   e.EntityName,
			})

		case "CONTROLLER":
			if e.EntityID > 0 {
				ctrl, _ := strconv.Atoi(value)
				_ = p.OnEntityControllerChanged(&action.EntityControllerChangedAction{
					ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
					ControllerID: ctrl,
				})
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

	// Track entity IDs during combat phase for retroactive partner board scan.
	if p.combatPhaseActive && p.isDuos {
		p.combatPhaseEntityIDs = append(p.combatPhaseEntityIDs, e.EntityID)
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
	if race, ok := e.Tags["CARDRACE"]; ok {
		info.Race = race
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
	if zp, ok := e.Tags["ZONE_POSITION"]; ok {
		info.ZonePosition = parseInt(zp)
	}
	if pid, ok := e.Tags["PLAYER_ID"]; ok {
		info.PlayerID = parseInt(pid)
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

	// Track partner combat copies during partner's combat phase.
	// In duos, the partner's combat copies have CONTROLLER=localPlayerID,
	// while the opponent's copies have CONTROLLER=botID. The partner hero
	// copy receives PLAYER_ID=partnerPlayerID via a subsequent TAG_CHANGE.
	// Only collect minions with ZONE_POSITION > 0 (initial board setup),
	// not mid-combat spawns (deathrattles, etc.) which lack a zone position.
	zonePos := parseInt(e.Tags["ZONE_POSITION"])
	if p.partnerCombatActive && !p.partnerBoardSetupDone &&
		controllerID > 0 && controllerID == p.localPlayerID {
		if cardType == "HERO" {
			// Partner hero copy — CONTROLLER matches localPlayerID during combat.
			p.partnerCombatHeroCtrl = controllerID
		} else if cardType == "MINION" && zonePos > 0 &&
			info.Attack > 0 && info.Health > 0 && info.Zone == "PLAY" {
			mn := MinionState{
				EntityID:   e.EntityID,
				CardID:     info.CardID,
				Name:       info.Name,
				Attack:     info.Attack,
				Health:     info.Health,
				MinionType: info.Race,
			}
			if (mn.Name == "" || isBareNumber(mn.Name)) && mn.CardID != "" {
				mn.Name = CardName(mn.CardID)
			}
			p.partnerCombatMinions = append(p.partnerCombatMinions, mn)
		}
	}
	// Collect opponent board during partner's combat: minions with a controller that
	// is neither the local player nor zero (i.e. some other bot controller).
	if p.partnerCombatActive && !p.partnerBoardSetupDone &&
		cardType == "MINION" && zonePos > 0 &&
		info.Attack > 0 && info.Health > 0 && info.Zone == "PLAY" &&
		controllerID > 0 && controllerID != p.localPlayerID {
		mn := MinionState{
			EntityID:   e.EntityID,
			CardID:     info.CardID,
			Name:       info.Name,
			Attack:     info.Attack,
			Health:     info.Health,
			MinionType: info.Race,
		}
		if (mn.Name == "" || isBareNumber(mn.Name)) && mn.CardID != "" {
			mn.Name = CardName(mn.CardID)
		}
		p.opponentCombatMinions = append(p.opponentCombatMinions, mn)
	}

	// Dispatch to visitor method for machine mutations.
	// Everything above this line is pre-visitor work that is intentionally NOT delegated
	// to visitor methods:
	//   - Entity registry writes (entityProps, entityController, heroEntities): raw property
	//     bags that must be populated before any visitor can query them via cardInfoFromProps
	//     or resolveController. Visitors read this registry; they must not write to it.
	//   - Tribe detection (trackTribesFromEntity): scans raw BACON_SUBSET tags which are
	//     only present in the original FULL_ENTITY/SHOW_ENTITY block, not in TAG_CHANGE events.
	//   - Partner combat tracking (partnerCombatActive, partnerCombatMinions): irreducible
	//     duos-specific state that depends on cross-cutting fields (controllerID, cardType,
	//     zonePos, info) already computed above, and has no natural visitor home.
	//   - Combat-phase entity ID accumulation (combatPhaseEntityIDs): bookkeeping for
	//     retroactive partner board resolution; likewise cross-cutting.
	switch cardType {
	case "BATTLEGROUND_ANOMALY":
		_ = p.OnAnomalyRegistered(&action.AnomalyRegisteredAction{
			ActionBase: action.ActionBase{
				Entity: action.EntityID(e.EntityID),
				Card:   p.cardInfoFromProps(e.EntityID),
			},
		})
	case "ENCHANTMENT":
		_ = p.OnEnchantmentRegistered(&action.EnchantmentRegisteredAction{
			ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
			ControllerID: controllerID,
		})
	case "HERO":
		_ = p.OnHeroRegistered(&action.HeroRegisteredAction{
			ActionBase: action.ActionBase{
				Entity: action.EntityID(e.EntityID),
				Card:   p.cardInfoFromProps(e.EntityID),
			},
			ControllerID: controllerID,
			PlayerID:     info.PlayerID,
			Tags:         e.Tags,
		})
	default:
		_ = p.OnMinionRegistered(&action.MinionRegisteredAction{
			ActionBase:   action.ActionBase{Entity: action.EntityID(e.EntityID)},
			ControllerID: controllerID,
			Zone:         info.Zone,
			ZonePosition: info.ZonePosition,
		})
	}
}

// cardInfoFromProps constructs a card.CardInfo from the entity registry.
// Returns nil when entityID is unknown or has no CardID.
func (p *Processor) cardInfoFromProps(entityID int) *card.CardInfo {
	info := p.entityProps[entityID]
	if info == nil || info.CardID == "" {
		return nil
	}
	return &card.CardInfo{ID: info.CardID, Name: CardName(info.CardID)}
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

// updatePartnerCombatMinion updates a partner combat minion's stat if it was
// collected before the board setup cutoff. Combat copies arrive via FULL_ENTITY
// with base stats, then receive buffed stats via TAG_CHANGE before combat starts.
// After PROPOSED_ATTACKER fires (partnerBoardSetupDone), stat changes are combat
// damage or post-combat resets — ignore those.
func (p *Processor) updatePartnerCombatMinion(entityID int, stat string, val int) {
	if !p.partnerCombatActive || p.partnerBoardSetupDone {
		return
	}
	for i := range p.partnerCombatMinions {
		if p.partnerCombatMinions[i].EntityID == entityID {
			switch stat {
			case "ATK":
				p.partnerCombatMinions[i].Attack = val
			case "HEALTH":
				p.partnerCombatMinions[i].Health = val
			}
			return
		}
	}
	// Also update opponent combat minions — they receive buffed stats via TAG_CHANGE
	// before PROPOSED_ATTACKER fires, just like partner minions.
	for i := range p.opponentCombatMinions {
		if p.opponentCombatMinions[i].EntityID == entityID {
			switch stat {
			case "ATK":
				p.opponentCombatMinions[i].Attack = val
			case "HEALTH":
				p.opponentCombatMinions[i].Health = val
			}
			return
		}
	}
}

// finalizePartnerCombat snapshots the collected partner combat minions and opponent minions.
func (p *Processor) finalizePartnerCombat() {
	p.partnerCombatActive = false
	if len(p.partnerCombatMinions) > 0 {
		// Sort by ZONE_POSITION (ascending) so positions 1-7 are kept.
		sort.Slice(p.partnerCombatMinions, func(i, j int) bool {
			posI, posJ := 0, 0
			if info := p.entityProps[p.partnerCombatMinions[i].EntityID]; info != nil {
				posI = info.ZonePosition
			}
			if info := p.entityProps[p.partnerCombatMinions[j].EntityID]; info != nil {
				posJ = info.ZonePosition
			}
			return posI < posJ
		})
		if len(p.partnerCombatMinions) > 7 {
			p.partnerCombatMinions = p.partnerCombatMinions[:7]
		}
		turn := p.machine.currentTurn()
		p.machine.SetPartnerBoard(p.partnerCombatMinions, turn)
		slog.Info("partner board captured from combat", "minions", len(p.partnerCombatMinions), "turn", turn)
	}
	p.partnerCombatMinions = nil

	// Snapshot the opponent board collected during partner's combat.
	if len(p.opponentCombatMinions) > 0 {
		sort.Slice(p.opponentCombatMinions, func(i, j int) bool {
			posI, posJ := 0, 0
			if info := p.entityProps[p.opponentCombatMinions[i].EntityID]; info != nil {
				posI = info.ZonePosition
			}
			if info := p.entityProps[p.opponentCombatMinions[j].EntityID]; info != nil {
				posJ = info.ZonePosition
			}
			return posI < posJ
		})
		p.machine.SetOpponentBoard(p.opponentCombatMinions)
		slog.Info("opponent board captured from partner combat", "minions", len(p.opponentCombatMinions))
	}
	p.opponentCombatMinions = nil

	p.partnerCombatHeroCtrl = 0
	p.partnerBoardSetupDone = false
}

// collectPartnerCombatRetro scans recently created combat entities for partner
// combat copies that were created before BACON_CURRENT_COMBAT_PLAYER_ID fired.
// In duos, the partner's hero copy (CONTROLLER=localPlayerID, PLAYER_ID=partnerPlayerID)
// and its minions are sometimes emitted before the combat flag.
// Only entities tracked in combatPhaseEntityIDs are considered, avoiding confusion
// with the local player's real board minions.
func (p *Processor) collectPartnerCombatRetro() {
	if p.partnerPlayerID == 0 || p.localPlayerID == 0 {
		return
	}
	// Find the partner's hero copy among combat-phase entities.
	var heroCtrl int
	for _, eid := range p.combatPhaseEntityIDs {
		info := p.entityProps[eid]
		if info == nil {
			continue
		}
		if info.CardType == "HERO" && info.PlayerID == p.partnerPlayerID && info.Zone == "PLAY" {
			ctrl := p.entityController[eid]
			if ctrl == p.localPlayerID {
				heroCtrl = ctrl
				p.partnerCombatHeroCtrl = ctrl
				slog.Debug("partner hero found retroactively", "entityID", eid, "ctrl", ctrl)
				break
			}
		}
	}
	if heroCtrl == 0 {
		return
	}
	// Collect minions with the same controller from combat-phase entities.
	// Only include minions with ZonePosition > 0 (initial board setup minions).
	// Also collect opponent minions: any PLAY minion whose controller is not localPlayerID.
	for _, eid := range p.combatPhaseEntityIDs {
		info := p.entityProps[eid]
		if info == nil {
			continue
		}
		ctrl := p.entityController[eid]
		if info.CardType == "MINION" && info.Zone == "PLAY" &&
			info.ZonePosition > 0 &&
			info.Attack > 0 && info.Health > 0 {
			mn := MinionState{
				EntityID:   eid,
				CardID:     info.CardID,
				Name:       info.Name,
				Attack:     info.Attack,
				Health:     info.Health,
				MinionType: info.Race,
			}
			if (mn.Name == "" || isBareNumber(mn.Name)) && mn.CardID != "" {
				mn.Name = CardName(mn.CardID)
			}
			if ctrl == heroCtrl {
				p.partnerCombatMinions = append(p.partnerCombatMinions, mn)
			} else if ctrl > 0 && ctrl != p.localPlayerID {
				// Opponent minion: controlled by a different bot (not local player).
				p.opponentCombatMinions = append(p.opponentCombatMinions, mn)
			}
		}
	}
	if len(p.partnerCombatMinions) > 0 {
		slog.Debug("partner minions collected retroactively", "count", len(p.partnerCombatMinions))
	}
	if len(p.opponentCombatMinions) > 0 {
		slog.Debug("opponent minions collected retroactively", "count", len(p.opponentCombatMinions))
	}
}

// isLocalDntTarget returns true if entityID is a Dnt enchantment attached to a
// local player entity — i.e., the local player entity or the local hero.
//
// In Duos, local player Dnt enchantments always have CONTROLLER == localPlayerID
// and ATTACHED == local player entity. Opponent combat copy Dnt enchantments have
// CONTROLLER == botPlayerID and ATTACHED == bot entity. We must NOT treat
// bot-attached enchantments as local — that was the source of buff leakage.
func (p *Processor) isLocalDntTarget(entityID int) bool {
	info := p.entityProps[entityID]
	if info == nil {
		return false
	}
	if info.AttachedTo > 0 {
		if info.AttachedTo == p.localHeroID {
			return true
		}
		if pid, ok := p.playerEntityIDs[info.AttachedTo]; ok {
			if pid == p.localPlayerID {
				return true
			}
		}
	}
	return false
}

// resolvePartner retroactively identifies the partner from a PlayerID discovered
// via BACON_CURRENT_COMBAT_PLAYER_ID. Scans heroEntities for a hero with matching
// PLAYER_ID and sets partner state accordingly.
func (p *Processor) resolvePartner(playerID int) {
	p.partnerPlayerID = playerID
	slog.Info("deferred partner resolution", "partnerPlayerID", playerID)

	// Scan hero entities for one with matching PLAYER_ID.
	for heroID := range p.heroEntities {
		info := p.entityProps[heroID]
		if info == nil {
			continue
		}
		if info.PlayerID == playerID {
			p.partnerHeroID = heroID
			if info.CardID != "" && !strings.HasPrefix(info.CardID, "TB_BaconShop_HERO_PH") {
				p.machine.UpdatePartnerHeroCardID(info.CardID)
			}
			if info.Name != "" {
				p.partnerPlayerName = info.Name
				p.machine.UpdatePartnerName(info.Name)
			}
			if info.Health > 0 {
				p.machine.UpdatePartnerTag("HEALTH", strconv.Itoa(info.Health))
			}
			if info.Armor > 0 {
				p.machine.UpdatePartnerTag("ARMOR", strconv.Itoa(info.Armor))
			}
			slog.Info("partner hero resolved retroactively",
				"heroID", heroID, "cardID", info.CardID, "name", info.Name)
			break
		}
	}
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

// updateCombatCopyPeak updates the peak ATK/HEALTH for a tracked combat copy
// and syncs the board snapshot when higher stats are observed. During combat
// setup, copies briefly show the real (enchantment-inclusive) stats before
// being stripped to base. Tracking the peak values recovers the recruit stats
// for minions like Wrath Weaver whose buffs never appear as TAG_CHANGE on the
// recruit entity.
func (p *Processor) updateCombatCopyPeak(entityID int, stat string, val int) {
	cc := p.combatCopies[entityID]
	if cc == nil {
		return
	}
	updated := false
	switch stat {
	case "ATK":
		if val > cc.ATK {
			cc.ATK = val
			updated = true
		}
	case "HEALTH":
		if val > cc.Health {
			cc.Health = val
			updated = true
		}
	}
	if updated {
		p.machine.UpdateSnapshotFromCombatCopy(entityID, cc.CardID, cc.ATK, cc.Health)
	}
}

// isPlayerOrHeroEntity returns true if the event belongs to the local player
// entity or hero, but NOT enchantments. Used for buff source / ability counter
// tags that should only be accepted from player entities and heroes.
// Enchantments like Bacon_TagTransferPlayerE mirror player tags (e.g.
// TAVERN_SPELL_ATTACK_INCREASE) with stale values — accepting them would
// overwrite the real counters.
func (p *Processor) isPlayerOrHeroEntity(e parser.GameEvent, controllerID int) bool {
	// Reject known enchantment entities.
	if e.EntityID > 0 {
		if info := p.entityProps[e.EntityID]; info != nil && info.CardType == "ENCHANTMENT" {
			return false
		}
	}
	return p.isLocalPlayerEntity(e) || p.isLocalHero(e, controllerID)
}

// updateMinionStatByID is the core stat-update path called by the Phase 5 visitor
// (OnMinionPermStatChanged).
func (p *Processor) updateMinionStatByID(entityID int, entityName, stat string, newVal int) {
	if entityID <= 0 {
		return
	}

	// Update entity registry.
	info := p.entityProps[entityID]
	if info == nil {
		info = &entityInfo{}
		p.entityProps[entityID] = info
	}
	if entityName != "" && (info.Name == "" || isBareNumber(info.Name)) {
		info.Name = cleanEntityName(entityName)
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
	if p.heroEntities[entityID] {
		return
	}

	// Update board minion stats if it's on the board.
	onBoard := p.machine.UpdateMinionStat(entityID, stat, newVal)
	phase := p.machine.Phase()
	if onBoard && phase == PhaseRecruit {
		p.machine.UpdateBoardSnapshot()
	}

	// Buffer stat changes during recruit phase for board-wide buff detection.
	// Skip during combat (simulation noise).
	if oldVal > 0 && newVal != oldVal && (onBoard || info.Zone == "PLAY") && phase != PhaseCombat {
		delta := newVal - oldVal
		name := info.Name
		if name == "" {
			name = info.CardID
		}
		p.pendingStatChanges = append(p.pendingStatChanges, pendingStatChange{
			entityID: entityID,
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
	if info.CardType != "MINION" {
		return
	}
	if p.machine.Phase() == PhaseGameOver {
		return
	}
	mn := MinionState{
		EntityID:   entityID,
		CardID:     info.CardID,
		Name:       info.Name,
		Attack:     info.Attack,
		Health:     info.Health,
		MinionType: info.Race,
	}
	if (mn.Name == "" || isBareNumber(mn.Name)) && mn.CardID != "" {
		mn.Name = CardName(mn.CardID)
	}
	p.machine.UpsertMinion(mn)
	if p.machine.Phase() == PhaseRecruit {
		p.machine.UpdateBoardSnapshot()
	}
}

// pruneStaleEntities removes dead entities (REMOVEDFROMGAME, GRAVEYARD) from
// the entity registry. Called on recruit phase transitions to prevent unbounded
// growth from combat simulation entities.
func (p *Processor) pruneStaleEntities() {
	for id, info := range p.entityProps {
		if info.Zone == "REMOVEDFROMGAME" || info.Zone == "GRAVEYARD" {
			delete(p.entityProps, id)
			delete(p.entityController, id)
			delete(p.heroEntities, id)
		}
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

	// Determine if this enchantment is relevant to the local player.
	targetCtrl := p.entityController[info.AttachedTo]
	enchCtrl := p.entityController[e.EntityID]

	isRelevant := false
	isPartnerEnch := p.isDuos && enchCtrl == p.partnerPlayerID
	if targetCtrl == p.localPlayerID {
		// Enchantment on a local minion — always track.
		isRelevant = true
	} else if p.isLocalDntTarget(e.EntityID) {
		// Dnt enchantment attached to local/bot player entity — track.
		isRelevant = true
	} else if enchCtrl == p.localPlayerID && category != CatGeneral {
		// Enchantment owned by local player on non-local target (e.g., aura effects).
		// Only track if it has a specific (non-general) category.
		isRelevant = true
	} else if isPartnerEnch && category != CatGeneral {
		// Partner enchantment with a specific category — track for partner buff display.
		isRelevant = true
	}
	if !isRelevant {
		return
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
	// Allow local player, local Dnt targets, and partner through — handleDntTagChange
	// routes to the correct tracker internally.
	if info.ScriptData1 != 0 || info.ScriptData2 != 0 {
		isLocalOrDnt := enchCtrl == p.localPlayerID || (enchCtrl != 0 && p.isLocalDntTarget(e.EntityID))
		if isLocalOrDnt || isPartnerEnch {
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
	// Only process enchantments controlled by local player, partner, or attached to local entities.
	// Reject controller=0 (unknown) to prevent opponent buff leakage.
	ctrl := p.entityController[entityID]
	isLocal := ctrl == p.localPlayerID || p.isLocalDntTarget(entityID)
	isPartner := p.isDuos && ctrl == p.partnerPlayerID
	if ctrl == 0 || (!isLocal && !isPartner) {
		return
	}

	bt := &p.localBuffs
	setBS := p.machine.SetBuffSource
	if isPartner {
		bt = &p.partnerBuffs
		setBS = p.machine.SetPartnerBuffSource
	}

	cardID := info.CardID
	isSD1 := tag == "TAG_SCRIPT_DATA_NUM_1"

	switch cardID {
	case "BG_ShopBuff":
		p.handleGenericShopBuffDnt(bt, setBS, isSD1, value, CatShopBuff)
	case "BG_ShopBuff_Elemental":
		p.handleShopBuffDnt(bt, setBS, entityID, isSD1, value)
	case "BG30_MagicItem_544pe":
		p.handleNomiStickerDnt(bt, setBS, entityID, isSD1, value)
	case "BG34_855pe":
		p.handleNomiAllDnt(bt, setBS, entityID, isSD1, value)
	case "BG31_808pe":
		p.handleAbsoluteDnt(bt, setBS, CatBeetle, isSD1, value, 1, 1)
	case "BG34_854pe":
		p.handleAbsoluteDnt(bt, setBS, CatRightmost, isSD1, value, 0, 0)
	case "BG34_402pe":
		p.handleAbsoluteDnt(bt, setBS, CatWhelp, isSD1, value, 0, 0)
	case "BG25_011pe":
		if isSD1 {
			if p.isDuos && !isPartner {
				p.handleAbsoluteDntDuos(CatUndead, true, value, 0, 0)
			} else {
				setBS(CatUndead, value, 0)
			}
		}
	case "BG34_170e":
		p.handleAbsoluteDnt(bt, setBS, CatVolumizer, isSD1, value, 0, 0)
	case "BG34_689e2":
		p.handleAbsoluteDnt(bt, setBS, CatBloodgemBarrage, isSD1, value, 0, 0)
	default:
		if cardID != "" && value != 0 {
			slog.Debug("untracked Dnt enchantment", "cardID", cardID, "tag", tag, "value", value, "entityID", entityID)
		}
	}
}

// handleAbsoluteDnt sets a buff source from an absolute Dnt value plus base offset.
func (p *Processor) handleAbsoluteDnt(bt *buffTracker, setBS func(string, int, int), category string, isSD1 bool, value, baseAtk, baseHp int) {
	// In duos, local-controlled Dnt values are team totals that need splitting
	// by combat phase. Explicitly partner-controlled Dnt goes directly to partner.
	if p.isDuos && bt == &p.localBuffs {
		p.handleAbsoluteDntDuos(category, isSD1, value, baseAtk, baseHp)
		return
	}
	state := bt.buffSourceState[category]
	if isSD1 {
		state[0] = baseAtk + value
	} else {
		state[1] = baseHp + value
	}
	bt.buffSourceState[category] = state
	setBS(category, state[0], state[1])
}

// handleAbsoluteDntDuos splits an absolute Dnt value into local vs partner
// contributions in duos. Uses partnerCombatActive to attribute deltas:
// increments during partner combat go to partner, all others go to local.
func (p *Processor) handleAbsoluteDntDuos(category string, isSD1 bool, value, baseAtk, baseHp int) {
	idx := 0
	if !isSD1 {
		idx = 1
	}

	prev := p.dntTeamTotal[category]
	delta := value - prev[idx]
	if delta < 0 {
		// New combat copy with lower value than accumulated — treat as reset.
		delta = 0
	}

	// Update team total.
	prev[idx] = value
	p.dntTeamTotal[category] = prev

	// Attribute delta to partner if their combat is active.
	if p.partnerCombatActive && delta > 0 {
		accum := p.dntPartnerAccum[category]
		accum[idx] += delta
		p.dntPartnerAccum[category] = accum
	}

	// Compute display values.
	teamTotal := p.dntTeamTotal[category]
	partnerAccum := p.dntPartnerAccum[category]

	localAtk := baseAtk + teamTotal[0] - partnerAccum[0]
	localHp := baseHp + teamTotal[1] - partnerAccum[1]

	p.localBuffs.buffSourceState[category] = [2]int{localAtk, localHp}
	p.machine.SetBuffSource(category, localAtk, localHp)

	if partnerAccum[0] > 0 || partnerAccum[1] > 0 {
		p.partnerBuffs.buffSourceState[category] = [2]int{partnerAccum[0], partnerAccum[1]}
		p.machine.SetPartnerBuffSource(category, partnerAccum[0], partnerAccum[1])
	}
}

// handleGenericShopBuffDnt handles BG_ShopBuff (generic shop buff) with differential accumulation.
// BG_ShopBuff is a cumulative Dnt: a fresh entity is spawned each reconnect carrying the running
// total. Tracking by entity ID would restart the delta from zero each time, summing all snapshots
// instead of computing increments — so we track the last absolute value per category globally.
func (p *Processor) handleGenericShopBuffDnt(bt *buffTracker, setBS func(string, int, int), isSD1 bool, value int, category string) {
	global := bt.shopBuffGlobal[category]
	var delta int
	if isSD1 {
		delta = value - global[0]
		global[0] = value
	} else {
		delta = value - global[1]
		global[1] = value
	}
	bt.shopBuffGlobal[category] = global

	state := bt.buffSourceState[category]
	if isSD1 {
		state[0] += delta
	} else {
		state[1] += delta
	}
	bt.buffSourceState[category] = state
	setBS(category, state[0], state[1])
}

// handleShopBuffDnt handles BG_ShopBuff_Elemental with differential accumulation.
func (p *Processor) handleShopBuffDnt(bt *buffTracker, setBS func(string, int, int), entityID int, isSD1 bool, value int) {
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
	setBS(CatNomi, bt.nomiCounter[0], bt.nomiCounter[1])
}

// handleNomiAllDnt handles BG34_855pe (Timewarped Nomi / Kitchen Dream) with differential
// accumulation. Same pattern as regular Nomi but tracked under CatNomiAll.
func (p *Processor) handleNomiAllDnt(bt *buffTracker, setBS func(string, int, int), entityID int, isSD1 bool, value int) {
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
	setBS(CatNomiAll, bt.nomiAllCounter[0], bt.nomiAllCounter[1])
}

// handleNomiStickerDnt handles BG30_MagicItem_544pe where SD1 applies to BOTH atk and hp.
func (p *Processor) handleNomiStickerDnt(bt *buffTracker, setBS func(string, int, int), entityID int, isSD1 bool, value int) {
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
	setBS(CatNomi, bt.nomiCounter[0], bt.nomiCounter[1])
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
