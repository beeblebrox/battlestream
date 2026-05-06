import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.bloodgem-buff' })
export class BloodgemBuffAction extends BaseStat {
  label = 'BLOODGEM';
  gradient = ['#3a1a00', '#e67e22'] as const;
  extract(s: GameState) {
    const bs = s.buff_sources.find(b => b.category === 'BLOODGEM');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
