package builder_test

import (
	"testing"
	"time"

	"battlestream.fixates.io/internal/gamestate/action"
	"battlestream.fixates.io/internal/gamestate/builder"
	"battlestream.fixates.io/internal/gamestate/card"
	"battlestream.fixates.io/internal/parser"
)

// noopCatalog returns a Catalog that always returns nil (no card data).
func noopCatalog() card.Catalog {
	return card.NewFuncCatalog(func(id string) string { return "" })
}

// turnEvent builds an EventTurnStart with a TURN tag.
func turnEvent(turn int) parser.GameEvent {
	return parser.GameEvent{
		Type: parser.EventTurnStart,
		Tags: map[string]string{
			"TURN": intStr(turn),
		},
	}
}

// intStr converts an int to a string without importing fmt.
func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if negative {
		digits = append([]byte{'-'}, digits...)
	}
	return string(digits)
}

// ── Scenario 1: buildGameStart ────────────────────────────────────────────────

func TestBuildGameStart_NonZeroTimestamp(t *testing.T) {
	b := builder.New(noopCatalog())
	ts := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	e := parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: ts,
	}
	got := b.Build(e)
	a, ok := got.(*action.GameStartAction)
	if !ok {
		t.Fatalf("expected *action.GameStartAction, got %T", got)
	}
	want := "game-" + intStr(int(ts.UnixMilli()))
	if a.GameID != want {
		t.Errorf("GameID = %q, want %q", a.GameID, want)
	}
	if a.Timestamp != ts {
		t.Errorf("Timestamp = %v, want %v", a.Timestamp, ts)
	}
}

func TestBuildGameStart_ZeroTimestamp(t *testing.T) {
	b := builder.New(noopCatalog())
	e := parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: time.Time{}, // zero
	}
	got := b.Build(e)
	a, ok := got.(*action.GameStartAction)
	if !ok {
		t.Fatalf("expected *action.GameStartAction, got %T", got)
	}
	if a.GameID != "" {
		t.Errorf("GameID = %q, want empty string for zero timestamp", a.GameID)
	}
}

func TestBuildGameStart_ResetsEntityCards(t *testing.T) {
	b := builder.New(noopCatalog())

	// Seed a card into the entity cache by building an event with CardID+EntityID.
	b.Build(parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: time.Now(),
	})
	// Build a tag-change that would cache entity 42 → "OLD_CARD"
	b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 42,
		CardID:   "OLD_CARD",
		Tags:     map[string]string{"BACON_FREE_REFRESH_COUNT": "1"},
	})

	// Start a new game — must clear the cache.
	b.Build(parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: time.Now(),
	})

	// After game start the builder phase is PhaseIdle, so a tag-change will
	// return nil regardless. Advance to recruit phase first.
	b.Build(turnEvent(1)) // odd → PhaseRecruit

	// Build a tag-change for entity 42 WITHOUT a CardID.
	// If the cache was cleared, the resulting action's Card should only have
	// an ID that came from the event's CardID field — which is empty, so Card = nil.
	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 42,
		Tags:     map[string]string{"BACON_FREE_REFRESH_COUNT": "3"},
	})
	eco, ok := got.(*action.EconomyChangedAction)
	if !ok {
		t.Fatalf("expected *action.EconomyChangedAction, got %T", got)
	}
	if eco.Card != nil {
		t.Errorf("Card = %v after game-start reset, want nil", eco.Card)
	}
}

// ── Scenario 2: buildPlayerDef ────────────────────────────────────────────────

