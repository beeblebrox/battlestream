---
name: bs-counters
description: "Research, audit, and implement BG buff source counters using HDT and HearthDb as reference"
---

# bs-counters

Discover, audit, and implement Battlegrounds buff source counters and ability trackers by cross-referencing the HDT (Hearthstone Deck Tracker) counter system and HearthDb card database against our implementation.

## Usage

`/bs-counters audit` — Compare our counters against HDT's, find missing/outdated ones
`/bs-counters add <name>` — Research and implement a specific new counter
`/bs-counters update` — Pull latest HDT/HearthDb changes and check for new counters
`/bs-counters verify` — Run tests + reparse to validate all counters work end-to-end

## Architecture Overview

### Our counter system (`internal/gamestate/`)

Buff sources are tracked via two mechanisms:

1. **Player-level tags** (handled in `processor.go:updateBuffSourceFromPlayerTag`):
   - `BACON_BLOODGEMBUFF{ATK,HEALTH}VALUE` -> CatBloodgem (raw + 1, min 1)
   - `BACON_ELEMENTAL_BUFF{ATK,HEALTH}VALUE` -> CatElemental (max 0)
   - `TAVERN_SPELL_{ATTACK,HEALTH}_INCREASE` -> CatTavernSpell (raw value)

2. **Dnt enchantment TAG_CHANGE counters** (handled in `processor.go:handleDntTagChange`):
   - Each Dnt entity has a CardID that maps to a counter type
   - TAG_SCRIPT_DATA_NUM_1 = ATK component, TAG_SCRIPT_DATA_NUM_2 = HP component
   - **Differential** (value - prevValue): ShopBuff (Nomi), Nomi Sticker
   - **Absolute** (base + value): Beetle, Rightmost, Whelp, Volumizer, Barrage
   - **ATK-only**: Undead (SD1 only)

3. **Ability counters** (tag 3809 = Spellcraft stacks on player entity)

### Key files

| File | Purpose |
|---|---|
| `internal/gamestate/processor.go` | `handleDntTagChange()` dispatches SD changes to counter handlers |
| `internal/gamestate/categories.go` | Category constants, CardID->category maps, display names |
| `internal/gamestate/state.go` | `BuffSource`, `AbilityCounter`, `Enchantment` types; `SetBuffSource()`, `SetAbilityCounter()` |
| `internal/gamestate/gamestate_test.go` | Counter tests (`TestCounter*`) |
| `internal/tui/tui.go` | TUI display for BUFF SOURCES and ABILITIES panels |
| `proto/battlestream/v1/game.proto` | Proto definitions for BuffSource, AbilityCounter |

### Counter dispatch table (processor.go)

**Dnt enchantment counters (handleDntTagChange):**

| CardID | Category | Accumulation | Base |
|---|---|---|---|
| `BG_ShopBuff_Elemental` | NOMI | Differential | 0/0 |
| `BG30_MagicItem_544pe` | NOMI | Differential (SD1->both) | 0/0 |
| `BG31_808pe` | BEETLE | Absolute | 1/1 |
| `BG34_854pe` | RIGHTMOST | Absolute | 0/0 |
| `BG34_402pe` | WHELP | Absolute | 0/0 |
| `BG25_011pe` | UNDEAD | Absolute (SD1 only) | 0/0 |
| `BG34_170e` | VOLUMIZER | Absolute | 0/0 |
| `BG34_689e2` | BLOODGEM_BARRAGE | Absolute | 0/0 |

**Economy/ability counters (handleTagChange):**

| Tag/CardID | Category | Type | Display |
|---|---|---|---|
| tag `BACON_FREE_REFRESH_COUNT` | FREE_REFRESH | AbilityCounter | `"{raw}"` |
| tag `BACON_PLAYER_EXTRA_GOLD_NEXT_TURN` | GOLD_NEXT_TURN | AbilityCounter | `"{sure}"` or `"{sure} ({sure + overconf*3})"` |
| `BG28_884e` zone transitions | GOLD_NEXT_TURN | zone-tracked | +1 per PLAY enter, -1 per PLAY leave |
| tag `3809` | SPELLCRAFT | AbilityCounter | `"{1+(raw/4)} ({raw%4}/4)"` |

