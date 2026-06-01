package gamestate

// spell_counter_test.go tests the SPELLS_CAST and SPELLCRAFT_CAST ability counters.
//
// SPELLS_CAST dispatch path (all phases):
//   Tag 3809 (SpellsPlayedForNagas) TAG_CHANGE on the local player entity
//     → OnSpellcraftChanged sets SPELLS_CAST to the absolute 3809 value (HDT-style;
//       see SpellsPlayedForNagasCounter.cs which does Counter = value).
//
//   3809 is the all-phases total: it counts hand-played recruit spells AND combat-triggered
//   casts (e.g. Naga minions casting during the local player's own combat). It supersedes
//   NUM_SPELLS_PLAYED_THIS_GAME, which only counts hand-played spells and freezes during
//   combat (the old, undercounting source). OnTavernSpellPlayed (driven by
//   NUM_SPELLS_PLAYED_THIS_GAME) is now a no-op for the counter.
//
// SPELLCRAFT_CAST dispatch path (any phase):
//   ZONE=PLAY on a SPELL entity with SpellcraftHint=true (controllerID==localPlayerID, prevZone==HAND)
//     → OnCombatTavernSpell → SPELLCRAFT_CAST + 1
//   Spellcraft cards carry SPELLCRAFT_HINT=1 while in the player's HAND. The hint clears
//   after play. System/anomaly spells never carry the hint and are silently ignored.
//   LIMITATION: combat-triggered spellcraft casts cannot be separated from regular combat
//   casts in 3809 alone, so SPELLCRAFT_CAST counts only hand-played spellcraft (see
//   docs/combat-spell-cast-fix.md).
//
//   Tag 3809 is NOT used for SPELLCRAFT_CAST — only for SPELLS_CAST and the CatNagaSpells display.
//
// Non-local entities must be silently ignored and never route to OnMinionBought.

