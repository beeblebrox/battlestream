package card

// NewFuncCatalog wraps a simple name-lookup function as a Catalog.
// The gamestate package uses this to bridge the existing CardName function
// without creating an import cycle.
func NewFuncCatalog(lookupName func(cardID string) string) Catalog {
	return &funcCatalog{lookupName: lookupName}
}

type funcCatalog struct {
	lookupName func(cardID string) string
}

func (c *funcCatalog) Lookup(cardID string) *CardInfo {
	if cardID == "" {
		return nil
	}
	name := c.lookupName(cardID)
	// Phase 0: name only. Text, Type, Tribes etc. populated in later phases.
	return &CardInfo{
		ID:   cardID,
		Name: name,
	}
}