## Reference Repos

### HDT Counter Classes

Located at: `reference/Hearthstone-Deck-Tracker/Hearthstone Deck Tracker/Hearthstone/CounterSystem/`

**Base classes:**
- `BaseCounter.cs` — abstract base; defines `HandleTagChange(tag, gameState, entity, value, prevValue)`, `ShouldShow()`, `ValueToShow()`, `RelatedCards[]`
- `StatsCounter.cs` — extends BaseCounter with `AttackCounter`/`HealthCounter` (int pair)
- `NumericCounter.cs` — extends BaseCounter with single `Counter` (int)
- `CounterManager.cs` — discovers all counter subclasses via reflection, dispatches tag changes to all

**BG counter files** (`BgCounters/` subdirectory):

| HDT Class | Dnt CardID | Our Category | Notes |
|---|---|---|---|
| `ShopBuffStatsCounter` (ElementalTavernBuffStatsCounter.cs) | `BG_ShopBuff_Elemental` + `BG30_MagicItem_544pe` | NOMI | Differential; Sticker SD1->both |
| `BeetlesSizeCounter` | `BG31_808pe` | BEETLE | Absolute; base from card DB (1/1) |
| `RightMostTavernMinionBuffCounter` | `BG34_854pe` | RIGHTMOST | Absolute |
| `WhelpStatsBuffCounter` | `BG34_402pe` | WHELP | Absolute |
| `UndeadAttackBonusCounter` | `BG25_011pe` | UNDEAD | SD1 only |
| `VolumizerBuffCounter` | `BG34_170pe` | VOLUMIZER | Absolute |
| `BloodGemBarrageBuffCounter` | `BG34_689e2` | BLOODGEM_BARRAGE | Absolute |
| `BloodGemCounter` | player tags | BLOODGEM | BACON_BLOODGEMBUFF* |
| `ElementalExtraStatsCounter` | player tags | ELEMENTAL | BACON_ELEMENTAL_BUFF* |
| `TavernSpellsBuffCounter` | player tags | TAVERN_SPELL | TAVERN_SPELL_*_INCREASE |
| `SpellsPlayedForNagasCounter` | tag 3809 | SPELLCRAFT | Ability counter |
| `FreeRefreshCounter` | `BACON_FREE_REFRESH_COUNT` | FREE_REFRESH | Economy; AbilityCounter |
| `GoldNextTurnCounter` | `BACON_PLAYER_EXTRA_GOLD_NEXT_TURN` + Overconfidence Dnt (`BG28_884e`) | GOLD_NEXT_TURN | Economy; AbilityCounter + zone tracking |

### HearthDb CardID Constants

Located at: `reference/HearthDb/HearthDb/`

**Key file for BG enchantment CardIDs:**
- `CardIds.NonCollectible.Neutral.cs` — all BG Dnt enchantment constants

**Naming conventions:**
- Player-level Dnt enchantments end with `PlayerEnchantDnt` or `PlayerEnchDnt`
- Per-minion enchantments end with `Enchantment` or `Ench`
- BG cards prefixed with `BG` followed by set number (BG25=Undead, BG30=Stickers, BG31=Beetles, BG34=current)

**Useful search patterns in HearthDb:**
```
# Find all BG player-level Dnt enchantments
grep -i "PlayerEnchantDnt\|PlayerEnchDnt" reference/HearthDb/HearthDb/CardIds.NonCollectible.Neutral.cs

# Find all BG ShopBuff variants (per-tribe Nomi Dnts)
grep "ShopBuff.*Dnt" reference/HearthDb/HearthDb/CardIds.NonCollectible.Neutral.cs

# Find BG card by partial name
grep -i "beetle\|volumizer\|whelp" reference/HearthDb/HearthDb/CardIds.NonCollectible.Neutral.cs
```

