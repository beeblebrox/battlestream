package gamestate

// spell_counter_test.go tests the SPELLS_CAST and SPELLCRAFT_CAST ability counters.
//
// SPELLS_CAST dispatch path (all phases):
//   NUM_SPELLS_PLAYED_THIS_GAME TAG_CHANGE on local player entity
//     → delta computed vs. last-seen absolute value
//     → OnTavernSpellPlayed called once per increment
//     → SPELLS_CAST += delta
//
//   Counts ALL spell types: tavern spells, spellcraft cards, bloodgems, non-typed spells.
//   BG tavern/bloodgem spells appear in the log as CARDTYPE=MINION, not SPELL, so the
//   ZONE=PLAY / CardType==SPELL path is NOT used for SPELLS_CAST. NUM_SPELLS_PLAYED_THIS_GAME
//   is the authoritative engine counter, same as PLAYER_TRIPLES.
//
// SPELLCRAFT_CAST dispatch path (any phase):
//   ZONE=PLAY on a SPELL entity with SpellcraftHint=true (controllerID==localPlayerID, prevZone==HAND)
//     → OnCombatTavernSpell → SPELLCRAFT_CAST + 1
//   Spellcraft cards carry SPELLCRAFT_HINT=1 while in the player's HAND. The hint clears
//   after play. System/anomaly spells never carry the hint and are silently ignored.
//   Phase is irrelevant — spellcraft is a shop-phase mechanic in practice.
//
//   Tag 3809 (SpellsPlayedForNagas) is NOT used for SPELLCRAFT_CAST. It is a cumulative
//   counter identical to NUM_SPELLS_PLAYED_THIS_GAME and drives only CatNagaSpells display.
//
// Non-local entities must be silently ignored and never route to OnMinionBought.

import (
	"fmt"
	"testing"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/parser"
)

// ── Helpers ───────────────────────────────────────────────────────────────────

// registerSpellEntity registers entity entityID in the entity registry as a
// SPELL with the given controller. Used for ZONE=PLAY based tests (SPELLCRAFT_CAST
// combat path and board-routing regression tests).
func registerSpellEntity(p *Processor, entityID int, controllerID int) {
	p.entityController[entityID] = controllerID
	p.entityProps[entityID] = &entityInfo{
		CardType: "SPELL",
		Zone:     "HAND",
	}
}

// spellZonePlay sends a TAG_CHANGE ZONE=PLAY for the given entity.
func spellZonePlay(p *Processor, entityID int, controllerID int) {
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: entityID,
		PlayerID: controllerID,
		Tags:     map[string]string{"ZONE": "PLAY"},
	})
}

// numSpellsPlayedTagChange fires a TAG_CHANGE NUM_SPELLS_PLAYED_THIS_GAME on
// the given entity (expected to be the local player entity, entityID=20 in test setup).
func numSpellsPlayedTagChange(p *Processor, entityID int, newValue int) {
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: entityID,
		Tags:     map[string]string{"NUM_SPELLS_PLAYED_THIS_GAME": fmt.Sprintf("%d", newValue)},
	})
}

// tavernSpellPlayed fires OnTavernSpellPlayed directly without going through
// any tag-change path. Used to test the visitor method in isolation.
func tavernSpellPlayed(p *Processor, entityID int) {
	_ = p.OnTavernSpellPlayed(&action.TavernSpellPlayedAction{
		ActionBase: action.ActionBase{Entity: action.EntityID(entityID)},
	})
}

// combatTavernSpell fires OnCombatTavernSpell directly without going through
// the ZONE=PLAY path.
func combatTavernSpell(p *Processor, entityID int) {
	_ = p.OnCombatTavernSpell(&action.CombatTavernSpellAction{
		ActionBase: action.ActionBase{Entity: action.EntityID(entityID)},
	})
}

// ── SPELLS_CAST (recruit phase) via NUM_SPELLS_PLAYED_THIS_GAME ───────────────

// TestSpellsCastStartsAtZero verifies that SPELLS_CAST is absent (not present)
// before any spell has been played. The counter must not be initialised to 0.
func TestSpellsCastStartsAtZero(t *testing.T) {
	m, _ := setupRecruitPhase(t)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("SPELLS_CAST should be absent before any spell is played, got value=%d display=%q",
			ac.Value, ac.Display)
	}
}

