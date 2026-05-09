import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.beetle-buff' })
export class BeetleBuffAction extends BaseStat {
  label = 'BEETLES';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'BEETLE');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
