package gamestate

// processor_visitor.go implements the three visitor interfaces on Processor.
//
// Migration status:
//   Phase 1 — all no-ops (Handle() does all work)
//   Phase 2 — lifecycle events migrated: OnGameStart, OnPlayerDef, OnPlayerName,
//              OnTurnTransition, OnGameEnd
//   Phase 3 — player buff tags and economy counters migrated: OnPlayerTagChanged,
//              OnEconomyChanged, OnCombatEconomyEffect
//   Phase 4 — Dnt enchantments migrated: OnDntEnchantment
//   Phase 5 — board/stat changes migrated: OnMinionPermStatChanged, OnMinionBought, OnMinionSold
//   Phase 7.1 — reconnect migrated: OnReconnect
//   Phase 7.2 — combat outcome migrated: OnCombatOutcome
//   Phase 7.3 — player stat tags migrated: OnPlayerTriplesChanged, OnGoldChanged, OnTavernTierChanged
//   Phase 7.4 — entity registration migrated: OnAnomalyRegistered, OnEnchantmentRegistered,
//               OnHeroRegistered, OnMinionRegistered
//   Phase 8 — hero stat tags migrated: OnHeroStatChanged (HEALTH, ATK, ARMOR, DAMAGE, SPELL_POWER)
//   Phase 9 — duos teammate and hero entity migrated: OnDuosTeammate, OnHeroEntityAssigned
//   Phase 10 — hero stat tags thin-adapted: OnHeroStatChanged (HEALTH, ATK, ARMOR, DAMAGE, SPELL_POWER)
//   Phase 11 — duos backup detection migrated: OnDuosPassableChanged, OnDuosPunishLeaversChanged,
//               OnCombatAttacker; punishLeaversActive regression fixed via ReconnectAction
//   Phase 12 — CONTROLLER and BACON_CURRENT_COMBAT_PLAYER_ID migrated:
//               OnEntityControllerChanged, OnCombatPlayerChanged
//   Phase 13 — PROPOSED_DEFENDER migrated: OnCombatDefender; zero-attacker reset fixed;
//               duosFromTeammate added to reconnectStash for correct reconnect restoration

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/parser"
)

// Compile-time interface compliance checks.
var _ action.RecruitVisitor    = (*Processor)(nil)
var _ action.CombatVisitor     = (*Processor)(nil)
var _ action.TransitionVisitor = (*Processor)(nil)

// ── Visitor adapter accessors ─────────────────────────────────────────────────

