import { CATEGORY_META, DYNAMIC_CATEGORIES, TAVERN_WIDE_CATEGORIES, categoryIconPath } from '../categories.js';

test('TAVERN_WIDE_CATEGORIES has exactly 3 entries', () => {
  expect(TAVERN_WIDE_CATEGORIES.size).toBe(3);
  expect(TAVERN_WIDE_CATEGORIES.has('NOMI_ALL')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('SHOP_BUFF')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('GENERAL')).toBe(true);
  expect(TAVERN_WIDE_CATEGORIES.has('TAVERN_SPELL')).toBe(false);
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
  for (const cat of ['NOMI', 'ELEMENTAL', 'UNDEAD', 'LIGHTFANG', 'WHELP', 'BEETLE', 'VOLUMIZER', 'CONSUMED', 'TAVERN_SPELL']) {
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(true);
  }
});

test('every CATEGORY_META entry has displayName, group, and gradient', () => {
  for (const [cat, meta] of Object.entries(CATEGORY_META)) {
    expect(typeof meta.displayName).toBe('string');
    expect(meta.gradient).toHaveLength(2);
    expect(['TARGETED', 'TYPE_BUFFS', 'TAVERN_WIDE']).toContain(meta.group);
    expect(DYNAMIC_CATEGORIES.has(cat)).toBe(true);
  }
});

test('TAVERN_WIDE dynamic category aggregates NOMI_ALL + SHOP_BUFF + GENERAL', () => {
  const meta = CATEGORY_META['TAVERN_WIDE'];
  expect(meta).toBeDefined();
  expect(meta!.aggregateCategories).toEqual(expect.arrayContaining(['NOMI_ALL', 'SHOP_BUFF', 'GENERAL']));
  expect(meta!.aggregateCategories).toHaveLength(3);
});

test('DYNAMIC_CATEGORIES includes TAVERN_WIDE aggregate', () => {
  expect(DYNAMIC_CATEGORIES.has('TAVERN_WIDE')).toBe(true);
});

test('categoryIconPath returns a path ending with the iconFile for known icons', () => {
  const p = categoryIconPath('BLOODGEM');
  expect(p).toBeDefined();
  expect(p!.endsWith('bloodgem-buff.png')).toBe(true);
});

test('categoryIconPath returns a path for all CATEGORY_META entries with iconFile', () => {
  const withIcon = ['BLOODGEM', 'BLOODGEM_BARRAGE', 'RIGHTMOST', 'NOMI', 'ELEMENTAL',
    'UNDEAD', 'LIGHTFANG', 'WHELP', 'BEETLE', 'VOLUMIZER', 'CONSUMED', 'TAVERN_SPELL', 'TAVERN_WIDE'];
  for (const cat of withIcon) {
    expect(categoryIconPath(cat)).toBeDefined();
  }
});

test('categoryIconPath returns undefined for unknown categories', () => {
  expect(categoryIconPath('UNKNOWN_CATEGORY')).toBeUndefined();
});