## Procedures

### Audit: Find missing/changed counters

1. List all HDT BgCounter classes:
   ```
   ls reference/Hearthstone-Deck-Tracker/Hearthstone\ Deck\ Tracker/Hearthstone/CounterSystem/BgCounters/
   ```

2. For each HDT counter, read the class and extract:
   - The Dnt CardID it watches (from `RelatedCards` or entity CardId checks in `HandleTagChange`)
   - The accumulation method (differential vs absolute)
   - The `ShouldShow()` conditions
   - The `ValueToShow()` format

3. Cross-reference with `categoryByEnchantmentCardID` in `categories.go` and the `handleDntTagChange` switch in `processor.go`

4. Check HearthDb for NEW Dnt enchantments not yet in HDT:
   ```
   grep "PlayerEnch.*Dnt\|PlayerEnchantDnt" reference/HearthDb/HearthDb/CardIds.NonCollectible.Neutral.cs | grep -i "BG"
   ```

5. Report: which counters are missing, which have wrong accumulation, which CardIDs changed

### Add a new counter

1. Read the HDT counter class for the exact behavior
2. Find the Dnt CardID in HearthDb (`CardIds.NonCollectible.Neutral.cs`)
3. Add category constant to `categories.go` (e.g. `CatFreeRefresh = "FREE_REFRESH"`)
4. Add CardID mapping to `categoryByEnchantmentCardID` in `categories.go`
5. Add display name to `CategoryDisplayName` in `categories.go`
6. Add case to `handleDntTagChange` switch in `processor.go` (or player tag handler if tag-based)
7. If new accumulation pattern needed, add handler method to `processor.go`
8. Add test in `gamestate_test.go` following `TestCounter*` pattern
9. Verify: `go test ./internal/gamestate/ -v -run TestCounter`
10. Build and reparse: `go build ./cmd/battlestream/ && echo "yes" | ./battlestream db-reset && ./battlestream reparse`
11. Check TUI: `./battlestream tui --dump --width 120`

### Update reference repos

```bash
cd reference/Hearthstone-Deck-Tracker && git pull && cd ../..
cd reference/HearthDb && git pull && cd ../..
```

Then run the audit procedure to find new counters.

### Verify all counters

```bash
# Unit tests
go test ./internal/gamestate/ -v -run "TestCounter|TestProcessorBuff|TestProcessorEnchant"

# Build
go build ./cmd/battlestream/

# Full reparse cycle
ps aux | grep battlestream | grep -v grep | awk '{print $2}' | xargs -r kill -9
echo "yes" | ./battlestream db-reset && ./battlestream reparse

# Start daemon and check TUI
./battlestream daemon > /tmp/battlestream-daemon.log 2>&1 &
sleep 1 && ./battlestream tui --dump --width 120

# Check via gRPC
grpcurl -plaintext 127.0.0.1:50051 battlestream.v1.BattlestreamService/GetCurrentGame | jq '.buffSources, .abilityCounters'

# Cleanup
ps aux | grep battlestream | grep -v grep | awk '{print $2}' | xargs -r kill -9
```

## HDT Pattern Reference

### Differential counter (Nomi-style)
```csharp
// HDT: ShopBuffStatsCounter
var buffValue = value - prevValue;
if(tag == TAG_SCRIPT_DATA_NUM_1 && entity.CardId == NomiStickerDnt) {
    AttackCounter += buffValue;
    HealthCounter += buffValue;  // SD1 applies to both
} else {
    if(tag == TAG_SCRIPT_DATA_NUM_1) AttackCounter += buffValue;
    if(tag == TAG_SCRIPT_DATA_NUM_2) HealthCounter += buffValue;
}
```
Our equivalent: `handleShopBuffDnt()` / `handleNomiStickerDnt()` in processor.go

