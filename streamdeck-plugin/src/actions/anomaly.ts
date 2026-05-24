import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.anomaly' })
export class AnomalyAction extends BaseStat {
  label = 'ANOMALY';
  gradient = ['#1a1a1a', '#566573'] as const;
  extract(s: GameState) {
    const name = s.anomaly_name || 'None';
    if (name.length <= 10) return { value: name, subtitle: '' };
    // Wrap: first 10 chars on value line (no ellipsis — text continues below),
    // up to 10 chars on subtitle with ellipsis only if still more text remains.
    const v = name.slice(0, 10);
    const sub = name.length > 20 ? name.slice(10, 19) + '…' : name.slice(10);
    return { value: v, subtitle: sub };
  }
}
