import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.loss-streak' })
export class LossStreakAction extends BaseStat {
  label = 'LOSS STR.';
  gradient = ['#4a2000', '#e67e22'] as const;
  extract(s: GameState) { return { value: String(s.player.loss_streak), subtitle: '' }; }
}