func (p *Processor) AsRecruitVisitor() action.RecruitVisitor       { return p }
func (p *Processor) AsCombatVisitor() action.CombatVisitor         { return p }
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
			duosFromTeammate:   p.duosFromTeammate,
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
	// Safety: clear any stale triple gate that was never resolved by PLAYER_TRIPLES.
	p.tripleFormationActive = false
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
	// On combat transitions in duos, the partner board captured during the
	// previous combat is now outdated. Mark it stale; SetPartnerBoard clears
	// the flag again if fresh combat copies are captured during this combat.
	if turn%2 == 0 && p.isDuos {
		p.machine.MarkPartnerBoardStale()
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
// Called when EventGameEntityTags sees STATE=RUNNING with TURN > 1 after a
// new CREATE_GAME was received mid-game. Stash is populated by OnGameStart.
//
// PunishLeaversActive must be set unconditionally — it is needed for the backup duos
// detection path (DUO_PASSABLE conjunction check) even on normal game starts where
// IsRunning is false.
func (p *Processor) OnReconnect(a *action.ReconnectAction) error {
	if a.PunishLeaversActive {
		p.punishLeaversActive = true
		slog.Info("PUNISH_LEAVERS flag recorded (not sufficient alone for duos)")
	}

	if !a.IsRunning || a.Turn <= 1 || p.reconnectStash == nil {
		p.reconnectStash = nil
		return nil
	}
	slog.Info("reconnect detected, restoring game state",
		"origGameID", p.reconnectStash.gameID,
		"origTurn", p.reconnectStash.turn,
		"reconnectTurn", a.Turn)
	rs := p.reconnectStash
	p.machine.RestoreFromReconnect(
		rs.gameID, rs.startTime, rs.turn, rs.tavernTier,
		rs.isDuos, rs.heroCardID, rs.partnerHeroCardID, rs.partnerPlayerName,
		rs.buffSources, rs.abilityCounters,
		rs.partnerBuffSources, rs.partnerAbilityCtrs,
		rs.modifications, rs.turnSnapshots,
		rs.prevBuffSources, rs.prevAbilityCtrs, rs.prevModCount,
		rs.anomalyCardID, rs.anomalyName, rs.anomalyDescription,
		rs.availableTribes,
	)
	p.isDuos = rs.isDuos
	p.duosFromTeammate = rs.duosFromTeammate
	p.partnerPlayerID = rs.partnerPlayerID
	p.partnerPlayerName = rs.partnerPlayerName
	p.isReconnect = true
	p.reconnectStash = nil
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

// OnPlayerTriplesChanged handles PLAYER_TRIPLES tag changes on hero or player entities.
func (p *Processor) OnPlayerTriplesChanged(a *action.PlayerTriplesChangedAction) error {
	entityID := int(a.Entity)
	isLocal := p.actionIsLocalHeroOrPlayer(entityID, a.ControllerID, a.EntityName)
	if isLocal {
		p.machine.UpdatePlayerTag("PLAYER_TRIPLES", strconv.Itoa(a.Value))
		// Clear the triple-formation gate opened by TB_BaconShop_3ofKindChecke GRAVEYARD.
		p.tripleFormationActive = false
		return nil
	}
	if p.actionIsPartnerHeroOrPlayer(entityID, a.ControllerID, a.EntityName) {
		p.machine.UpdatePartnerTag("PLAYER_TRIPLES", strconv.Itoa(a.Value))
	}
	return nil
}

// OnGoldChanged handles RESOURCES / RESOURCES_USED tag changes on the local player entity.
func (p *Processor) OnGoldChanged(a *action.GoldChangedAction) error {
	entityID := int(a.Entity)
	isLocal := (a.ControllerID > 0 && a.ControllerID == p.localPlayerID) ||
		p.actionIsLocalPlayerEntity(entityID, a.EntityName)
	if isLocal {
		p.machine.UpdateGold(a.Tag, a.Value)
	}
	return nil
}

// OnTavernTierChanged handles PLAYER_TECH_LEVEL / TAVERN_TIER tag changes.
// Guards against zero-valued tiers and ambiguous controllerID==0 before localPlayerID is set.
func (p *Processor) OnTavernTierChanged(a *action.TavernTierChangedAction) error {
	if a.Tier <= 0 {
		return nil
	}
	entityID := int(a.Entity)
	// Require a positively-identified local entity: either controllerID matches
	// localPlayerID (non-zero), or fall back to player-entity name/ID check when
	// controllerID is unknown. Avoids false-positive when localPlayerID is 0.
	isLocal := (a.ControllerID != 0 && a.ControllerID == p.localPlayerID) ||
		(a.ControllerID == 0 && p.actionIsLocalPlayerEntity(entityID, a.EntityName))
	if isLocal {
		p.machine.SetTavernTier(a.Tier)
		return nil
	}
	if p.actionIsPartnerHeroOrPlayer(entityID, a.ControllerID, a.EntityName) {
		p.machine.UpdatePartnerTag(a.Tag, strconv.Itoa(a.Tier))
	}
	return nil
}

// OnPlacementChanged records the final leaderboard placement for the local player.
// Accepts placement from either the local player entity or a local hero entity
// (in Duos, placement may fire on a hero copy controlled by the local player).
func (p *Processor) OnPlacementChanged(a *action.PlacementChangedAction) error {
	if a.Value <= 0 {
		return nil
	}
	entityID := int(a.Entity)
	isLocal := p.actionIsLocalPlayerEntity(entityID, a.EntityName) ||
		(p.localPlayerID > 0 && a.ControllerID == p.localPlayerID && p.heroEntities[entityID])
	if isLocal {
		p.pendingPlacement = a.Value
	}
	return nil
}

// OnSpellcraftChanged handles numeric tag 3809 (SpellsPlayedForNagas) changes on the
// local player entity.
//
// Tag 3809 is a cumulative, all-phases count of every spell the LOCAL side casts this
// game. This is NOT identical in scope to NUM_SPELLS_PLAYED_THIS_GAME: the latter only
// counts spells played from hand during the recruit phase and stays frozen during
// combat, whereas 3809 ALSO increments for combat-triggered casts (e.g. Naga minions
// casting spells during the local player's own combat). In the repro duos game,
// NUM_SPELLS_PLAYED_THIS_GAME topped out at 85 (hand-played) while 3809 reached 136
// (85 hand + 51 combat-triggered). 3809 is therefore the authoritative spell total.
//
// 3809 lives on each player entity and only ticks for that player's own casts, so the
// actionIsLocalPlayerEntity filter restricts us to the local side — including during the
// local player's combat window. Partner/opponent player entities carry their own 3809
// (e.g. "Phoenix", "Musicisbreth" in the repro log) and are filtered out here. The DNT
// TagTransferPlayerEnchant mirror entity is also filtered (it is neither a player-entity
// ID nor the local player name).
//
// Responsibilities:
//   - Spells Played (CatSpellsCast): the absolute 3809 value (HDT mirrors this exactly —
//     SpellsPlayedForNagasCounter sets Counter = value). Setting the absolute value makes
//     this idempotent and immune to the reset/restore (3809→0→N) cycle some enchantments
//     produce. This supersedes the old NUM_SPELLS_PLAYED_THIS_GAME-driven increment, which
//     undercounted by ignoring combat casts.
//   - Naga synergy display (CatNagaSpells): the "Tier N · M/4" charge display, shown only
//     when a Naga synergy minion (Thaumaturgist, Arcane Cannoneer, Showy Cyclist,
//     Groundbreaker) is on the board.
//
// LIMITATION (Spellcraft, CatSpellcraftCast): combat-triggered casts cannot be split into
// spellcraft vs regular spells from 3809 alone — it is a single undifferentiated total and
// combat casts arrive SETASIDE→PLAY without the SPELLCRAFT_HINT flag. Spellcraft therefore
// still counts only hand-played spellcraft cards (see OnCombatTavernSpell). See
// docs/combat-spell-cast-fix.md.
func (p *Processor) OnSpellcraftChanged(a *action.SpellcraftChangedAction) error {
	entityID := int(a.Entity)
	isLocal := p.actionIsLocalPlayerEntity(entityID, a.EntityName) ||
		(p.localPlayerID > 0 && a.ControllerID == p.localPlayerID && p.heroEntities[entityID])
	if !isLocal {
		return nil
	}

	// Spells Played: authoritative all-phases total, set to the absolute 3809 value.
	//
	// Monotonic guard (`a.Value > current`) vs. HDT's plain `Counter = value`: on the
	// LOCAL player entity, 3809 is strictly monotonic — it only ever increments. The
	// transient reset/restore dips (3809→0→N) live on NON-local entities (the DNT
	// Bacon_TagTransferPlayerE enchantment mirror, combat copies), which the isLocal gate
	// above already excludes, so HDT's plain assignment would behave identically here.
	// The guard is therefore belt-and-suspenders against any unforeseen local dip and is
	// fully reconnect-safe: absolute assignment is idempotent, so a reconnect that restores
	// the counter to N and then re-emits 3809=N (or N+k) lands at N (or N+k), never 2N —
	// unlike the old NUM_SPELLS_PLAYED_THIS_GAME delta path, which could double-count.
	if a.Value > p.machine.GetAbilityCounter(CatSpellsCast) {
		p.machine.SetAbilityCounter(CatSpellsCast, a.Value, fmt.Sprintf("%d", a.Value))
	}

	snap := p.machine.State()
	if HasNagaSynergyMinion(snap.Board) {
		stacks := 1 + (a.Value / 4)
		progress := a.Value % 4
		display := fmt.Sprintf("Tier %d · %d/4", stacks, progress)
		p.machine.SetAbilityCounter(CatNagaSpells, a.Value, display)
	} else {
		p.machine.RemoveAbilityCounter(CatNagaSpells)
	}
	return nil
}

// OnPlayerBGTurnChanged handles the player TURN tag — the actual BG turn number the player sees.
// Guards against the GameEntity TURN (which is doubled and handled by OnTurnTransition).
// At each new turn: records the prior combat outcome, resets counters, and advances the game turn.
func (p *Processor) OnPlayerBGTurnChanged(a *action.PlayerBGTurnChangedAction) error {
	if a.Value <= 0 {
		return nil
	}
	entityID := int(a.Entity)
	if !p.actionIsLocalPlayerEntity(entityID, a.EntityName) {
		return nil
	}
	turn := a.Value
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
	return nil
}

// OnAnomalyRegistered handles entity registration for CARDTYPE=BATTLEGROUND_ANOMALY.
// Reads CardID from entityProps (written by handleEntityUpdate's pre-visitor section).
func (p *Processor) OnAnomalyRegistered(a *action.AnomalyRegisteredAction) error {
	info := p.entityProps[int(a.Entity)]
	if info == nil || info.CardID == "" {
		return nil
	}
	name := CardName(info.CardID)
	if name == "" {
		name = info.CardID
	}
	p.machine.SetAnomaly(info.CardID, name, CardDescription(info.CardID))
	return nil
}

// OnEnchantmentRegistered handles entity registration for CARDTYPE=ENCHANTMENT.
// Delegates to handleEnchantmentEntity which only uses e.EntityID from the event struct.
func (p *Processor) OnEnchantmentRegistered(a *action.EnchantmentRegisteredAction) error {
	info := p.entityProps[int(a.Entity)]
	if info == nil {
		return nil
	}
	p.handleEnchantmentEntity(parser.GameEvent{EntityID: int(a.Entity)}, info)
	return nil
}

// OnHeroRegistered handles entity registration for CARDTYPE=HERO.
// Replicates the three hero-identification paths from handleEntityUpdate.
func (p *Processor) OnHeroRegistered(a *action.HeroRegisteredAction) error {
	entityID := int(a.Entity)
	cardID := ""
	if a.Card != nil {
		cardID = a.Card.ID
	}

	// Path 1: local hero by controllerID.
	if a.ControllerID == p.localPlayerID && p.localPlayerID > 0 {
		if p.localHeroID > 0 && entityID != p.localHeroID {
			return nil
		}
		if hp, ok := a.Tags["HEALTH"]; ok {
			p.machine.UpdatePlayerTag("HEALTH", hp)
		}
		if dmg, ok := a.Tags["DAMAGE"]; ok {
			p.machine.UpdatePlayerTag("DAMAGE", dmg)
		}
		if armor, ok := a.Tags["ARMOR"]; ok {
			p.machine.UpdatePlayerTag("ARMOR", armor)
		}
		if tier, ok := a.Tags["PLAYER_TECH_LEVEL"]; ok {
			if t, _ := strconv.Atoi(tier); t > 0 {
				p.machine.SetTavernTier(t)
			}
		}
		if triples, ok := a.Tags["PLAYER_TRIPLES"]; ok {
			p.machine.UpdatePlayerTag("PLAYER_TRIPLES", triples)
		}
		if cardID != "" && !strings.HasPrefix(cardID, "TB_BaconShop_HERO_PH") {
			if !p.isReconnect || p.machine.State().Player.HeroCardID == "" {
				p.machine.UpdateHeroCardID(cardID)
			}
		}
		return nil
	}

	// Path 2: partner hero by PLAYER_ID tag.
	if p.isDuos && p.partnerPlayerID > 0 && a.PlayerID == p.partnerPlayerID {
		if p.partnerHeroID == 0 || entityID == p.partnerHeroID {
			p.partnerHeroID = entityID
			p.heroEntities[entityID] = true
			slog.Info("partner hero identified via PLAYER_ID tag",
				"entityID", entityID, "cardID", cardID, "playerID", a.PlayerID)
			if hp, ok := a.Tags["HEALTH"]; ok {
				p.machine.UpdatePartnerTag("HEALTH", hp)
			}
			if dmg, ok := a.Tags["DAMAGE"]; ok {
				p.machine.UpdatePartnerTag("DAMAGE", dmg)
			}
			if armor, ok := a.Tags["ARMOR"]; ok {
				p.machine.UpdatePartnerTag("ARMOR", armor)
			}
			if tier, ok := a.Tags["PLAYER_TECH_LEVEL"]; ok {
				p.machine.UpdatePartnerTag("PLAYER_TECH_LEVEL", tier)
			}
			if triples, ok := a.Tags["PLAYER_TRIPLES"]; ok {
				p.machine.UpdatePartnerTag("PLAYER_TRIPLES", triples)
			}
			if cardID != "" && !strings.HasPrefix(cardID, "TB_BaconShop_HERO_PH") {
				if !p.isReconnect || (p.machine.State().Partner == nil || p.machine.State().Partner.HeroCardID == "") {
					p.machine.UpdatePartnerHeroCardID(cardID)
				}
			}
			info := p.entityProps[entityID]
			if info != nil && info.Name != "" && p.partnerPlayerName == "" {
				p.partnerPlayerName = info.Name
				p.machine.UpdatePartnerName(p.partnerPlayerName)
			}
			return nil
		}
	}

	// Path 3: partner hero by controllerID fallback.
	if p.isDuos && a.ControllerID == p.partnerPlayerID && p.partnerPlayerID > 0 {
		if p.partnerHeroID > 0 && entityID != p.partnerHeroID {
			return nil
		}
		if hp, ok := a.Tags["HEALTH"]; ok {
			p.machine.UpdatePartnerTag("HEALTH", hp)
		}
		if dmg, ok := a.Tags["DAMAGE"]; ok {
			p.machine.UpdatePartnerTag("DAMAGE", dmg)
		}
		if armor, ok := a.Tags["ARMOR"]; ok {
			p.machine.UpdatePartnerTag("ARMOR", armor)
		}
		if tier, ok := a.Tags["PLAYER_TECH_LEVEL"]; ok {
			p.machine.UpdatePartnerTag("PLAYER_TECH_LEVEL", tier)
		}
		if triples, ok := a.Tags["PLAYER_TRIPLES"]; ok {
			p.machine.UpdatePartnerTag("PLAYER_TRIPLES", triples)
		}
		if cardID != "" && !strings.HasPrefix(cardID, "TB_BaconShop_HERO_PH") {
			if !p.isReconnect || (p.machine.State().Partner == nil || p.machine.State().Partner.HeroCardID == "") {
				p.machine.UpdatePartnerHeroCardID(cardID)
			}
		}
	}
	return nil
}

// OnMinionRegistered handles entity registration for CARDTYPE=MINION (and default/unknown).
// Replicates the minion section from handleEntityUpdate.
func (p *Processor) OnMinionRegistered(a *action.MinionRegisteredAction) error {
	entityID := int(a.Entity)
	info := p.entityProps[entityID]
	if info == nil {
		return nil
	}
	// Require ATK or HEALTH — matches the original guard.
	if info.Attack == 0 && info.Health == 0 {
		return nil
	}
	// Skip non-minion entities (heroes, enchantments, etc.)
	if info.CardType != "MINION" {
		return nil
	}
	// Register SETASIDE combat copies during combat for snapshot recovery.
	// Skip while the partner's combat is active: in duos, the PARTNER's combat
	// copies also have CONTROLLER=localPlayerID (see the partner tracking
	// comment in handleEntityUpdate), and feeding their buffed stats into
	// UpdateSnapshotFromCombatCopy would corrupt the local board snapshot
	// whenever a partner minion shares a CardID with a local board minion.
	// Constraint: copies created before BACON_CURRENT_COMBAT_PLAYER_ID fires
	// cannot be attributed per-entity (minion copies carry no PLAYER_ID); those
	// are handled by clearing combatCopies when partner combat is identified
	// (see OnCombatPlayerChanged). In solo games partnerCombatActive is never
	// set, so behavior is unchanged.
	if a.Zone == "SETASIDE" && a.ControllerID == p.localPlayerID &&
		p.machine.Phase() == PhaseCombat && info.CardID != "" &&
		!p.partnerCombatActive {
		if p.combatCopies == nil {
			p.combatCopies = make(map[int]*combatCopyPeak)
		}
		p.combatCopies[entityID] = &combatCopyPeak{
			CardID: info.CardID,
			ATK:    info.Attack,
			Health: info.Health,
		}
		if info.Attack > 0 || info.Health > 0 {
			p.machine.UpdateSnapshotFromCombatCopy(entityID, info.CardID, info.Attack, info.Health)
		}
	}
	// Only board minions in PLAY zone.
	if a.Zone != "PLAY" {
		return nil
	}
	if p.machine.Phase() == PhaseGameOver {
		return nil
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
	if a.ControllerID == p.localPlayerID {
		p.machine.UpsertMinion(mn)
		if p.machine.Phase() == PhaseRecruit {
			p.machine.UpdateBoardSnapshot()
		}
	}
	return nil
}

// OnHeroStatChanged handles HEALTH/ATK/ARMOR/DAMAGE/SPELL_POWER changes on hero entities.
// ARMOR is cached in entityProps for retroactive application when the local hero is later
// identified via HERO_ENTITY — that caching happens in the Handle() thin adapter, not here.
// ATK has no hero path in the original code (heroes don't receive ATK changes in BG).
// SPELL_POWER uses a controller-only check (no entity-level hero check).
func (p *Processor) OnHeroStatChanged(a *action.HeroStatChangedAction) error {
	entityID := int(a.Entity)
	switch a.Tag {
	case "SPELL_POWER":
		// SPELL_POWER uses a player-level controller check (not hero entity check).
		// No partner path exists in the original code.
		if a.ControllerID == p.localPlayerID || a.ControllerID == 0 {
			p.machine.UpdatePlayerTag(a.Tag, strconv.Itoa(a.NewValue))
		}
		return nil
	}
	// Local hero path.
	if p.actionIsLocalHero(entityID, a.ControllerID) {
		p.machine.UpdatePlayerTag(a.Tag, strconv.Itoa(a.NewValue))
		return nil
	}
	// Partner hero path.
	if p.actionIsPartnerHero(entityID, a.ControllerID) {
		p.machine.UpdatePartnerTag(a.Tag, strconv.Itoa(a.NewValue))
	}
	return nil
}

// OnDuosTeammate handles BACON_DUO_TEAMMATE_PLAYER_ID changes on the local player entity.
// This is a fallback/reconnect detection path; the authoritative path is OnPlayerDef
// (which fires from the CREATE_GAME Player block). Resolves partner name from entityProps
// if already seen.
func (p *Processor) OnDuosTeammate(a *action.DuosTeammateAction) error {
	if a.PartnerPlayerID <= 0 {
		return nil
	}
	if p.isDuos {
		// Duos already detected (e.g. via the backup PUNISH_LEAVERS+DUO_PASSABLE
		// path). The teammate tag is still authoritative evidence: record the
		// partner ID if not yet known and upgrade detection so a later
		// BACON_DUOS_PUNISH_LEAVERS=0 cannot incorrectly clear duos.
		// Only the redundant re-detection side effects are skipped.
		if !p.duosFromTeammate {
			p.duosFromTeammate = true
			slog.Info("Duos detection upgraded to authoritative teammate signal",
				"partnerPlayerID", a.PartnerPlayerID)
		}
		if p.partnerPlayerID == 0 {
			p.partnerPlayerID = a.PartnerPlayerID
			p.resolvePartnerNameFromDefs(a.PartnerPlayerID)
		}
		return nil
	}
	p.isDuos = true
	p.duosFromTeammate = true
	p.partnerPlayerID = a.PartnerPlayerID
	p.machine.SetDuosMode(true)
	slog.Info("Duos detected", "partnerPlayerID", a.PartnerPlayerID)
	p.resolvePartnerNameFromDefs(a.PartnerPlayerID)
	return nil
}

// resolvePartnerNameFromDefs tries to resolve the partner's display name from
// already-seen player defs and pushes it to the machine if found.
func (p *Processor) resolvePartnerNameFromDefs(partnerPlayerID int) {
	if entityID, ok := p.realPlayerIDs[partnerPlayerID]; ok {
		if info := p.entityProps[entityID]; info != nil && info.Name != "" {
			p.partnerPlayerName = info.Name
			p.machine.UpdatePartnerName(info.Name)
		}
	}
}

// OnHeroEntityAssigned handles HERO_ENTITY changes on the local or partner player entity.
// Applies the placeholder-hero swap logic (TB_BaconShop_HERO_PH check) and retroactive
// ARMOR/Health/CardID application from entityProps when the hero is first identified.
func (p *Processor) OnHeroEntityAssigned(a *action.HeroEntityAssignedAction) error {
	entityID := int(a.Entity)
	isLocal := p.actionIsLocalPlayerEntity(entityID, a.EntityName)
	isPartner := !isLocal && p.actionIsPartnerPlayerEntity(entityID, a.EntityName)
	if !isLocal && !isPartner {
		return nil
	}

	if isLocal {
		heroID := a.HeroEntityID
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
	} else {
		heroID := a.HeroEntityID
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
	return nil
}

// OnDuosPassableChanged handles BACON_DUO_PASSABLE TAG_CHANGE events.
// When combined with punishLeaversActive, a value of 1 triggers backup duos detection.
func (p *Processor) OnDuosPassableChanged(a *action.DuosPassableChangedAction) error {
	if a.Value == 1 && !p.isDuos && p.punishLeaversActive {
		p.isDuos = true
		p.machine.SetDuosMode(true)
		slog.Info("Duos detected from combined PUNISH_LEAVERS + DUO_PASSABLE")
	}
	return nil
}

// OnDuosPunishLeaversChanged handles BACON_DUOS_PUNISH_LEAVERS TAG_CHANGE events.
// A value of 0 clears backup-path duos detection if duos was not set via the authoritative
// BACON_DUO_TEAMMATE_PLAYER_ID path.
func (p *Processor) OnDuosPunishLeaversChanged(a *action.DuosPunishLeaversChangedAction) error {
	if a.Value == 0 && p.isDuos && !p.duosFromTeammate {
		p.isDuos = false
		p.punishLeaversActive = false
		p.machine.SetDuosMode(false)
		slog.Info("Duos cleared — PUNISH_LEAVERS changed to 0 (backup-only detection)")
	}
	return nil
}

// OnEntityControllerChanged updates the entity controller registry for TAG_CHANGE CONTROLLER events.
func (p *Processor) OnEntityControllerChanged(a *action.EntityControllerChangedAction) error {
	entityID := int(a.Entity)
	if entityID > 0 && a.ControllerID > 0 {
		p.entityController[entityID] = a.ControllerID
	}
	return nil
}

// OnCombatPlayerChanged handles BACON_CURRENT_COMBAT_PLAYER_ID tag changes on the local
// player entity. Manages partner combat tracking, phase bookkeeping, and deferred partner
// resolution. combatPhaseActive is set before collectPartnerCombatRetro is called — that
// ordering is required for the retro scan to collect the right entities.
func (p *Processor) OnCombatPlayerChanged(a *action.CombatPlayerChangedAction) error {
	if !p.actionIsLocalPlayerEntity(int(a.Entity), a.EntityName) {
		return nil
	}
	combatPlayerID := a.CombatPlayerID

	// If partner combat was active and is now ending, snapshot the board.
	if p.partnerCombatActive && combatPlayerID != p.partnerPlayerID {
		p.finalizePartnerCombat()
	}

	// Track combat phase for entity collection.
	if combatPlayerID > 0 && !p.combatPhaseActive {
		p.combatPhaseActive = true
		p.combatPhaseEntityIDs = nil
	} else if combatPlayerID == 0 {
		p.combatPhaseActive = false
		p.combatPhaseEntityIDs = nil
	}

	// Deferred partner resolution — fires when partner is first identified via combat.
	if combatPlayerID > 0 && combatPlayerID != p.localPlayerID && p.isDuos && p.partnerPlayerID == 0 {
		p.resolvePartner(combatPlayerID)
	}

	// Start tracking partner combat if this is the partner's turn to fight.
	if combatPlayerID > 0 && combatPlayerID == p.partnerPlayerID {
		p.partnerCombatActive = true
		p.partnerCombatMinions = nil
		p.opponentCombatMinions = nil
		p.partnerBoardSetupDone = false
		// Drop combat-copy peak trackers: any copies registered earlier in this
		// combat that belong to the partner (created before this flag fired —
		// the same pre-flag window collectPartnerCombatRetro exists for) must
		// not feed buffed partner stats into the local board snapshot. Local
		// copies from an earlier local combat lose nothing: their peaks were
		// already applied to the snapshot (UpdateSnapshotFromCombatCopy only
		// ever raises stats), and any later raises on them would not be local
		// recruit stats anyway.
		p.combatCopies = nil
		p.collectPartnerCombatRetro()
	}

	return nil
}

// ── RecruitVisitor ────────────────────────────────────────────────────────────

// OnMinionBought handles ZONE→PLAY transitions for the local player's minions.
func (p *Processor) OnMinionBought(a *action.MinionBoughtAction) error {
	p.tryAddMinionFromRegistry(int(a.Entity), a.ControllerID)
	return nil
}

// OnMinionSold handles non-PLAY zone transitions: removes minion from board.
func (p *Processor) OnMinionSold(a *action.MinionSoldAction) error {
	entityID := int(a.Entity)

	// TB_BaconShop_3ofKindChecke going GRAVEYARD fires before the consumed minion
	// zone changes during triple formation. Gate subsequent board removals so they
	// are not counted as sells. The gate is cleared in OnPlayerTriplesChanged.
	if info := p.entityProps[entityID]; info != nil && info.CardID == "TB_BaconShop_3ofKindChecke" {
		p.tripleFormationActive = true
		return nil
	}

	// Capture guards before mutation: only count explicit sells by the local player.
	// Phase guard prevents combat deaths from counting; board guard filters opponent entities.
	isRecruit := p.machine.Phase() == PhaseRecruit
	isOnBoard := p.machine.HasMinion(entityID)

	p.machine.RemoveMinion(entityID)
	p.machine.RemoveEnchantmentsForEntity(entityID)
	if p.machine.Phase() == PhaseRecruit {
		p.machine.UpdateBoardSnapshot()
	}
	// PLAY→HAND is a bounce back to hand (e.g. return-to-hand effects), not a
	// sale — the minion leaves the board but the sold counter must not move.
	bouncedToHand := a.NewZone == "HAND"
	if isRecruit && isOnBoard && !p.tripleFormationActive && !bouncedToHand {
		count := p.machine.GetAbilityCounter("MINIONS_SOLD") + 1
		p.machine.SetAbilityCounter("MINIONS_SOLD", count, fmt.Sprintf("%d", count))
	}
	return nil
}

func (p *Processor) OnTavernUpgraded(_ *action.TavernUpgradedAction) error { return nil }

// OnTavernSpellPlayed is a no-op. It exists only to satisfy the RecruitVisitor interface.
// The Spells Played counter (CatSpellsCast) is driven exclusively from tag 3809 (see
// OnSpellcraftChanged), which is the all-phases total and supersedes the hand-only
// NUM_SPELLS_PLAYED_THIS_GAME signal. That tag is no longer dispatched at all (the
// processor.go case was removed), so this method has no live caller — it is retained as
// the recruit-phase visitor hook in case future logic needs a per-hand-spell event.
func (p *Processor) OnTavernSpellPlayed(_ *action.TavernSpellPlayedAction) error {
	return nil
}

// OnMinionPermStatChanged handles ATK/HEALTH changes during recruit phase.
func (p *Processor) OnMinionPermStatChanged(a *action.MinionPermStatChangedAction) error {
	p.updateMinionStatByID(int(a.Entity), a.EntityName, a.Stat, a.NewValue)
	return nil
}

// OnDntEnchantment handles TAG_SCRIPT_DATA_NUM changes on Dnt enchantment entities.
// Delegates to handleDntTagChange which dispatches by enchantment card ID.
func (p *Processor) OnDntEnchantment(a *action.DntEnchantmentAction) error {
	p.handleDntTagChange(int(a.Entity), a.Tag, a.Value)
	return nil
}

// OnPlayerTagChanged handles buff-source player tags: Bloodgem, Elemental, TavernSpell.
// Guards against enchantment entities (e.g. Bacon_TagTransferPlayerE) that mirror
// player tags with stale values.
// Skips combat phase: combat save/restore enchantments (e.g. BG29_813e) zero these
// tags at combat start and restore via PowerTaskList (filtered), which would otherwise
// leave a spurious ComputeBloodgemValue(0)=1 in the display.
func (p *Processor) OnPlayerTagChanged(a *action.PlayerTagChangedAction) error {
	if p.machine.Phase() == PhaseCombat {
		return nil
	}
	entityID := int(a.Entity)
	if entityID > 0 {
		if info := p.entityProps[entityID]; info != nil && info.CardType == "ENCHANTMENT" {
			return nil
		}
	}
	if !p.isLocalEntityByIDAndController(entityID, a.ControllerID, a.EntityName) {
		return nil
	}
	p.updateBuffSourceFromPlayerTag(a.Tag, strconv.Itoa(a.Value))
	return nil
}

// OnEconomyChanged handles BACON_FREE_REFRESH_COUNT and BACON_PLAYER_EXTRA_GOLD_NEXT_TURN
// during recruit phase.
func (p *Processor) OnEconomyChanged(a *action.EconomyChangedAction) error {
	entityID := int(a.Entity)
	if entityID > 0 {
		if info := p.entityProps[entityID]; info != nil && info.CardType == "ENCHANTMENT" {
			return nil
		}
	}
	if !p.isLocalEntityByIDAndController(entityID, a.ControllerID, a.EntityName) {
		return nil
	}
	switch a.Tag {
	case "BACON_FREE_REFRESH_COUNT":
		if a.Value > 0 {
			p.machine.SetAbilityCounter(CatFreeRefresh, a.Value, fmt.Sprintf("%d", a.Value))
		} else {
			p.machine.RemoveAbilityCounter(CatFreeRefresh)
		}
	case "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN":
		p.localBuffs.goldNextTurnSure = a.Value
		p.updateGoldNextTurnCounter()
	}
	return nil
}

// ── CombatVisitor ─────────────────────────────────────────────────────────────

// OnMinionTempStatChanged handles ATK/HEALTH changes during combat.
// The board snapshot preserves recruit stats; combat changes are simulation-only.
// Partner combat minion tracking and combat copy peaks are handled by the Handle()
// caller (they need additional event context not carried in the action).
func (p *Processor) OnMinionTempStatChanged(a *action.MinionTempStatChangedAction) error {
	// During combat, stat changes don't update the main board (snapshot-preserved).
	// updateMinionStatByID handles the entity registry update and change buffering.
	p.updateMinionStatByID(int(a.Entity), "", a.Stat, a.NewValue)
	return nil
}
func (p *Processor) OnHeroDamaged(_ *action.HeroDamagedAction) error                   { return nil }
func (p *Processor) OnMinionAttacked(_ *action.MinionAttackedAction) error             { return nil }
func (p *Processor) OnDeathrattleTriggered(_ *action.DeathrattleTriggeredAction) error { return nil }
// OnCombatTavernSpell fires when a SPELL entity with SPELLCRAFT_HINT=1 transitions
// ZONE=PLAY from a local player's HAND — i.e., a spellcraft card was played.
// Spellcraft cards are identified by the SPELLCRAFT_HINT tag set while in HAND.
// This fires in any phase (typically recruit; spellcraft is a shop-phase mechanic).
func (p *Processor) OnCombatTavernSpell(_ *action.CombatTavernSpellAction) error {
	count := p.machine.GetAbilityCounter(CatSpellcraftCast) + 1
	p.machine.SetAbilityCounter(CatSpellcraftCast, count, fmt.Sprintf("%d", count))
	return nil
}

// OnCombatAttacker handles GameEntity PROPOSED_ATTACKER changes during combat.
// On the first combat action, marks partner board setup as done to stop collecting
// partner combat minions. Buffers the hero attacker ID for the subsequent PROPOSED_DEFENDER
// resolution in OnCombatOutcome.
func (p *Processor) OnCombatAttacker(a *action.CombatAttackerAction) error {
	if p.partnerCombatActive && !p.partnerBoardSetupDone {
		p.partnerBoardSetupDone = true
	}
	if a.AttackerID > 0 && a.IsHeroAttacker {
		p.pendingHeroAttackerID = a.AttackerID
	} else {
		p.pendingHeroAttackerID = 0
	}
	return nil
}

// OnCombatDefender resolves the win/loss outcome when PROPOSED_DEFENDER fires.
// Checks pendingHeroAttackerID (set by OnCombatAttacker) and defender hero status
// to determine if this is a hero-vs-hero attack involving the local player.
// Always clears pendingHeroAttackerID regardless of outcome.
func (p *Processor) OnCombatDefender(a *action.CombatDefenderAction) error {
	defer func() { p.pendingHeroAttackerID = 0 }()
	if p.pendingHeroAttackerID <= 0 || !a.IsHeroDefender {
		return nil
	}
	// Both attacker and defender are heroes — end-of-combat attack.
	// Winner's hero attacks the loser's hero.
	if p.pendingHeroAttackerID == p.localHeroID {
		return p.OnCombatOutcome(&action.CombatOutcomeAction{
			ActionBase: a.ActionBase,
			IsLocalWin: true,
		})
	} else if a.DefenderID == p.localHeroID {
		return p.OnCombatOutcome(&action.CombatOutcomeAction{
			ActionBase: a.ActionBase,
			IsLocalWin: false,
		})
	}
	return nil
}

// OnCombatOutcome records the win/loss result when a hero-vs-hero attack resolves.
// Called after PROPOSED_ATTACKER + PROPOSED_DEFENDER are both confirmed as hero entities
// and one of them is the local hero. localCombatResult is read at TURN change time
// to update the streak counters.
func (p *Processor) OnCombatOutcome(a *action.CombatOutcomeAction) error {
	if a.IsLocalWin {
		p.localCombatResult = 1
	} else {
		p.localCombatResult = -1
	}
	return nil
}

// OnCombatEconomyEffect handles BACON_PLAYER_EXTRA_GOLD_NEXT_TURN changes during combat
// (e.g. from Overconfidence or rally spells that grant extra gold next turn).
func (p *Processor) OnCombatEconomyEffect(a *action.CombatEconomyEffectAction) error {
	entityID := int(a.Entity)
	if entityID > 0 {
		if info := p.entityProps[entityID]; info != nil && info.CardType == "ENCHANTMENT" {
			return nil
		}
	}
	if !p.isLocalEntityByIDAndController(entityID, a.ControllerID, "") {
		return nil
	}
	if a.Tag == "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN" {
		p.localBuffs.goldNextTurnSure = a.Value
		p.updateGoldNextTurnCounter()
	}
	return nil
}

// isLocalEntityByIDAndController returns true if the entity belongs to the local player or hero.
// entityName is used as a fallback when controllerID is 0 (bare-name log references like
// "TAG_CHANGE Entity=Alice" have no player= field and thus PlayerID=0 in the event).
func (p *Processor) isLocalEntityByIDAndController(entityID, controllerID int, entityName string) bool {
	// Direct controller match.
	if p.localPlayerID > 0 && controllerID == p.localPlayerID {
		return true
	}
	// Name fallback: bare-name entity references have controllerID==0; use localPlayerName.
	if p.localPlayerID > 0 && controllerID == 0 && p.localPlayerName != "" && entityName == p.localPlayerName {
		return true
	}
	return false
}

// ── Action-layer entity identity helpers ──────────────────────────────────────
// These mirror isLocalHero / isLocalPlayerEntity / isPartnerHero / isPartnerPlayerEntity
// but accept (entityID, controllerID, entityName) from action structs rather than
// parser.GameEvent. Used by visitor methods that have no access to the raw event.

// actionIsLocalHero returns true when the entity is the local hero.
// Mirrors isLocalHero(e, controllerID): requires entityID > 0 and controller match.
func (p *Processor) actionIsLocalHero(entityID, controllerID int) bool {
	if entityID <= 0 || p.localPlayerID == 0 || controllerID != p.localPlayerID {
		return false
	}
	if p.localHeroID > 0 {
		return entityID == p.localHeroID
	}
	return p.heroEntities[entityID]
}

// actionIsLocalPlayerEntity returns true when the entity is the local player entity.
// Mirrors isLocalPlayerEntity(e): uses playerEntityIDs map then name fallback.
func (p *Processor) actionIsLocalPlayerEntity(entityID int, entityName string) bool {
	if p.localPlayerID > 0 {
		if pid, ok := p.playerEntityIDs[entityID]; ok {
			return pid == p.localPlayerID
		}
	}
	if p.localPlayerID > 0 && p.localPlayerName != "" && entityName == p.localPlayerName {
		return true
	}
	return false
}

// actionIsLocalHeroOrPlayer combines both local-entity checks.
func (p *Processor) actionIsLocalHeroOrPlayer(entityID, controllerID int, entityName string) bool {
	return p.actionIsLocalHero(entityID, controllerID) ||
		p.actionIsLocalPlayerEntity(entityID, entityName)
}

// actionIsPartnerHero returns true when the entity is the partner's hero.
// Mirrors isPartnerHero(e, controllerID).
func (p *Processor) actionIsPartnerHero(entityID, controllerID int) bool {
	if !p.isDuos || entityID <= 0 {
		return false
	}
	if p.partnerHeroID > 0 && entityID == p.partnerHeroID {
		return true
	}
	return p.partnerPlayerID > 0 && controllerID == p.partnerPlayerID && p.heroEntities[entityID]
}

// actionIsPartnerPlayerEntity returns true when the entity is the partner player entity.
// Mirrors isPartnerPlayerEntity(e).
func (p *Processor) actionIsPartnerPlayerEntity(entityID int, entityName string) bool {
	if !p.isDuos || p.partnerPlayerID == 0 {
		return false
	}
	if pid, ok := p.playerEntityIDs[entityID]; ok {
		return pid == p.partnerPlayerID
	}
	return p.partnerPlayerName != "" && entityName == p.partnerPlayerName
}

// actionIsPartnerHeroOrPlayer combines both partner-entity checks.
func (p *Processor) actionIsPartnerHeroOrPlayer(entityID, controllerID int, entityName string) bool {
	return p.actionIsPartnerHero(entityID, controllerID) ||
		p.actionIsPartnerPlayerEntity(entityID, entityName)
}

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
	p.partnerCombatMinions = nil
	p.opponentCombatMinions = nil
	p.partnerBoardSetupDone = false
	p.combatPhaseActive = false
	p.combatPhaseEntityIDs = nil
	p.combatCopies = nil
	p.tripleFormationActive = false
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
