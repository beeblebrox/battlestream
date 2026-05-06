import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.health' })
export class HealthAction extends BaseStat {
  label = 'HEALTH';
  gradient = ['#7b0000', '#c0392b'] as const;
  extract(s: GameState) {
    const hp = s.player.health - s.player.damage;
    return { value: String(hp), subtitle: `/ ${s.player.max_health}` };
  }
}
