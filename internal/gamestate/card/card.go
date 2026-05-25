// Package card provides the Catalog interface and card-type classification
// used by the ActionBuilder to resolve entity card types during event processing.
package card

// CardType classifies the kind of card.
type CardType int

const (
	CardTypeUnknown   CardType = iota
	CardTypeMinion
	CardTypeSpell
	CardTypeHeroPower
	CardTypeHero
)

// CardSubType further classifies a card within its type.
type CardSubType int

const (
	CardSubTypeNone CardSubType = iota
	CardSubTypeTavernSpell
)

// Tribe enumerates Battlegrounds minion tribe affiliations.
type Tribe int

const (
	TribeNone      Tribe = iota
	TribeBeast
	TribeDemon
	TribeDragon
	TribeElemental
	TribeMech
	TribeMurloc
	TribeNaga
	TribePirate
	TribeQuillboar
	TribeUndead
	TribeAll
)

// CardInfo holds static data about a Hearthstone card, enriched from the
// catalog at action-build time. Tribes is always a deep copy.
type CardInfo struct {
	ID         string
	Name       string
	Text       string
	Type       CardType
	SubType    CardSubType
	Tribes     []Tribe
	BaseATK    int
	BaseHealth int
	Cost       int
}

// Catalog is a read-only lookup service. Safe to call from any goroutine
// (backed by static data; no mutable state).
type Catalog interface {
	// Lookup returns nil if the cardID is not found.
	Lookup(cardID string) *CardInfo
}
