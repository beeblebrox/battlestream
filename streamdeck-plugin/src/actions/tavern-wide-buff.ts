import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import { TAVERN_WIDE_CATEGORIES } from '../categories.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.tavern-wide-buff' })
export class TavernWideBuffAction extends BaseStat {
  label = 'TVN WIDE';
  gradient = ['#001a26', '#1a6b8a'] as const;

  extract(s: GameState) {
    let atk = 0, hp = 0;
    for (const bs of s.buff_sources ?? []) {
      if (TAVERN_WIDE_CATEGORIES.has(bs.category)) {
        atk += bs.attack;
        hp += bs.health;
      }
    }
    return { value: `+${atk}/+${hp}`, subtitle: '' };
  }
}
