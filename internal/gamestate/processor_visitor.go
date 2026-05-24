package gamestate

// processor_visitor.go implements the three visitor interfaces on Processor.
//
// Migration status:
//   Phase 1 — all no-ops (Handle() does all work)
//   Phase 2 — lifecycle events migrated: OnGameStart, OnPlayerDef, OnPlayerName,
//              OnTurnTransition, OnGameEnd

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"battlestream.fixates.io/internal/gamestate/action"
)

// ── Visitor adapter accessors ─────────────────────────────────────────────────

func (p *Processor) AsRecruitVisitor() action.RecruitVisitor    { return p }
func (p *Processor) AsCombatVisitor() action.CombatVisitor      { return p }
func (p *Processor) AsTransitionVisitor() action.TransitionVisitor { return p }

// ── TransitionVisitor ─────────────────────────────────────────────────────────

// OnGameStart handles EventGameStart: resets all processor state, stashes any
// in-progress game for reconnect detection, and starts a new game on the Machine.
func (p *Processor) OnGameStart(a *action.GameStartAction) error {
	p.flushPendingStatChanges()
	p.isReconnect = false

	// Stash current game state before reset — restored if the next
	// EventGameEntityTags confirms this is a reconnect (STATE=RUNNING, TURN>1).
	if phase := p.machine.Phase(); phase != PhaseIdle && phase != PhaseGameOver {
		s := p.machine.State()
		turnSnaps, prevBS, prevAC, prevMC := p.machine.ReconnectStashData()
		p.reconnectStash = &reconnectStash{
			gameID:             s.GameID,
			startTime:          s.StartTime,
			turn:               s.Turn,
			tavernTier:         s.TavernTier,
			isDuos:             s.IsDuos,
			partnerPlayerID:    p.partnerPlayerID,
			partnerPlayerName:  p.partnerPlayerName,
			heroCardID:         s.Player.HeroCardID,
			partnerHeroCardID:  "",
			turnSnapshots:      turnSnaps,
			buffSources:        append([]BuffSource(nil), s.BuffSources...),
			abilityCounters:    append([]AbilityCounter(nil), s.AbilityCounters...),
			partnerBuffSources: append([]BuffSource(nil), s.PartnerBuffSources...),
			partnerAbilityCtrs: append([]AbilityCounter(nil), s.PartnerAbilityCounters...),
			modifications:      append([]StatMod(nil), s.Modifications...),
			prevBuffSources:    prevBS,
			prevAbilityCtrs:    prevAC,
			prevModCount:       prevMC,
			anomalyCardID:      s.AnomalyCardID,
			anomalyName:        s.AnomalyName,
			anomalyDescription: s.AnomalyDescription,
			availableTribes:    append([]string(nil), s.AvailableTribes...),
		}
		if s.Partner != nil {
			p.reconnectStash.partnerHeroCardID = s.Partner.HeroCardID
		}
	} else {
		p.reconnectStash = nil
	}

	p.resetProcessorState()

	// Derive stable game ID from CREATE_GAME timestamp; fall back to sequence.
	gameID := a.GameID
	if gameID == "" {
		p.gameSeq++
		gameID = fmt.Sprintf("game-%d", p.gameSeq)
	} else {
		p.gameSeq++
	}

	ts := a.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	p.machine.GameStart(gameID, ts)
	return nil
}

// OnTurnTransition handles EventTurnStart: flushes pending stat changes, advances
// phase on the Machine, and prunes stale combat entities on recruit transitions.
func (p *Processor) OnTurnTransition(a *action.TurnTransitionAction) error {
	p.flushPendingStatChanges()
	turn := a.GameEntityTurn
	if turn == 0 {
		return nil
	}
	p.machine.SetGameEntityTurn(turn)
	// On recruit-phase transition (odd GameEntity turn), prune dead combat copies.
	if turn%2 == 1 {
		p.pruneStaleEntities()
		p.combatCopies = nil
	}
	return nil
}

// OnGameEnd handles EventGameEnd: records the final combat result and ends the game.
func (p *Processor) OnGameEnd(a *action.GameEndAction) error {
	p.flushPendingStatChanges()
	// Record the final combat result — the TURN-based streak update won't fire
	// after the last combat round.
	if p.localCombatResult > 0 {
		p.machine.RecordRoundWin()
	} else if p.localCombatResult < 0 {
		p.machine.RecordRoundLoss()
	}
	p.localCombatResult = 0

	placement := a.Placement
	if placement == 0 {
		placement = p.pendingPlacement
	}
	p.pendingPlacement = 0

	ts := a.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	p.machine.GameEnd(placement, ts)
	return nil
}

// OnReconnect handles confirmed mid-game reconnects: restores game state from stash.
// (Reconnect detection still happens in Handle()/EventGameEntityTags during Phase 2;
// this method is called once the stash has been populated and checked.)
func (p *Processor) OnReconnect(a *action.ReconnectAction) error {
	// Reconnect restoration is currently handled inline in Handle() case
	// EventGameEntityTags. This method is a stub until Phase 2.5.
	return nil
}

