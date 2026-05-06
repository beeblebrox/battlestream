import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.phase' })
export class PhaseAction extends BaseStat {
  label = 'PHASE';
  gradient = ['#1a0030', '#6c3483'] as const;
  extract(s: GameState) {
    const short: Record<string, string> = { RECRUIT: 'BUY', COMBAT: 'FIGHT', GAME_OVER: 'END' };
    return { value: short[s.phase] ?? s.phase, subtitle: '' };
  }
}