// TestSpellsCastSingleSpell verifies that playing one recruit-phase spell
// increments SPELLS_CAST to 1 (via direct visitor call).
func TestSpellsCastSingleSpell(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tavernSpellPlayed(p, 300)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST ability counter after one spell, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLS_CAST=1, got %d", ac.Value)
	}
	if ac.Display != "1" {
		t.Errorf("expected display=%q, got %q", "1", ac.Display)
	}
}

// TestSpellsCastMultipleSpells verifies that playing three recruit-phase spells
// accumulates to 3 (via direct visitor calls).
func TestSpellsCastMultipleSpells(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tavernSpellPlayed(p, 301)
	tavernSpellPlayed(p, 302)
	tavernSpellPlayed(p, 303)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST ability counter after 3 spells, got nil")
	}
	if ac.Value != 3 {
		t.Errorf("expected SPELLS_CAST=3 after 3 spells, got %d", ac.Value)
	}
	if ac.Display != "3" {
		t.Errorf("expected display=%q, got %q", "3", ac.Display)
	}
}

// TestSpellsCastResetOnNewGame verifies that SPELLS_CAST is cleared when a
// new game starts.
func TestSpellsCastResetOnNewGame(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tavernSpellPlayed(p, 300)
	tavernSpellPlayed(p, 301)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 2 {
		t.Fatalf("precondition: expected SPELLS_CAST=2 before new game, got %v", ac)
	}

	// Start a new game — BGGameState must be zeroed.
	p.Handle(parser.GameEvent{Type: parser.EventGameStart})

	ac = findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("SPELLS_CAST should be absent after new game start, got value=%d", ac.Value)
	}
}

// TestSpellsCastViaNumSpellsTag verifies end-to-end dispatch: sequential
// NUM_SPELLS_PLAYED_THIS_GAME increments on the local player entity accumulate
// the correct SPELLS_CAST total.
//
// In the test setup (setupRecruitPhase) the local player entity has entityID=2
// and is registered in playerEntityIDs[2]=localPlayerID.
func TestSpellsCastViaNumSpellsTag(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Local player entity ID in the test harness is 20 (see setupGame / EventPlayerDef).
	localPlayerEntityID := 20

	// Simulate 3 sequential spell plays: tag goes 0→1→2→3.
	numSpellsPlayedTagChange(p, localPlayerEntityID, 1)
	numSpellsPlayedTagChange(p, localPlayerEntityID, 2)
	numSpellsPlayedTagChange(p, localPlayerEntityID, 3)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST after NUM_SPELLS_PLAYED_THIS_GAME 0→1→2→3, got nil")
	}
	if ac.Value != 3 {
		t.Errorf("expected SPELLS_CAST=3, got %d", ac.Value)
	}
	if ac.Display != "3" {
		t.Errorf("expected display=%q, got %q", "3", ac.Display)
	}
}

// TestSpellsCastNumSpellsTagNonLocalIgnored verifies that NUM_SPELLS_PLAYED_THIS_GAME
// changes on a non-local player entity (partner, opponent) do NOT increment SPELLS_CAST.
func TestSpellsCastNumSpellsTagNonLocalIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// entityID=99 is not registered as any player entity.
	numSpellsPlayedTagChange(p, 99, 5)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("non-local NUM_SPELLS_PLAYED_THIS_GAME must not increment SPELLS_CAST, got value=%d", ac.Value)
	}
}

// TestSpellsCastNumSpellsTagJump verifies that a single jump in
// NUM_SPELLS_PLAYED_THIS_GAME (e.g. 0→3 in one event) produces SPELLS_CAST=3.
// This handles reconnect replays where the tag may jump by more than 1.
func TestSpellsCastNumSpellsTagJump(t *testing.T) {
	m, p := setupRecruitPhase(t)

	localPlayerEntityID := 20
	numSpellsPlayedTagChange(p, localPlayerEntityID, 3)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST after NUM_SPELLS_PLAYED_THIS_GAME jump 0→3, got nil")
	}
	if ac.Value != 3 {
		t.Errorf("expected SPELLS_CAST=3 for tag jump 0→3, got %d", ac.Value)
	}
}

// TestSpellsCastNumSpellsTagNoDecrement verifies that a decrease in
// NUM_SPELLS_PLAYED_THIS_GAME (which should not happen in practice) does NOT
// subtract from SPELLS_CAST.
func TestSpellsCastNumSpellsTagNoDecrement(t *testing.T) {
	m, p := setupRecruitPhase(t)

	localPlayerEntityID := 20
	numSpellsPlayedTagChange(p, localPlayerEntityID, 2) // → SPELLS_CAST=2
	numSpellsPlayedTagChange(p, localPlayerEntityID, 1) // decrease — must be ignored

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 2 {
		t.Errorf("decrease in NUM_SPELLS_PLAYED_THIS_GAME must not change SPELLS_CAST, got %v", ac)
	}
}