### Absolute counter (Beetle-style)
```csharp
// HDT: BeetlesSizeCounter
if(tag == TAG_SCRIPT_DATA_NUM_1) AttackCounter = baseAttack + value;
if(tag == TAG_SCRIPT_DATA_NUM_2) HealthCounter = baseHealth + value;
```
Our equivalent: `handleAbsoluteDnt(category, isSD1, value, baseAtk, baseHp)` in processor.go

### Numeric counter (Undead-style)
```csharp
// HDT: UndeadAttackBonusCounter
if(tag == TAG_SCRIPT_DATA_NUM_1) Counter = value;
```
Our equivalent: direct `SetBuffSource(CatUndead, value, 0)` in handleDntTagChange

### Player tag counter (Bloodgem-style)
```csharp
// HDT: BloodGemCounter reads BACON_BLOODGEMBUFFATKVALUE/HEALTHVALUE from player entity
```
Our equivalent: `updateBuffSourceFromPlayerTag()` in processor.go

## Per-tribe ShopBuff Dnt CardIDs (from HearthDb)

The game creates separate ShopBuff Dnt enchantments per tribe. Currently we only track the Elemental one (Nomi). Full list:

| Tribe | Dnt CardID | HearthDb Constant |
|---|---|---|
| Elemental | `BG_ShopBuff_Elemental` | `ElementalShopBuffPlayerEnchantmentDnt` |
| Beast | `BG_ShopBuff_Beast` | `BeastShopBuffPlayerEnchantmentDnt` |
| Demon | `BG_ShopBuff_Demon` | `DemonShopBuffPlayerEnchantmentDnt` |
| Dragon | `BG_ShopBuff_Dragon` | `DragonShopBuffPlayerEnchantmentDnt` |
| Mech | `BG_ShopBuff_Mech` | `MechShopBuffPlayerEnchantmentDnt` |
| Murloc | `BG_ShopBuff_Murloc` | `MurlocShopBuffPlayerEnchantmentDnt` |
| Naga | `BG_ShopBuff_Naga` | `NagaShopBuffPlayerEnchantmentDnt` |
| Pirate | `BG_ShopBuff_Pirate` | `PirateShopBuffPlayerEnchantmentDnt` |
| Quilboar | `BG_ShopBuff_Quilboar` | `QuilboarShopBuffPlayerEnchantmentDnt` |
| Undead | `BG_ShopBuff_Undead` | `UndeadShopBuffPlayerEnchantmentDnt` |
| MultiRace | `BG_ShopBuff_MultiRace` | `MultiraceShopBuffPlayerEnchantmentDnt` |
| Generic | `BG_ShopBuff` | `ShopBuffPlayerEnchantDnt` |

HDT's `ShopBuffStatsCounter.RelatedCards` only includes `ElementalShopBuffPlayerEnchantmentDnt` — it treats ALL ShopBuff Dnts as contributing to the same Nomi counter because the ELEMENTAL Dnt is the one that accumulates for the Nomi/elemental tavern buff mechanic.

## Not-yet-implemented counters (candidates)

| Counter | Type | Tag/CardID | Notes |
|---|---|---|---|
| Timewarped Nomi | Unknown | `BG34_855pe` NomiKitchenDreamDnt | Never seen in logs; may not fire |
| Per-tribe ShopBuff | Differential | `BG_ShopBuff_{Beast,Demon,...}` | Only relevant with tribe-specific Nomi variants |
| Pirate Buffs | Player tag | `BACON_PIRATE_BUFF{ATK,HEALTH}VALUE` | Exists in HearthDb enums, not yet seen in HDT |
| Timewarped variants | Dnt SD | `BG34_Giant_*`, `BG34_Treasure_*` | Timewarped tavern enchantments |

## Related agent

The **buff-scout** agent (`.claude/agents/buff-scout.md`) handles broader discovery:
- Scraping live Power.log for untracked enchantments
- Mining HearthDb for new Dnt enchantments after patches
- Full cross-reference audits across HDT + HearthDb + our code
- Use it when new BG patches drop or when you see untracked buffs in game