import (
	"fmt"
	"testing"
	"time"

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

// spellsCast3809 fires OnSpellcraftChanged directly with an absolute 3809 value on
// the given local player entity. This is the source of truth for SPELLS_CAST.
func spellsCast3809(p *Processor, entityID int, value int) {
	_ = p.OnSpellcraftChanged(&action.SpellcraftChangedAction{
		ActionBase: action.ActionBase{Entity: action.EntityID(entityID)},
		Value:      value,
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

// TestSpellsCastSingleSpell verifies that one 3809 increment sets SPELLS_CAST to 1.
func TestSpellsCastSingleSpell(t *testing.T) {
	m, p := setupRecruitPhase(t)

	spellsCast3809(p, 20, 1)

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

// TestSpellsCastMultipleSpells verifies that 3809 climbing to 3 yields SPELLS_CAST=3.
func TestSpellsCastMultipleSpells(t *testing.T) {
	m, p := setupRecruitPhase(t)

	spellsCast3809(p, 20, 1)
	spellsCast3809(p, 20, 2)
	spellsCast3809(p, 20, 3)

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

	spellsCast3809(p, 20, 1)
	spellsCast3809(p, 20, 2)

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

// TestSpellsCastViaTag3809 verifies end-to-end dispatch: sequential tag 3809
// TAG_CHANGE events on the local player entity set SPELLS_CAST to the absolute value.
//
// In the test setup (setupGame) the local player entity has entityID=20 and is
// registered in playerEntityIDs[20]=localPlayerID (7).
func TestSpellsCastViaTag3809(t *testing.T) {
	m, p := setupRecruitPhase(t)

	localPlayerEntityID := 20

	// Simulate 3 sequential spell plays: tag goes 1→2→3.
	tag3809Change(p, localPlayerEntityID, 1)
	tag3809Change(p, localPlayerEntityID, 2)
	tag3809Change(p, localPlayerEntityID, 3)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST after tag 3809 1→2→3, got nil")
	}
	if ac.Value != 3 {
		t.Errorf("expected SPELLS_CAST=3, got %d", ac.Value)
	}
	if ac.Display != "3" {
		t.Errorf("expected display=%q, got %q", "3", ac.Display)
	}
}

// TestSpellsCastNumSpellsDoesNotDrive verifies that NUM_SPELLS_PLAYED_THIS_GAME no
// longer drives SPELLS_CAST: SPELLS_CAST is sourced exclusively from tag 3809.
func TestSpellsCastNumSpellsDoesNotDrive(t *testing.T) {
	m, p := setupRecruitPhase(t)

	localPlayerEntityID := 20
	numSpellsPlayedTagChange(p, localPlayerEntityID, 1)
	numSpellsPlayedTagChange(p, localPlayerEntityID, 2)
	numSpellsPlayedTagChange(p, localPlayerEntityID, 3)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("NUM_SPELLS_PLAYED_THIS_GAME must not drive SPELLS_CAST (now 3809-driven), got value=%d", ac.Value)
	}
}

// TestSpellsCastTag3809NonLocalIgnored verifies that tag 3809 changes on a non-local
// player entity (partner, opponent) do NOT set SPELLS_CAST.
func TestSpellsCastTag3809NonLocalIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// entityID=99 is not registered as any player entity.
	tag3809Change(p, 99, 5)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("non-local tag 3809 must not set SPELLS_CAST, got value=%d", ac.Value)
	}
}

// TestSpellsCastTag3809Jump verifies that a single jump in tag 3809 (e.g. 0→3 in one
// event, as on a reconnect replay) produces SPELLS_CAST=3.
func TestSpellsCastTag3809Jump(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tag3809Change(p, 20, 3)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST after tag 3809 jump 0→3, got nil")
	}
	if ac.Value != 3 {
		t.Errorf("expected SPELLS_CAST=3 for tag jump 0→3, got %d", ac.Value)
	}
}

// TestSpellsCastTag3809NoDecrement verifies that a transient decrease in tag 3809
// (e.g. the TagTransferPlayerEnchant reset/restore cycle) does NOT pull SPELLS_CAST
// backwards. The absolute value is monotonic in practice; we guard against dips.
func TestSpellsCastTag3809NoDecrement(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tag3809Change(p, 20, 2) // → SPELLS_CAST=2
	tag3809Change(p, 20, 0) // transient reset — must be ignored
	tag3809Change(p, 20, 1) // partial restore, still below 2 — must be ignored

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 2 {
		t.Errorf("transient 3809 dip must not lower SPELLS_CAST, got %v", ac)
	}
}

// TestSpellsCastTag3809ResetOnNewGame verifies that SPELLS_CAST is zeroed on game
// start, so a new game starts fresh.
func TestSpellsCastTag3809ResetOnNewGame(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tag3809Change(p, 20, 8) // game 1: 8 spells

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 8 {
		t.Fatalf("precondition: expected SPELLS_CAST=8 in game 1, got %v", ac)
	}

	// New game — all state resets.
	p.Handle(parser.GameEvent{Type: parser.EventGameStart})

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

	// SPELLS_CAST must remain absent — SPELLS_CAST is driven by tag 3809,
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

// TestSpellcraftCastNonLocalWithHintIgnored verifies that a SPELL entity with
// SpellcraftHint=true controlled by a non-local player (partner, opponent) does
// NOT increment SPELLCRAFT_CAST. The controllerID guard must filter it out even
// when the entity carries the spellcraft hint.
func TestSpellcraftCastNonLocalWithHintIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a spellcraft SPELL for opponent controller=15 (not localPlayerID=7).
	registerSpellcraftEntity(p, 603, 15)
	spellZonePlay(p, 603, 15)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac != nil {
		t.Errorf("non-local spellcraft SPELL must not increment SPELLCRAFT_CAST, got value=%d", ac.Value)
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
// accumulate independently: 3809 only touches SPELLS_CAST and spellcraft hand-plays
// only touch SPELLCRAFT_CAST.
func TestSpellCountersAreIndependent(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Two spells via tag 3809 (the SPELLS_CAST source of truth).
	spellsCast3809(p, 20, 1)
	spellsCast3809(p, 20, 2)

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

// ── Regression: combat-triggered spell undercount (repro game) ────────────────

// TestSpellsCastIncludesCombatCasts reproduces the original undercount bug and locks
// in the fix. In the repro duos game (local=Moch#1358), spells were played in recruit
// (NUM_SPELLS_PLAYED_THIS_GAME and tag 3809 climbing in lockstep), then the local
// player's Nagas cast spells DURING COMBAT: tag 3809 kept climbing while
// NUM_SPELLS_PLAYED_THIS_GAME stayed frozen.
//
// Before the fix (SPELLS_CAST driven by NUM_SPELLS_PLAYED_THIS_GAME) the counter froze
// at the hand-only total and undercounted. After the fix (SPELLS_CAST driven by tag
// 3809) it reflects the all-phases total including combat casts.
//
// This uses a scaled-down version of the repro progression (recruit 3→5, combat 5→8;
// the real game was 124→130 recruit, 130→136 combat ending at the true total).
func TestSpellsCastIncludesCombatCasts(t *testing.T) {
	m, p := setupRecruitPhase(t)

	localPlayerEntityID := 20

	// Recruit phase: hand-played spells. NUM and 3809 move in lockstep.
	for i := 1; i <= 5; i++ {
		numSpellsPlayedTagChange(p, localPlayerEntityID, i)
		tag3809Change(p, localPlayerEntityID, i)
	}

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 5 {
		t.Fatalf("after recruit phase expected SPELLS_CAST=5, got %v", ac)
	}

	// Enter combat. NUM_SPELLS_PLAYED_THIS_GAME freezes (the engine stops emitting it),
	// but the local Nagas cast spells so tag 3809 keeps climbing 5→8.
	enterCombat(p)
	tag3809Change(p, localPlayerEntityID, 6)
	tag3809Change(p, localPlayerEntityID, 7)
	tag3809Change(p, localPlayerEntityID, 8)

	ac = findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST after combat casts, got nil")
	}
	if ac.Value != 8 {
		t.Errorf("SPELLS_CAST must include combat-triggered casts: expected 8 (5 hand + 3 combat), got %d", ac.Value)
	}
	if ac.Display != "8" {
		t.Errorf("expected display=%q, got %q", "8", ac.Display)
	}
}

// TestSpellsCastCombatIgnoresNonLocalSide verifies duos-safety: during combat, tag 3809
// on a NON-local player entity (partner or opponent) must not affect the local
// SPELLS_CAST counter. In the repro log the partner "Phoenix" / opponent "Musicisbreth"
// entities carry their own 3809; only Moch#1358's must count.
func TestSpellsCastCombatIgnoresNonLocalSide(t *testing.T) {
	m, p := setupRecruitPhase(t)

	tag3809Change(p, 20, 5) // local → SPELLS_CAST=5
	enterCombat(p)

	// Non-local player entities tick their own 3809 during combat. Must be ignored.
	tag3809Change(p, 99, 40)  // opponent-style entity, not registered as local
	tag3809Change(p, 21, 30)  // the dummy/bot player entity (PLAYER_ID 15, not local)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil || ac.Value != 5 {
		t.Errorf("non-local 3809 must not change local SPELLS_CAST, expected 5, got %v", ac)
	}
}

// TestSpellsCastSetWithoutNagaMinion locks the regression-critical invariant that
// CatSpellsCast is set on EVERY local 3809 tick regardless of board contents — it is NOT
// gated behind HasNagaSynergyMinion (only CatNagaSpells is). With no Naga synergy minion
// on the board, Spells Played must still reflect the 3809 value while CatNagaSpells stays
// absent. (Mirrors the real-log case in TestGameLog2026_03_07_SpellsPlayedFinal.)
func TestSpellsCastSetWithoutNagaMinion(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Board is empty — no Thaumaturgist / Arcane Cannoneer / Showy Cyclist / Groundbreaker.
	tag3809Change(p, 20, 7)

	sc := findAbilityCounter(m, CatSpellsCast)
	if sc == nil || sc.Value != 7 {
		t.Errorf("SPELLS_CAST must be set to 7 with no Naga minion on board, got %v", sc)
	}
	if naga := findAbilityCounter(m, CatNagaSpells); naga != nil {
		t.Errorf("CatNagaSpells must be absent with no synergy minion, got value=%d", naga.Value)
	}
}

// TestSpellsCastReconnectIdempotent locks the adversary's main latent-risk callout:
// absolute assignment makes CatSpellsCast immune to double-counting across a mid-game
// reconnect. We drive a real reconnect (CREATE_GAME mid-game → STATE=RUNNING/TURN>1),
// which restores the stashed CatSpellsCast=N, then re-emit 3809=N and 3809=N+k. With
// absolute assignment the counter lands at N then N+k — never 2N (which the old
// NUM_SPELLS_PLAYED_THIS_GAME delta path could produce).
func TestSpellsCastReconnectIdempotent(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Pre-reconnect: local player casts spells; 3809 reaches 40.
	tag3809Change(p, 20, 40)
	if ac := findAbilityCounter(m, CatSpellsCast); ac == nil || ac.Value != 40 {
		t.Fatalf("precondition: expected SPELLS_CAST=40 before reconnect, got %v", ac)
	}
	m.SetTurn(8) // mid-game turn, so reconnect is detected (TURN>1)

	// Mid-game reconnect: a fresh CREATE_GAME arrives, then GameEntity STATE=RUNNING TURN>1.
	// The processor stashes the live state (including CatSpellsCast=40) and restores it.
	p.Handle(parser.GameEvent{Type: parser.EventGameStart, Timestamp: time.Now()})
	// Re-register the local player WITHOUT another EventGameStart (a second GameStart would
	// re-stash and clobber the captured state). The real log re-emits PlayerDef/name here.
	p.Handle(parser.GameEvent{
		Type:     parser.EventPlayerDef,
		EntityID: 20,
		PlayerID: 7,
		Tags:     map[string]string{"hi": "144115193835963207", "lo": "30722021", "PLAYER_ID": "7"},
	})
	p.Handle(parser.GameEvent{Type: parser.EventPlayerName, PlayerID: 7, EntityName: "Moch#1358"})
	p.Handle(parser.GameEvent{
		Type: parser.EventGameEntityTags,
		Tags: map[string]string{"STATE": "RUNNING", "TURN": "16"},
	})
	if !p.isReconnect {
		t.Fatal("expected reconnect to be detected")
	}

	// After restore, SPELLS_CAST must still be 40 (not reset, not doubled).
	if ac := findAbilityCounter(m, CatSpellsCast); ac == nil || ac.Value != 40 {
		t.Fatalf("after reconnect restore expected SPELLS_CAST=40, got %v", ac)
	}

	// The engine re-emits 3809 at the same absolute value (40), then continues (43).
	// Absolute assignment is idempotent: 40 stays 40, then climbs to 43 — never 80.
	tag3809Change(p, 20, 40)
	if ac := findAbilityCounter(m, CatSpellsCast); ac == nil || ac.Value != 40 {
		t.Errorf("re-emitted 3809=40 after reconnect must keep SPELLS_CAST=40 (no double-count), got %v", ac)
	}
	tag3809Change(p, 20, 43)
	if ac := findAbilityCounter(m, CatSpellsCast); ac == nil || ac.Value != 43 {
		t.Errorf("post-reconnect 3809=43 must set SPELLS_CAST=43, got %v", ac)
	}
}
