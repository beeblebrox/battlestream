import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.spells-cast' })
export class SpellsCastAction extends BaseStat {
  label = 'SPELLS';
  gradient = ['#1a001a', '#8e44ad'] as const;
  extract(s: GameState) {
    const ac = (s.ability_counters ?? []).find(a => a.category === 'SPELLS_CAST');
    return { value: ac ? String(ac.value) : '0', subtitle: '' };
  }
}