func TestBuildPlayerDef_RealPlayer(t *testing.T) {
	b := builder.New(noopCatalog())
	e := parser.GameEvent{
		Type:       parser.EventPlayerDef,
		PlayerID:   2,
		EntityName: "CoolPlayer#1234",
		Tags: map[string]string{
			"hi":                           "144115198130930503",
			"HERO_ENTITY":                  "77",
			"BACON_DUO_TEAMMATE_PLAYER_ID": "3",
			"TURN":                         "5",
			"RESOURCES":                    "10",
			"RESOURCES_USED":               "7",
		},
	}
	got := b.Build(e)
	a, ok := got.(*action.PlayerDefAction)
	if !ok {
		t.Fatalf("expected *action.PlayerDefAction, got %T", got)
	}
	if a.PlayerID != 2 {
		t.Errorf("PlayerID = %d, want 2", a.PlayerID)
	}
	if a.AccountHi != 144115198130930503 {
		t.Errorf("AccountHi = %d, want 144115198130930503", a.AccountHi)
	}
	if a.HeroEntityID != 77 {
		t.Errorf("HeroEntityID = %d, want 77", a.HeroEntityID)
	}
	if a.DuoTeammateID != 3 {
		t.Errorf("DuoTeammateID = %d, want 3", a.DuoTeammateID)
	}
	if a.InitialTurn != 5 {
		t.Errorf("InitialTurn = %d, want 5", a.InitialTurn)
	}
	if a.Resources != 10 {
		t.Errorf("Resources = %d, want 10", a.Resources)
	}
	if a.ResourcesUsed != 7 {
		t.Errorf("ResourcesUsed = %d, want 7", a.ResourcesUsed)
	}
	if a.BattleTag != "CoolPlayer#1234" {
		t.Errorf("BattleTag = %q, want \"CoolPlayer#1234\"", a.BattleTag)
	}
}

func TestBuildPlayerDef_BotPlayer(t *testing.T) {
	b := builder.New(noopCatalog())
	e := parser.GameEvent{
		Type:     parser.EventPlayerDef,
		PlayerID: 5,
		Tags:     map[string]string{},
	}
	got := b.Build(e)
	a, ok := got.(*action.PlayerDefAction)
	if !ok {
		t.Fatalf("expected *action.PlayerDefAction, got %T", got)
	}
	if a.AccountHi != 0 {
		t.Errorf("AccountHi = %d, want 0 for bot", a.AccountHi)
	}
}

// ── Scenario 3: buildTurnTransition phase logic ───────────────────────────────

func TestBuildTurnTransition_OddIsRecruit(t *testing.T) {
	b := builder.New(noopCatalog())
	got := b.Build(turnEvent(1))
	a, ok := got.(*action.TurnTransitionAction)
	if !ok {
		t.Fatalf("expected *action.TurnTransitionAction, got %T", got)
	}
	if a.NewPhase != action.PhaseRecruit {
		t.Errorf("NewPhase = %v, want PhaseRecruit for turn 1", a.NewPhase)
	}
	if a.GameEntityTurn != 1 {
		t.Errorf("GameEntityTurn = %d, want 1", a.GameEntityTurn)
	}
}

func TestBuildTurnTransition_EvenIsCombat(t *testing.T) {
	b := builder.New(noopCatalog())
	got := b.Build(turnEvent(2))
	a, ok := got.(*action.TurnTransitionAction)
	if !ok {
		t.Fatalf("expected *action.TurnTransitionAction, got %T", got)
	}
	if a.NewPhase != action.PhaseCombat {
		t.Errorf("NewPhase = %v, want PhaseCombat for turn 2", a.NewPhase)
	}
}

// After a combat turn transition, a buff-source tag-change must return nil.
func TestBuildTurnTransition_PhaseGateCombat(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(2)) // even → PhaseCombat

	got := b.Build(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"BACON_BLOODGEMBUFFATKVALUE": "5"},
	})
	if got != nil {
		t.Errorf("expected nil during combat phase for buff-source tag, got %T", got)
	}
}

// After a recruit turn transition, a buff-source tag-change must return an action.
func TestBuildTurnTransition_PhaseGateRecruit(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // odd → PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"BACON_BLOODGEMBUFFATKVALUE": "5"},
	})
	if got == nil {
		t.Fatal("expected non-nil action during recruit phase for buff-source tag")
	}
	if _, ok := got.(*action.PlayerTagChangedAction); !ok {
		t.Errorf("expected *action.PlayerTagChangedAction, got %T", got)
	}
}

// ── Scenario 4: buildTagChange — recruit phase, handled buff-source tags ──────

var buffSourceTags = []string{
	"BACON_BLOODGEMBUFFATKVALUE",
	"BACON_BLOODGEMBUFFHEALTHVALUE",
	"BACON_ELEMENTAL_BUFFATKVALUE",
	"BACON_ELEMENTAL_BUFFHEALTHVALUE",
	"TAVERN_SPELL_ATTACK_INCREASE",
	"TAVERN_SPELL_HEALTH_INCREASE",
}

