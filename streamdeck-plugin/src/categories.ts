import path from 'node:path';

export type BuffGroup = 'TAVERN_WIDE' | 'TARGETED' | 'TYPE_BUFFS';

export interface CategoryMeta {
  displayName: string;
  group: BuffGroup;
  gradient: readonly [string, string];
  iconFile?: string;
  aggregateCategories?: string[];
}

export const TAVERN_WIDE_CATEGORIES = new Set([
  'NOMI_ALL', 'SHOP_BUFF', 'GENERAL',
]);

export const CATEGORY_META: Record<string, CategoryMeta> = {
  BLOODGEM:         { displayName: 'Bloodgems',  group: 'TARGETED',   gradient: ['#3a1a00', '#e67e22'], iconFile: 'bloodgem-buff.png' },
  BLOODGEM_BARRAGE: { displayName: 'BG Barrage', group: 'TARGETED',   gradient: ['#1a1000', '#7a5000'], iconFile: 'bg-barrage-buff.png' },
  RIGHTMOST:        { displayName: 'Rightmost',  group: 'TARGETED',   gradient: ['#1a1000', '#7a5000'], iconFile: 'rightmost-buff.png' },
  NOMI:             { displayName: 'Nomi',         group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'nomi-buff.png' },
  ELEMENTAL:        { displayName: 'Elementals',   group: 'TYPE_BUFFS', gradient: ['#3a2a00', '#f39c12'], iconFile: 'elemental-buff.png' },
  UNDEAD:           { displayName: 'Undead',       group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'undead-buff.png' },
  LIGHTFANG:        { displayName: 'Lightfang',    group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'lightfang-buff.png' },
  WHELP:            { displayName: 'Whelps',       group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'whelp-buff.png' },
  BEETLE:           { displayName: 'Beetles',      group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'beetle-buff.png' },
  VOLUMIZER:        { displayName: 'Volumizer',    group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'volumizer-buff.png' },
  CONSUMED:         { displayName: 'Consumed',     group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'], iconFile: 'consumed-buff.png' },
  TAVERN_SPELL:     { displayName: 'SPELL PWR',    group: 'TYPE_BUFFS', gradient: ['#4a004a', '#a93226'], iconFile: 'spell-power.png' },
  TAVERN_WIDE:      { displayName: 'TVN WIDE',     group: 'TAVERN_WIDE', gradient: ['#001a26', '#1a6b8a'], iconFile: 'tavern-wide-buff.png', aggregateCategories: ['NOMI_ALL', 'SHOP_BUFF', 'GENERAL'] },
};

export const DYNAMIC_CATEGORIES = new Set(Object.keys(CATEGORY_META));

export function categoryIconPath(cat: string): string | undefined {
  const file = CATEGORY_META[cat]?.iconFile;
  return file ? path.join(process.cwd(), 'imgs', 'actions', file) : undefined;
}
