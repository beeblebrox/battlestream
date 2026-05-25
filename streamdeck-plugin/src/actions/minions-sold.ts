import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.minions-sold' })
export class MinionsSoldAction extends BaseStat {
  label = 'SOLD';
  gradient = ['#1a0a00', '#c0392b'] as const;
  extract(s: GameState) {
    const ac = (s.ability_counters ?? []).find(a => a.category === 'MINIONS_SOLD');
    return { value: ac ? String(ac.value) : '0', subtitle: '' };
  }
}
