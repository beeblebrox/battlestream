import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.tavern-spell-buff' })
export class TavernSpellBuffAction extends BaseStat {
  label = 'TVN SPELL';
  gradient = ['#003030', '#1abc9c'] as const;
  extract(s: GameState) {
    const bs = s.buff_sources.find(b => b.category === 'TAVERN_SPELL');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
