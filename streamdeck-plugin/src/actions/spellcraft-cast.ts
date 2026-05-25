import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.spellcraft-cast' })
export class SpellcraftCastAction extends BaseStat {
  label = 'CRAFT';
  gradient = ['#0d001a', '#6c3483'] as const;
  extract(s: GameState) {
    const ac = (s.ability_counters ?? []).find(a => a.category === 'SPELLCRAFT_CAST');
    return { value: ac ? String(ac.value) : '0', subtitle: '' };
  }
}