// OnPlayerDef identifies the local and partner players from the CREATE_GAME block.
func (p *Processor) OnPlayerDef(a *action.PlayerDefAction) error {
	isReal := a.AccountHi != 0
	entityID := int(a.Entity)

	if entityID > 0 {
		p.playerEntityIDs[entityID] = a.PlayerID
	}
	if isReal {
		p.realPlayerIDs[a.PlayerID] = entityID
	}

	if isReal && p.localPlayerID == 0 {
		// First real player — this is the local player.
		p.localPlayerID = a.PlayerID
		slog.Info("identified local player", "playerID", p.localPlayerID, "entityID", entityID)

		if a.HeroEntityID > 0 {
			p.localHeroID = a.HeroEntityID
			slog.Info("local hero entity set (tentative) from player def", "heroID", p.localHeroID)
		}

		if a.DuoTeammateID > 0 {
			p.isDuos = true
			p.duosFromTeammate = true
			p.partnerPlayerID = a.DuoTeammateID
			p.machine.SetDuosMode(true)
			slog.Info("Duos detected from player def", "partnerPlayerID", a.DuoTeammateID)
		}

		// Capture initial player-entity tags (critical for reconnect state restore).
		if a.InitialTurn > 0 {
			p.machine.SetTurn(a.InitialTurn)
		}
		if a.Resources > 0 {
			p.machine.UpdateGold("RESOURCES", a.Resources)
		}
		if a.ResourcesUsed > 0 {
			p.machine.UpdateGold("RESOURCES_USED", a.ResourcesUsed)
		}

	} else if isReal && p.localPlayerID != 0 && a.PlayerID != p.localPlayerID {
		// Second real player in Duos — check if this is the partner.
		if p.isDuos && a.PlayerID == p.partnerPlayerID {
			slog.Info("identified partner player from def", "playerID", a.PlayerID, "entityID", entityID)
			if a.HeroEntityID > 0 {
				p.partnerHeroID = a.HeroEntityID
				slog.Info("partner hero entity set (tentative) from player def", "heroID", p.partnerHeroID)
			}
		}
	}
	return nil
}

// OnPlayerName maps a PlayerID to a display name (from DebugPrintGame lines).
func (p *Processor) OnPlayerName(a *action.PlayerNameAction) error {
	if a.PlayerID == p.localPlayerID {
		p.localPlayerName = a.BattleTag
		p.machine.UpdatePlayerName(a.BattleTag)
		slog.Info("local player name", "name", a.BattleTag)
	} else if p.isDuos && a.PlayerID != p.localPlayerID && p.partnerPlayerName == "" {
		p.partnerPlayerName = a.BattleTag
		p.machine.UpdatePartnerName(a.BattleTag)
		slog.Info("partner player name", "name", a.BattleTag)
	}
	return nil
}

// ── RecruitVisitor (no-ops until Phase 3+) ───────────────────────────────────

func (p *Processor) OnMinionBought(_ *action.MinionBoughtAction) error            { return nil }
func (p *Processor) OnMinionSold(_ *action.MinionSoldAction) error                { return nil }
func (p *Processor) OnTavernUpgraded(_ *action.TavernUpgradedAction) error        { return nil }
func (p *Processor) OnTavernSpellPlayed(_ *action.TavernSpellPlayedAction) error  { return nil }
func (p *Processor) OnMinionPermStatChanged(_ *action.MinionPermStatChangedAction) error { return nil }
func (p *Processor) OnDntEnchantment(_ *action.DntEnchantmentAction) error        { return nil }
func (p *Processor) OnPlayerTagChanged(_ *action.PlayerTagChangedAction) error     { return nil }
func (p *Processor) OnEconomyChanged(_ *action.EconomyChangedAction) error        { return nil }

// ── CombatVisitor (no-ops until Phase 6) ─────────────────────────────────────

func (p *Processor) OnMinionTempStatChanged(_ *action.MinionTempStatChangedAction) error { return nil }
func (p *Processor) OnHeroDamaged(_ *action.HeroDamagedAction) error                    { return nil }
func (p *Processor) OnMinionAttacked(_ *action.MinionAttackedAction) error               { return nil }
func (p *Processor) OnDeathrattleTriggered(_ *action.DeathrattleTriggeredAction) error   { return nil }
func (p *Processor) OnCombatTavernSpell(_ *action.CombatTavernSpellAction) error        { return nil }
func (p *Processor) OnCombatEconomyEffect(_ *action.CombatEconomyEffectAction) error    { return nil }

// ── resetProcessorState clears all per-game processor fields ─────────────────

func (p *Processor) resetProcessorState() {
	p.pendingPlacement = 0
	p.localPlayerID = 0
	p.localPlayerName = ""
	p.localHeroID = 0
	p.partnerPlayerID = 0
	p.partnerPlayerName = ""
	p.partnerHeroID = 0
	p.isDuos = false
	p.punishLeaversActive = false
	p.duosFromTeammate = false
	p.partnerCombatActive = false
	p.partnerCombatHeroCtrl = 0
	p.partnerCombatMinions = nil
	p.partnerBoardSetupDone = false
	p.combatPhaseActive = false
	p.combatPhaseEntityIDs = nil
	p.combatCopies = nil
	p.entityController = make(map[int]int)
	p.heroEntities = make(map[int]bool)
	p.entityProps = make(map[int]*entityInfo)
	p.localBuffs = newBuffTracker()
	p.partnerBuffs = newBuffTracker()
	p.dntTeamTotal = make(map[string][2]int)
	p.dntPartnerAccum = make(map[string][2]int)
	p.localCombatResult = 0
	p.pendingHeroAttackerID = 0
	p.bgTurnsStarted = 0
	p.seenTribes = make(map[string]bool)
	p.entityTribeReg = make(map[int]string)
	p.tribeConfirmCount = make(map[string]int)
	p.playerEntityIDs = make(map[int]int)
	p.realPlayerIDs = make(map[int]int)
}

// suppress unused imports during migration
var _ = strconv.Atoi
