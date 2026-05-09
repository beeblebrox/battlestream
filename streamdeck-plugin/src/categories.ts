import path from 'node:path';

export type BuffGroup = 'TAVERN_WIDE' | 'TARGETED' | 'TYPE_BUFFS';

export interface CategoryMeta {
  displayName: string;
  group: 'TARGETED' | 'TYPE_BUFFS';
  gradient: readonly [string, string];
  iconFile?: string;
}

export const TAVERN_WIDE_CATEGORIES = new Set([
  'NOMI_ALL', 'TAVERN_SPELL', 'SHOP_BUFF', 'GENERAL',
]);

export const CATEGORY_META: Record<string, CategoryMeta> = {
  BLOODGEM:         { displayName: 'Bloodgems',  group: 'TARGETED',   gradient: ['#3a1a00', '#e67e22'], iconFile: 'bloodgem-buff.png' },
  BLOODGEM_BARRAGE: { displayName: 'BG Barrage', group: 'TARGETED',   gradient: ['#1a1000', '#7a5000'] },
  RIGHTMOST:        { displayName: 'Rightmost',  group: 'TARGETED',   gradient: ['#1a1000', '#7a5000'] },
  NOMI:             { displayName: 'Nomi',       group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  ELEMENTAL:        { displayName: 'Elementals', group: 'TYPE_BUFFS', gradient: ['#3a2a00', '#f39c12'], iconFile: 'elemental-buff.png' },
  UNDEAD:           { displayName: 'Undead',     group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  LIGHTFANG:        { displayName: 'Lightfang',  group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  WHELP:            { displayName: 'Whelps',     group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  BEETLE:           { displayName: 'Beetles',    group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  VOLUMIZER:        { displayName: 'Volumizer',  group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
  CONSUMED:         { displayName: 'Consumed',   group: 'TYPE_BUFFS', gradient: ['#120a20', '#4a3070'] },
};

export const DYNAMIC_CATEGORIES = new Set(Object.keys(CATEGORY_META));

export function categoryIconPath(cat: string): string | undefined {
  const file = CATEGORY_META[cat]?.iconFile;
  return file ? path.join(process.cwd(), 'imgs', 'actions', file) : undefined;
}
