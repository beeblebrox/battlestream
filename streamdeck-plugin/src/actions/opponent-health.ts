import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.opponent-health' })
export class OpponentHealthAction extends BaseStat {
  label = 'OPP HP';
  gradient = ['#4a0070', '#8e44ad'] as const;
  extract(s: GameState) {
    if (!s.opponent || s.opponent.max_health === 0) return { value: '—', subtitle: '' };
    const hp = s.opponent.health - s.opponent.damage;
    return { value: String(hp), subtitle: `/ ${s.opponent.max_health}` };
  }
}
