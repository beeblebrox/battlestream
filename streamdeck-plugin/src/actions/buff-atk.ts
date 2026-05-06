import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.buff-atk' })
export class BuffAtkAction extends BaseStat {
  label = 'BUFF ATK';
  gradient = ['#3a1000', '#cb4335'] as const;
  extract(s: GameState) {
    const total = s.buff_sources.reduce((acc, b) => acc + b.attack, 0);
    return { value: `+${total}`, subtitle: '' };
  }
}