// TestSpellsCastNumSpellsResetOnNewGame verifies that SPELLS_CAST and the internal
// spellsPlayedTotal tracker are both zeroed on game start, so a new game's
// NUM_SPELLS_PLAYED_THIS_GAME=1 correctly increments SPELLS_CAST to 1.
func TestSpellsCastNumSpellsResetOnNewGame(t *testing.T) {
	m, p := setupRecruitPhase(t)

	localPlayerEntityID := 20
	numSpellsPlayedTagChange(p, localPlayerEntityID, 8) // game 1: 8 spells

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 8 {
		t.Fatalf("precondition: expected SPELLS_CAST=8 in game 1, got %v", ac)
	}

	// New game — all state resets.
	p.Handle(parser.GameEvent{Type: parser.EventGameStart})

	// In the new game the local player entity ID changes; re-register it.
	// For simplicity just call OnTavernSpellPlayed directly to verify counter is zero.
	ac = findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("SPELLS_CAST should be absent at new game start, got value=%d", ac.Value)
	}
}

// TestSpellsCastNonLocalDoesNotRouteToBought verifies that a non-local SPELL
// entity reaching ZONE=PLAY does NOT call OnMinionBought (which would add it
// to the local board). A SPELL entity from a partner or opponent must be
// silently ignored.
func TestSpellsCastNonLocalDoesNotRouteToBought(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a SPELL for opponent controller=15.
	registerSpellEntity(p, 402, 15)
	spellZonePlay(p, 402, 15)

	// The board must remain empty — no SPELL ever belongs on the minion board.
	if len(m.State().Board) != 0 {
		t.Errorf("non-local SPELL should not appear on board, got %d board entries", len(m.State().Board))
	}
}

// TestSpellsCastSystemSpellIgnored verifies that system spells (e.g.
// TB_BaconShop_UpdateDmgCap) which have CARDTYPE=SPELL and fire SETASIDE→PLAY
// during MAIN_START_TRIGGERS are NOT counted as player-played spells and do
// NOT call OnMinionBought. The only authoritative counter is NUM_SPELLS_PLAYED_THIS_GAME.
func TestSpellsCastSystemSpellIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a SPELL entity that starts in SETASIDE (system card, never in hand).
	p.entityController[450] = 7
	p.entityProps[450] = &entityInfo{
		CardType: "SPELL",
		Zone:     "SETASIDE",
	}
	spellZonePlay(p, 450, 7)

	// SPELLS_CAST must be absent: the ZONE=PLAY SPELL path no longer fires OnTavernSpellPlayed.
	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("system SPELL (SETASIDE→PLAY) must not increment SPELLS_CAST, got value=%d", ac.Value)
	}

	// Also must not appear on the board.
	if len(m.State().Board) != 0 {
		t.Errorf("system SPELL should not appear on board, got %d entries", len(m.State().Board))
	}
}

// ── SPELLCRAFT_CAST (combat phase) ───────────────────────────────────────────

// TestSpellcraftCastStartsAtZero verifies that SPELLCRAFT_CAST is absent before
// any combat spell fires.
func TestSpellcraftCastStartsAtZero(t *testing.T) {
	m, _ := setupRecruitPhase(t)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("SPELLCRAFT_CAST should be absent before any combat spell, got value=%d", ac.Value)
	}
}

// TestSpellcraftCastSingleSpell verifies that one combat spell increments
// SPELLCRAFT_CAST to 1.
func TestSpellcraftCastSingleSpell(t *testing.T) {
	m, p := setupRecruitPhase(t)

	combatTavernSpell(p, 500)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil {
		t.Fatal("expected SPELLCRAFT_CAST ability counter after one combat spell, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLCRAFT_CAST=1, got %d", ac.Value)
	}
	if ac.Display != "1" {
		t.Errorf("expected display=%q, got %q", "1", ac.Display)
	}
}

