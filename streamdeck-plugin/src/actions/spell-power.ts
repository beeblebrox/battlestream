import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.spell-power' })
export class SpellPowerAction extends BaseStat {
  label = 'SPELL PWR';
  gradient = ['#4a004a', '#a93226'] as const;
  extract(s: GameState) { return { value: String(s.player.spell_power), subtitle: '' }; }
}