func TestBuildTagChange_BuffSourceTagsRecruitPhase(t *testing.T) {
	for _, tag := range buffSourceTags {
		tag := tag
		t.Run(tag, func(t *testing.T) {
			b := builder.New(noopCatalog())
			b.Build(turnEvent(1)) // PhaseRecruit

			got := b.Build(parser.GameEvent{
				Type: parser.EventTagChange,
				Tags: map[string]string{tag: "3"},
			})
			a, ok := got.(*action.PlayerTagChangedAction)
			if !ok {
				t.Fatalf("expected *action.PlayerTagChangedAction, got %T", got)
			}
			if a.Tag != tag {
				t.Errorf("Tag = %q, want %q", a.Tag, tag)
			}
			if a.Value != 3 {
				t.Errorf("Value = %d, want 3", a.Value)
			}
		})
	}
}

// ── Scenario 5: buildTagChange — phase guard (combat) ────────────────────────

func TestBuildTagChange_BuffSourceTagsCombatPhase(t *testing.T) {
	for _, tag := range buffSourceTags {
		tag := tag
		t.Run(tag, func(t *testing.T) {
			b := builder.New(noopCatalog())
			b.Build(turnEvent(2)) // PhaseCombat

			got := b.Build(parser.GameEvent{
				Type: parser.EventTagChange,
				Tags: map[string]string{tag: "3"},
			})
			if got != nil {
				t.Errorf("expected nil during combat for tag %q, got %T", tag, got)
			}
		})
	}
}

// ── Scenario 6: BACON_PLAYER_EXTRA_GOLD_NEXT_TURN ────────────────────────────

func TestBuildTagChange_ExtraGold_Recruit(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 2,
		Tags:     map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "2"},
	})
	a, ok := got.(*action.EconomyChangedAction)
	if !ok {
		t.Fatalf("expected *action.EconomyChangedAction during recruit, got %T", got)
	}
	if a.Tag != "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN" {
		t.Errorf("Tag = %q, want BACON_PLAYER_EXTRA_GOLD_NEXT_TURN", a.Tag)
	}
	if a.Value != 2 {
		t.Errorf("Value = %d, want 2", a.Value)
	}
}

func TestBuildTagChange_ExtraGold_Combat(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(2)) // PhaseCombat

	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 2,
		Tags:     map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "1"},
	})
	a, ok := got.(*action.CombatEconomyEffectAction)
	if !ok {
		t.Fatalf("expected *action.CombatEconomyEffectAction during combat, got %T", got)
	}
	if a.Tag != "BACON_PLAYER_EXTRA_GOLD_NEXT_TURN" {
		t.Errorf("Tag = %q, want BACON_PLAYER_EXTRA_GOLD_NEXT_TURN", a.Tag)
	}
	if a.Value != 1 {
		t.Errorf("Value = %d, want 1", a.Value)
	}
}

func TestBuildTagChange_ExtraGold_NegativeClamped_Recruit(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "-1"},
	})
	a, ok := got.(*action.EconomyChangedAction)
	if !ok {
		t.Fatalf("expected *action.EconomyChangedAction, got %T", got)
	}
	if a.Value != 0 {
		t.Errorf("Value = %d, want 0 (clamped from -1)", a.Value)
	}
}

func TestBuildTagChange_ExtraGold_NegativeClamped_Combat(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(2)) // PhaseCombat

	got := b.Build(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "-1"},
	})
	a, ok := got.(*action.CombatEconomyEffectAction)
	if !ok {
		t.Fatalf("expected *action.CombatEconomyEffectAction, got %T", got)
	}
	if a.Value != 0 {
		t.Errorf("Value = %d, want 0 (clamped from -1)", a.Value)
	}
}

func TestBuildTagChange_ExtraGold_Idle_ReturnsNil(t *testing.T) {
	b := builder.New(noopCatalog())
	// No turn transition — phase is PhaseIdle.
	got := b.Build(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"BACON_PLAYER_EXTRA_GOLD_NEXT_TURN": "1"},
	})
	if got != nil {
		t.Errorf("expected nil during idle phase, got %T", got)
	}
}

// ── Scenario 7: Phase 7.3 tag-change actions ──────────────────────────────────

func TestBuildTagChange_PlayerTriples(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit (no phase guard for transition actions)

	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 2,
		Tags:     map[string]string{"PLAYER_TRIPLES": "5"},
	})
	a, ok := got.(*action.PlayerTriplesChangedAction)
	if !ok {
		t.Fatalf("expected *action.PlayerTriplesChangedAction, got %T", got)
	}
	if a.Value != 5 {
		t.Errorf("Value = %d, want 5", a.Value)
	}
	if a.ControllerID != 2 {
		t.Errorf("ControllerID = %d, want 2", a.ControllerID)
	}
}

