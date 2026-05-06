import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.placement' })
export class PlacementAction extends BaseStat {
  label = 'PLACE';
  gradient = ['#00474a', '#16a085'] as const;
  extract(s: GameState) {
    const v = s.placement > 0 ? `#${s.placement}` : '—';
    return { value: v, subtitle: '' };
  }
}