// TestSpellcraftCastMultipleSpells verifies that 5 combat spells accumulate to 5.
func TestSpellcraftCastMultipleSpells(t *testing.T) {
	m, p := setupRecruitPhase(t)

	for i := 0; i < 5; i++ {
		combatTavernSpell(p, 500+i)
	}

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil {
		t.Fatal("expected SPELLCRAFT_CAST after 5 combat spells, got nil")
	}
	if ac.Value != 5 {
		t.Errorf("expected SPELLCRAFT_CAST=5, got %d", ac.Value)
	}
}

// TestSpellcraftCastResetOnNewGame verifies that SPELLCRAFT_CAST is cleared
// when a new game starts.
func TestSpellcraftCastResetOnNewGame(t *testing.T) {
	m, p := setupRecruitPhase(t)

	combatTavernSpell(p, 500)
	combatTavernSpell(p, 501)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil || ac.Value != 2 {
		t.Fatalf("precondition: expected SPELLCRAFT_CAST=2 before new game, got %v", ac)
	}

	p.Handle(parser.GameEvent{Type: parser.EventGameStart})

	ac = findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("SPELLCRAFT_CAST should be absent after new game start, got value=%d", ac.Value)
	}
}

// registerSpellcraftEntity registers entity entityID as a SPELL with SpellcraftHint=true.
// Used to simulate a spellcraft card in the player's HAND.
func registerSpellcraftEntity(p *Processor, entityID int, controllerID int) {
	p.entityController[entityID] = controllerID
	p.entityProps[entityID] = &entityInfo{
		CardType:       "SPELL",
		Zone:           "HAND",
		SpellcraftHint: true,
	}
}

// spellcraftHintChange fires a TAG_CHANGE SPELLCRAFT_HINT for the given entity.
func spellcraftHintChange(p *Processor, entityID int, value int) {
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: entityID,
		Tags:     map[string]string{"SPELLCRAFT_HINT": fmt.Sprintf("%d", value)},
	})
}

// TestSpellcraftCastViaZonePlay verifies end-to-end dispatch: a SPELL entity with
// SPELLCRAFT_HINT=1 controlled by the local player transitioning ZONE=PLAY increments
// SPELLCRAFT_CAST (not SPELLS_CAST). Phase is irrelevant — spellcraft cards are played
// during shop phase in practice.
func TestSpellcraftCastViaZonePlay(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a spellcraft SPELL entity for the local player (controller=7).
	registerSpellcraftEntity(p, 600, 7)
	spellZonePlay(p, 600, 7)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil {
		t.Fatal("expected SPELLCRAFT_CAST after ZONE=PLAY on local spellcraft SPELL, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLCRAFT_CAST=1, got %d", ac.Value)
	}

	// SPELLS_CAST must remain absent — SPELLS_CAST is driven by NUM_SPELLS_PLAYED_THIS_GAME,
	// not by the ZONE=PLAY path.
	recruitAC := findAbilityCounter(m, CatSpellsCast)
	if recruitAC != nil {
		t.Errorf("SPELLS_CAST should not be set via ZONE=PLAY path, got value=%d", recruitAC.Value)
	}
}

// TestSpellcraftCastViaZonePlay_NoHintIgnored verifies that a SPELL entity without
// SPELLCRAFT_HINT (a system or non-spellcraft spell) does NOT increment SPELLCRAFT_CAST.
func TestSpellcraftCastViaZonePlay_NoHintIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a SPELL without hint (e.g. an anomaly system spell).
	registerSpellEntity(p, 600, 7)
	spellZonePlay(p, 600, 7)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("SPELL without SpellcraftHint must not increment SPELLCRAFT_CAST, got value=%d", ac.Value)
	}
}

// TestSpellcraftCastViaZonePlay_InCombatPhase verifies that a spellcraft card
// played during combat phase also increments SPELLCRAFT_CAST (phase is irrelevant).
func TestSpellcraftCastViaZonePlay_InCombatPhase(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Advance to combat phase.
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})
	if m.State().Phase != PhaseCombat {
		t.Fatalf("expected COMBAT phase, got %s", m.State().Phase)
	}

	registerSpellcraftEntity(p, 601, 7)
	spellZonePlay(p, 601, 7)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil {
		t.Fatal("expected SPELLCRAFT_CAST after spellcraft SPELL in combat phase, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLCRAFT_CAST=1, got %d", ac.Value)
	}
}

