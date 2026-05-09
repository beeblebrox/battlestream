import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.bg-barrage-buff' })
export class BgBarrageBuffAction extends BaseStat {
  label = 'BG BARRAGE';
  gradient = ['#1a1000', '#7a5000'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'BLOODGEM_BARRAGE');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
