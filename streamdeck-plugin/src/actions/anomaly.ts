import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.anomaly' })
export class AnomalyAction extends BaseStat {
  label = 'ANOMALY';
  gradient = ['#1a1a1a', '#566573'] as const;
  extract(s: GameState) {
    const name = s.anomaly_name || 'None';
    return { value: name.slice(0, 10), subtitle: name.length > 10 ? name.slice(10, 20) : '' };
  }
}