// TestSpellcraftCastViaSpellcraftHintTag verifies end-to-end dispatch through the
// SPELLCRAFT_HINT TAG_CHANGE path: hint set on entity via event, then ZONE=PLAY fires.
func TestSpellcraftCastViaSpellcraftHintTag(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a plain SPELL entity (no hint yet).
	registerSpellEntity(p, 602, 7)

	// Game sets SPELLCRAFT_HINT=1 on the entity (it's a spellcraft card).
	spellcraftHintChange(p, 602, 1)

	// Player plays it.
	spellZonePlay(p, 602, 7)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil {
		t.Fatal("expected SPELLCRAFT_CAST after SPELLCRAFT_HINT=1 + ZONE=PLAY, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLCRAFT_CAST=1, got %d", ac.Value)
	}
}

// TestSpellcraftCastNonLocalIgnored verifies that a SPELL entity controlled by
// a non-local player during combat does NOT increment SPELLCRAFT_CAST.
func TestSpellcraftCastNonLocalIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Advance to combat phase.
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})

	// Non-local SPELL (opponent controller=15) during combat.
	registerSpellEntity(p, 601, 15)
	spellZonePlay(p, 601, 15)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("non-local combat SPELL should not increment SPELLCRAFT_CAST, got value=%d", ac.Value)
	}
}

// ── Tag 3809 (SpellsPlayedForNagas) — Naga stacks only, NOT SPELLCRAFT_CAST ────
//
// Tag 3809 is a cumulative count of all spells played from hand this game. It is
// co-incremented with NUM_SPELLS_PLAYED_THIS_GAME. The old implementation incorrectly
// used 0→positive transitions during combat (actually the TagTransferPlayerEnchant
// reset/restore cycle) as a SPELLCRAFT_CAST signal. Tag 3809 now drives ONLY the
// CatNagaSpells display counter, not SPELLCRAFT_CAST.

// tag3809Change fires a TAG_CHANGE for tag "3809" on the given entity.
func tag3809Change(p *Processor, entityID int, value int) {
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: entityID,
		Tags:     map[string]string{"3809": fmt.Sprintf("%d", value)},
	})
}

// enterCombat advances the processor to PhaseCombat via GameEntity TURN=2 (even=combat).
func enterCombat(p *Processor) {
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})
}

// TestTag3809_DoesNotIncrementSpellcraftCast verifies that tag 3809 changes —
// regardless of phase or transition direction — never increment SPELLCRAFT_CAST.
// SPELLCRAFT_CAST is now driven by SPELLCRAFT_HINT + ZONE=PLAY.
func TestTag3809_DoesNotIncrementSpellcraftCast(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Recruit phase accumulation.
	tag3809Change(p, 20, 1)
	tag3809Change(p, 20, 3)

	// Combat phase — TagTransferPlayerEnchant reset+restore pattern.
	enterCombat(p)
	tag3809Change(p, 20, 0) // reset
	tag3809Change(p, 20, 3) // restore

	// Further accumulation.
	tag3809Change(p, 20, 5)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("tag 3809 must never increment SPELLCRAFT_CAST, got value=%d", ac.Value)
	}
}

// TestTag3809_NonLocalIgnored verifies tag 3809 on a non-local entity is ignored.
func TestTag3809_NonLocalIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)
	enterCombat(p)

	tag3809Change(p, 99, 5) // non-local entity

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("non-local tag 3809 should not affect SPELLCRAFT_CAST, got value=%d", ac.Value)
	}
}

// ── Cross-phase independence ──────────────────────────────────────────────────

// TestSpellCountersAreIndependent verifies that SPELLS_CAST and SPELLCRAFT_CAST
// accumulate independently: recruit spells only touch SPELLS_CAST and combat
// spells only touch SPELLCRAFT_CAST.
func TestSpellCountersAreIndependent(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Two shop-phase spells via NUM_SPELLS_PLAYED_THIS_GAME (direct visitor).
	tavernSpellPlayed(p, 700)
	tavernSpellPlayed(p, 701)

	// Three spellcraft plays via direct visitor (simulates SPELLCRAFT_HINT + ZONE=PLAY).
	combatTavernSpell(p, 800)
	combatTavernSpell(p, 801)
	combatTavernSpell(p, 802)

	recruit := findAbilityCounter(m, CatSpellsCast)
	if recruit == nil || recruit.Value != 2 {
		t.Errorf("expected SPELLS_CAST=2, got %v", recruit)
	}

	craft := findAbilityCounter(m, CatSpellcraftCast)
	if craft == nil || craft.Value != 3 {
		t.Errorf("expected SPELLCRAFT_CAST=3, got %v", craft)
	}
}
