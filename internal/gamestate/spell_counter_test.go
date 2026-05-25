package gamestate

// spell_counter_test.go tests OnTavernSpellPlayed (SPELLS_CAST) and
// OnCombatTavernSpell (SPELLCRAFT_CAST) ability counter behavior.
//
// The dispatch path is:
//   ZONE=PLAY on a SPELL entity whose controllerID == localPlayerID
//     → PhaseRecruit  → OnTavernSpellPlayed  → SPELLS_CAST + 1
//     → PhaseCombat   → OnCombatTavernSpell  → SPELLCRAFT_CAST + 1
//
// Non-local SPELL entities (partner, opponents, bot controller) must be
// silently ignored and must NOT route to OnMinionBought.

import (
	"testing"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/parser"
)

// registerSpellEntity registers entity entityID in the entity registry as a
// SPELL with the given controller. This is the prerequisite for the ZONE=PLAY
// path to recognise it as a spell rather than a minion.
func registerSpellEntity(p *Processor, entityID int, controllerID int) {
	p.entityController[entityID] = controllerID
	p.entityProps[entityID] = &entityInfo{
		CardType: "SPELL",
		Zone:     "HAND",
	}
}

// spellZonePlay sends a TAG_CHANGE ZONE=PLAY for the given entity with the
// given controllerID, which is the canonical event that triggers spell
// counter dispatch.
func spellZonePlay(p *Processor, entityID int, controllerID int) {
	p.Handle(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: entityID,
		PlayerID: controllerID,
		Tags:     map[string]string{"ZONE": "PLAY"},
	})
}

// tavernSpellPlayed fires OnTavernSpellPlayed directly without going through
// the ZONE=PLAY path. Used to test the visitor method in isolation.
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

// ── SPELLS_CAST (recruit phase) ───────────────────────────────────────────────

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
// increments SPELLS_CAST to 1.
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
// accumulates to 3.
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

// TestSpellsCastViaZonePlay verifies end-to-end dispatch: a SPELL entity
// controlled by the local player transitioning ZONE=PLAY during recruit phase
// increments SPELLS_CAST.
func TestSpellsCastViaZonePlay(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a SPELL entity for the local player (controller=7).
	registerSpellEntity(p, 400, 7)
	spellZonePlay(p, 400, 7)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac == nil {
		t.Fatal("expected SPELLS_CAST after ZONE=PLAY on local SPELL entity in recruit phase, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLS_CAST=1, got %d", ac.Value)
	}
}

// TestSpellsCastNonLocalIgnored verifies that a SPELL entity controlled by a
// different player (e.g. partner or opponent) does NOT increment SPELLS_CAST.
func TestSpellsCastNonLocalIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a SPELL for opponent controller=15.
	registerSpellEntity(p, 401, 15)
	spellZonePlay(p, 401, 15)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("non-local SPELL should not increment SPELLS_CAST, got value=%d", ac.Value)
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

// TestSpellcraftCastViaZonePlay verifies end-to-end dispatch: a SPELL entity
// controlled by the local player transitioning ZONE=PLAY during combat phase
// increments SPELLCRAFT_CAST (not SPELLS_CAST).
func TestSpellcraftCastViaZonePlay(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Advance to combat phase (GameEntity TURN=2 is even = combat).
	p.Handle(parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{"TURN": "2"},
	})
	if m.State().Phase != PhaseCombat {
		t.Fatalf("expected COMBAT phase after GameEntity TURN=2, got %s", m.State().Phase)
	}

	// Register a SPELL entity for the local player (controller=7).
	registerSpellEntity(p, 600, 7)
	spellZonePlay(p, 600, 7)

	ac := findAbilityCounter(m, CatSpellcraftCast)
	if ac == nil {
		t.Fatal("expected SPELLCRAFT_CAST after ZONE=PLAY on local SPELL in combat phase, got nil")
	}
	if ac.Value != 1 {
		t.Errorf("expected SPELLCRAFT_CAST=1, got %d", ac.Value)
	}

	// SPELLS_CAST must remain absent — recruit spell and combat spell are independent.
	recruitAC := findAbilityCounter(m, CatSpellsCast)
	if recruitAC != nil {
		t.Errorf("SPELLS_CAST should not be set for a combat SPELL, got value=%d", recruitAC.Value)
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

// TestSpellsCastSystemSpellIgnored verifies that system spells (e.g.
// TB_BaconShop_UpdateDmgCap) which fire SETASIDE→PLAY during MAIN_START_TRIGGERS
// are NOT counted. Only spells that transition from HAND to PLAY are player-played.
func TestSpellsCastSystemSpellIgnored(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Register a SPELL entity that starts in SETASIDE (system card, never in hand).
	p.entityController[450] = 7
	p.entityProps[450] = &entityInfo{
		CardType: "SPELL",
		Zone:     "SETASIDE",
	}
	spellZonePlay(p, 450, 7)

	ac := findAbilityCounter(m, CatSpellsCast)
	if ac != nil {
		t.Errorf("system SPELL (SETASIDE→PLAY) must not increment SPELLS_CAST, got value=%d", ac.Value)
	}
}

// ── Cross-phase independence ──────────────────────────────────────────────────

// TestSpellCountersAreIndependent verifies that SPELLS_CAST and SPELLCRAFT_CAST
// accumulate independently: recruit spells only touch SPELLS_CAST and combat
// spells only touch SPELLCRAFT_CAST.
func TestSpellCountersAreIndependent(t *testing.T) {
	m, p := setupRecruitPhase(t)

	// Two recruit spells.
	tavernSpellPlayed(p, 700)
	tavernSpellPlayed(p, 701)

	// Three combat spells.
	combatTavernSpell(p, 800)
	combatTavernSpell(p, 801)
	combatTavernSpell(p, 802)

	recruit := findAbilityCounter(m, CatSpellsCast)
	if recruit == nil || recruit.Value != 2 {
		t.Errorf("expected SPELLS_CAST=2, got %v", recruit)
	}

	combat := findAbilityCounter(m, CatSpellcraftCast)
	if combat == nil || combat.Value != 3 {
		t.Errorf("expected SPELLCRAFT_CAST=3, got %v", combat)
	}
}
