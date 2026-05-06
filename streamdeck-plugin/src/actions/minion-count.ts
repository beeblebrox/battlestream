import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.minion-count' })
export class MinionCountAction extends BaseStat {
  label = 'MINIONS';
  gradient = ['#003030', '#1abc9c'] as const;
  extract(s: GameState) { return { value: String(s.board.length), subtitle: '/ 7' }; }
}
