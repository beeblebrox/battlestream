import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.gold' })
export class GoldAction extends BaseStat {
  label = 'GOLD';
  gradient = ['#5c4a00', '#d4a017'] as const;
  extract(s: GameState) {
    return { value: String(s.player.current_gold), subtitle: `/ ${s.player.max_gold}` };
  }
}