func TestBuildTagChange_Resources(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 2,
		Tags:     map[string]string{"RESOURCES": "7"},
	})
	a, ok := got.(*action.GoldChangedAction)
	if !ok {
		t.Fatalf("expected *action.GoldChangedAction, got %T", got)
	}
	if a.Tag != "RESOURCES" {
		t.Errorf("Tag = %q, want \"RESOURCES\"", a.Tag)
	}
	if a.Value != 7 {
		t.Errorf("Value = %d, want 7", a.Value)
	}
}

func TestBuildTagChange_ResourcesUsed(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 2,
		Tags:     map[string]string{"RESOURCES_USED": "3"},
	})
	a, ok := got.(*action.GoldChangedAction)
	if !ok {
		t.Fatalf("expected *action.GoldChangedAction, got %T", got)
	}
	if a.Tag != "RESOURCES_USED" {
		t.Errorf("Tag = %q, want \"RESOURCES_USED\"", a.Tag)
	}
	if a.Value != 3 {
		t.Errorf("Value = %d, want 3", a.Value)
	}
}

func TestBuildTagChange_TavernTier(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type:     parser.EventTagChange,
		PlayerID: 2,
		Tags:     map[string]string{"PLAYER_TECH_LEVEL": "3"},
	})
	a, ok := got.(*action.TavernTierChangedAction)
	if !ok {
		t.Fatalf("expected *action.TavernTierChangedAction, got %T", got)
	}
	if a.Tag != "PLAYER_TECH_LEVEL" {
		t.Errorf("Tag = %q, want \"PLAYER_TECH_LEVEL\"", a.Tag)
	}
	if a.Tier != 3 {
		t.Errorf("Tier = %d, want 3", a.Tier)
	}
	if a.ControllerID != 2 {
		t.Errorf("ControllerID = %d, want 2", a.ControllerID)
	}
}

func TestBuildTagChange_TavernTier_Zero(t *testing.T) {
	b := builder.New(noopCatalog())
	b.Build(turnEvent(1)) // PhaseRecruit

	got := b.Build(parser.GameEvent{
		Type: parser.EventTagChange,
		Tags: map[string]string{"PLAYER_TECH_LEVEL": "0"},
	})
	// The builder must NOT return nil for zero — the zero guard lives in the visitor.
	a, ok := got.(*action.TavernTierChangedAction)
	if !ok {
		t.Fatalf("expected *action.TavernTierChangedAction (not nil), got %T", got)
	}
	if a.Tier != 0 {
		t.Errorf("Tier = %d, want 0", a.Tier)
	}
}

// ── Scenario 8: entityCards cache inheritance ─────────────────────────────────

func TestEntityCards_CardIDInheritedFromCache(t *testing.T) {
	b := builder.New(noopCatalog())

	// 1. Game start (resets cache).
	b.Build(parser.GameEvent{
		Type:      parser.EventGameStart,
		Timestamp: time.Now(),
	})

	// 2. Advance to recruit phase.
	b.Build(turnEvent(1))

	// 3. Event that seeds entity 42 → "BP_foo" in the cache.
	//    Use BACON_FREE_REFRESH_COUNT so the builder doesn't return nil.
	seedEv := parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 42,
		CardID:   "BP_foo",
		Tags:     map[string]string{"BACON_FREE_REFRESH_COUNT": "1"},
	}
	b.Build(seedEv)

	// 4. Later event: same EntityID, no CardID. Card.ID should be "BP_foo".
	laterEv := parser.GameEvent{
		Type:     parser.EventTagChange,
		EntityID: 42,
		// No CardID — must inherit from cache.
		Tags: map[string]string{"BACON_FREE_REFRESH_COUNT": "2"},
	}
	got := b.Build(laterEv)
	eco, ok := got.(*action.EconomyChangedAction)
	if !ok {
		t.Fatalf("expected *action.EconomyChangedAction, got %T", got)
	}
	if eco.Card == nil {
		t.Fatal("Card is nil; expected CardInfo with ID \"BP_foo\" inherited from cache")
	}
	if eco.Card.ID != "BP_foo" {
		t.Errorf("Card.ID = %q, want \"BP_foo\"", eco.Card.ID)
	}
}
