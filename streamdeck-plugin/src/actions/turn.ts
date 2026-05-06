import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.turn' })
export class TurnAction extends BaseStat {
  label = 'TURN';
  gradient = ['#1a1a3a', '#5d6d7e'] as const;
  extract(s: GameState) { return { value: String(s.turn), subtitle: '' }; }
}
