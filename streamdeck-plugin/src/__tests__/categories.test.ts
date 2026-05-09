import { CATEGORY_META, DYNAMIC_CATEGORIES, TAVERN_WIDE_CATEGORIES } from '../categories.js';

test('TAVERN_WIDE_CATEGORIES has exactly 4 entries', () => {
  expect(TAVERN_WIDE_CATEGORIES.size).toBe(4);
  expect(TAVERN_WIDE_CATEGORIES.has('NOMI_ALL')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('TAVERN_SPELL')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('SHOP_BUFF')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('GENERAL')).toBe(true);
});

test('DYNAMIC_CATEGORIES excludes all TAVERN_WIDE categories', () => {
  for (const cat of TAVERN_WIDE_CATEGORIES) {
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(false);
  }
});

test('DYNAMIC_CATEGORIES includes all TARGETED categories', () => {
  expect(DYNAMIC_CATEGORIES.has('BLOODGEM')).toBe(true);
  expect(DYNAMIC_CATEGORIES.has('BLOODGEM_BARRAGE')).toBe(true);
  expect(DYNAMIC_CATEGORIES.has('RIGHTMOST')).toBe(true);
});

test('DYNAMIC_CATEGORIES includes all TYPE_BUFFS categories', () => {
  for (const cat of ['NOMI', 'ELEMENTAL', 'UNDEAD', 'LIGHTFANG', 'WHELP', 'BEETLE', 'VOLUMIZER', 'CONSUMED']) {
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(true);
  }
});

test('every CATEGORY_META entry has displayName, group, and gradient', () => {
  for (const [cat, meta] of Object.entries(CATEGORY_META)) {
    expect(typeof meta.displayName).toBe('string');
    expect(meta.gradient).toHaveLength(2);
    expect(['TARGETED', 'TYPE_BUFFS']).toContain(meta.group);
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(true);
  }
});
