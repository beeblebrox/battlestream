import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.whelp-buff' })
export class WhelpBuffAction extends BaseStat {
  label = 'WHELPS';
  gradient = ['#120a20', '#4a3070'] as const;
  extract(s: GameState) {
    const bs = (s.buff_sources ?? []).find(b => b.category === 'WHELP');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
