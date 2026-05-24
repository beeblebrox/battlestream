import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.opponent-tavern-tier' })
export class OpponentTavernTierAction extends BaseStat {
  label = 'OPP TIER';
  gradient = ['#1a2a50', '#2980b9'] as const;
  extract(s: GameState) {
    if (!s.opponent || s.opponent.tavern_tier === 0) return { value: '—', subtitle: '' };
    return { value: String(s.opponent.tavern_tier), subtitle: '' };
  }
}
