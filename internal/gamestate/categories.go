package gamestate

// Buff category constants for Battlegrounds buff source tracking.
const (
	CatBloodgem        = "BLOODGEM"
	CatBloodgemBarrage = "BLOODGEM_BARRAGE"
	CatNomi            = "NOMI"
	CatElemental       = "ELEMENTAL"
	CatTavernSpell     = "TAVERN_SPELL"
	CatWhelp           = "WHELP"
	CatBeetle          = "BEETLE"
	CatRightmost       = "RIGHTMOST"
	CatUndead          = "UNDEAD"
	CatVolumizer       = "VOLUMIZER"
	CatLightfang       = "LIGHTFANG"
	CatConsumed        = "CONSUMED"
	CatNomiAll         = "NOMI_ALL"
	CatSpellcraft      = "SPELLCRAFT"
	CatFreeRefresh     = "FREE_REFRESH"
	CatGoldNextTurn    = "GOLD_NEXT_TURN"
	CatGeneral         = "GENERAL"
)

// categoryByEnchantmentCardID maps exact enchantment CardIDs to categories.
// Values sourced from HearthDb.CardIds.NonCollectible.Neutral (reference/HearthDb/).
var categoryByEnchantmentCardID = map[string]string{
	// --- Player-level Dnt enchantments (running totals) ---
	"BG_ShopBuff_Elemental":   CatNomi,         // Nomi shop buff total
	"BG30_MagicItem_544pe":    CatNomi,          // Nomi Sticker
	"BGS_104pe":               CatNomi,          // NomiKitchenNightmare Dnt (regular Nomi)
	"BG34_855pe":              CatNomiAll,       // NomiKitchenDream Dnt (Timewarped Nomi - buffs ALL)
	"BG34_689e2":              CatBloodgemBarrage,
	"BG34_402pe":              CatWhelp,
	"BG31_808pe":              CatBeetle,
	"BG34_854pe":              CatRightmost,
	"BG25_011pe":              CatUndead,
	"BG34_170e":               CatVolumizer,

	// --- Per-minion enchantments (applied to board minions) ---
	// Nomi / elemental shop buffs
	"BG_ShopBuff_Ench":              CatNomi,      // TavernBuffed (per-minion Nomi)
	"BG_ShopBuff_Elemental_Ench":    CatNomi,      // Elemental Tavern Buffed (per-minion)
	// Elemental synergy
	"BG31_859e":                     CatElemental,  // Technical Element
	"BG31_816e":                     CatElemental,  // FireBaller
	"BG32_846e":                     CatElemental,  // Unleashed Mana Surge
	// Consumed / eaten minions
	"BG_Consumed":                   CatConsumed,
}

// categoryByCreatorCardID maps CREATOR entity CardIDs to categories.
// Used when the enchantment itself doesn't have a recognizable CardID.
var categoryByCreatorCardID = map[string]string{
	// Lightfang Enforcer variants
	"BGS_009":         CatLightfang,
	"TB_BaconUps_082": CatLightfang,
}

// nomiStickerCardIDs are enchantment CardIDs where TAG_SCRIPT_DATA_NUM_1
// applies to BOTH ATK and HP (not just ATK).
var nomiStickerCardIDs = map[string]bool{
	"BG30_MagicItem_544pe": true,
}

// ClassifyEnchantment returns the buff category for an enchantment CardID.
func ClassifyEnchantment(cardID string) string {
	if cat, ok := categoryByEnchantmentCardID[cardID]; ok {
		return cat
	}
	return CatGeneral
}

// ClassifyCreator returns the buff category based on the CREATOR entity's CardID.
func ClassifyCreator(creatorCardID string) (string, bool) {
	cat, ok := categoryByCreatorCardID[creatorCardID]
	return cat, ok
}

// IsNomiSticker returns true if the enchantment uses TAG_SCRIPT_DATA_NUM_1
// for both ATK and HP.
func IsNomiSticker(cardID string) bool {
	return nomiStickerCardIDs[cardID]
}

// playerTagCategory maps player-level tag names to their buff category.
var playerTagCategory = map[string]string{
	"BACON_BLOODGEMBUFFATKVALUE":      CatBloodgem,
	"BACON_BLOODGEMBUFFHEALTHVALUE":   CatBloodgem,
	"BACON_ELEMENTAL_BUFFATKVALUE":    CatElemental,
	"BACON_ELEMENTAL_BUFFHEALTHVALUE": CatElemental,
	"TAVERN_SPELL_ATTACK_INCREASE":    CatTavernSpell,
	"TAVERN_SPELL_HEALTH_INCREASE":    CatTavernSpell,
}

// playerTagIsATK returns true if the tag represents the ATK component of a buff.
var playerTagIsATK = map[string]bool{
	"BACON_BLOODGEMBUFFATKVALUE":      true,
	"BACON_BLOODGEMBUFFHEALTHVALUE":   false,
	"BACON_ELEMENTAL_BUFFATKVALUE":    true,
	"BACON_ELEMENTAL_BUFFHEALTHVALUE": false,
	"TAVERN_SPELL_ATTACK_INCREASE":    true,
	"TAVERN_SPELL_HEALTH_INCREASE":    false,
}

// ClassifyPlayerTag returns the buff category and whether it's the ATK component.
func ClassifyPlayerTag(tag string) (category string, isATK bool, ok bool) {
	cat, found := playerTagCategory[tag]
	if !found {
		return "", false, false
	}
	return cat, playerTagIsATK[tag], true
}

// ComputeBloodgemValue applies the +1 offset for bloodgem tags.
// Raw tag value 0 → effective +1, value 1 → effective +2, etc.
func ComputeBloodgemValue(rawValue int) int {
	v := rawValue + 1
	if v < 1 {
		return 1
	}
	return v
}

// ComputeElementalValue applies max(0, value) for elemental tags.
func ComputeElementalValue(rawValue int) int {
	if rawValue < 0 {
		return 0
	}
	return rawValue
}

// CategoryDisplayName returns a human-readable name for a buff category.
var CategoryDisplayName = map[string]string{
	CatBloodgem:        "Bloodgems",
	CatBloodgemBarrage: "BG Barrage",
	CatNomi:            "Nomi",
	CatElemental:       "Elementals",
	CatTavernSpell:     "Tavern Spells",
	CatWhelp:           "Whelps",
	CatBeetle:          "Beetles",
	CatRightmost:       "Rightmost",
	CatUndead:          "Undead",
	CatVolumizer:       "Volumizer",
	CatLightfang:       "Lightfang",
	CatNomiAll:         "Nomi Dream",
	CatSpellcraft:      "Spellcraft",
	CatFreeRefresh:     "Refreshes",
	CatGoldNextTurn:    "Bonus Gold",
	CatConsumed:        "Consumed",
	CatGeneral:         "General",
}
