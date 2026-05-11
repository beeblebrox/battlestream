package gamestate

import (
	"testing"
)

func TestCategoryGroup_AllCategoriesMapped(t *testing.T) {
	cases := []struct {
		cat   string
		group string
	}{
		{CatNomiAll, GroupTavernWide},
		{CatShopBuff, GroupTavernWide},
		{CatGeneral, GroupTavernWide},
		{CatBloodgem, GroupTargeted},
		{CatBloodgemBarrage, GroupTargeted},
		{CatRightmost, GroupTargeted},
		{CatNomi, GroupTypeBuffs},
		{CatElemental, GroupTypeBuffs},
		{CatUndead, GroupTypeBuffs},
		{CatLightfang, GroupTypeBuffs},
		{CatWhelp, GroupTypeBuffs},
		{CatBeetle, GroupTypeBuffs},
		{CatVolumizer, GroupTypeBuffs},
		{CatConsumed, GroupTypeBuffs},
		{CatTavernSpell, GroupTypeBuffs},
	}
	for _, tc := range cases {
		t.Run(tc.cat, func(t *testing.T) {
			got, ok := CategoryGroup[tc.cat]
			if !ok {
				t.Fatalf("CategoryGroup missing %q", tc.cat)
			}
			if got != tc.group {
				t.Errorf("CategoryGroup[%q] = %q, want %q", tc.cat, got, tc.group)
			}
		})
	}
}

func TestCategoryGroup_NoUnexpectedEntries(t *testing.T) {
	expected := map[string]string{
		CatNomiAll:         GroupTavernWide,
		CatShopBuff:        GroupTavernWide,
		CatGeneral:         GroupTavernWide,
		CatBloodgem:        GroupTargeted,
		CatBloodgemBarrage: GroupTargeted,
		CatRightmost:       GroupTargeted,
		CatNomi:            GroupTypeBuffs,
		CatElemental:       GroupTypeBuffs,
		CatUndead:          GroupTypeBuffs,
		CatLightfang:       GroupTypeBuffs,
		CatWhelp:           GroupTypeBuffs,
		CatBeetle:          GroupTypeBuffs,
		CatVolumizer:       GroupTypeBuffs,
		CatConsumed:        GroupTypeBuffs,
		CatTavernSpell:     GroupTypeBuffs,
	}
	if len(CategoryGroup) != len(expected) {
		t.Errorf("CategoryGroup has %d entries, want %d", len(CategoryGroup), len(expected))
	}
	for cat, group := range CategoryGroup {
		wantGroup, ok := expected[cat]
		if !ok {
			t.Errorf("CategoryGroup has unexpected key %q", cat)
			continue
		}
		if group != wantGroup {
			t.Errorf("CategoryGroup[%q] = %q, want %q", cat, group, wantGroup)
		}
	}
}
