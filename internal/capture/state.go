package capture

import "time"

// CaptureState is a point-in-time snapshot of game state taken under lock.
type CaptureState struct {
	GameID        string    `json:"game_id"`
	Timestamp     time.Time `json:"timestamp"`
	Turn          int       `json:"turn"`
	Phase         string    `json:"phase"`
	TavernTier    int       `json:"tavern_tier"`
	Health        int       `json:"health"`
	Armor         int       `json:"armor"`
	Gold          int       `json:"gold"`
	Placement     int       `json:"placement"`
	IsDuos        bool      `json:"is_duos"`
	PartnerHealth int       `json:"partner_health,omitempty"`
	PartnerTier   int       `json:"partner_tier,omitempty"`
	Board         []MinionSnapshot     `json:"board"`
	BuffSources   []BuffSourceSnapshot `json:"buff_sources"`
}

// MinionSnapshot captures a single minion's state at capture time.
type MinionSnapshot struct {
	CardID     string `json:"card_id"`
	Name       string `json:"name"`
	Attack     int    `json:"attack"`
	Health     int    `json:"health"`
	Tribes     string `json:"tribes"`
	BuffAttack int    `json:"buff_attack"`
	BuffHealth int    `json:"buff_health"`
}

// BuffSourceSnapshot captures a buff source category total.
type BuffSourceSnapshot struct {
	Category string `json:"category"`
	Attack   int    `json:"attack"`
	Health   int    `json:"health"`
}
