import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.total-buffs' })
export class TotalBuffsAction extends BaseStat {
  label = 'ALL BUFFS';
  gradient = ['#1a1a2e', '#8b0000'] as const;

  extract(s: GameState) {
    const sources = s.buff_sources;
    if (!sources || sources.length === 0) return { value: '—', subtitle: 'all buffs' };
    let atk = 0, hp = 0;
    for (const bs of sources) {
      atk += bs.attack;
      hp += bs.health;
    }
    if (atk === 0 && hp === 0) return { value: '—', subtitle: 'all buffs' };
    return { value: `+${atk}/+${hp}`, subtitle: 'all buffs' };
  }
}
