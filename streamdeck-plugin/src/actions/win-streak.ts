import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.win-streak' })
export class WinStreakAction extends BaseStat {
  label = 'WIN STR.';
  gradient = ['#003366', '#2980b9'] as const;
  extract(s: GameState) { return { value: String(s.player.win_streak), subtitle: '' }; }
}
