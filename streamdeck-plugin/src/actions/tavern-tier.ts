import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.tavern-tier' })
export class TavernTierAction extends BaseStat {
  label = 'TIER';
  gradient = ['#1a3a00', '#27ae60'] as const;
  extract(s: GameState) { return { value: String(s.tavern_tier), subtitle: '' }; }
}
