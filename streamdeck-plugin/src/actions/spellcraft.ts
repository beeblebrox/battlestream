import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.spellcraft' })
export class SpellcraftAction extends BaseStat {
  label = 'SPELLCRAFT';
  gradient = ['#2a0030', '#9b59b6'] as const;
  extract(s: GameState) {
    const ac = s.ability_counters.find(a => a.category === 'SPELLCRAFT');
    return { value: ac ? String(ac.value) : '0', subtitle: '' };
  }
}
