import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.buff-hp' })
export class BuffHpAction extends BaseStat {
  label = 'BUFF HP';
  gradient = ['#3a003a', '#c0392b'] as const;
  extract(s: GameState) {
    const total = s.buff_sources.reduce((acc, b) => acc + b.health, 0);
    return { value: `+${total}`, subtitle: '' };
  }
}
