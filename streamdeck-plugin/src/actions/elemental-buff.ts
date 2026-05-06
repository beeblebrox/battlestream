import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.elemental-buff' })
export class ElementalBuffAction extends BaseStat {
  label = 'ELEMENTAL';
  gradient = ['#3a2a00', '#f39c12'] as const;
  extract(s: GameState) {
    const bs = s.buff_sources.find(b => b.category === 'ELEMENTAL');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
