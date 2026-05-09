import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.volumizer-buff' })
export class VolumizerBuffAction extends BaseStat {
  label = 'VOLUMIZER';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'VOLUMIZER');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
