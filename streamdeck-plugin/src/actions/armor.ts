import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.armor' })
export class ArmorAction extends BaseStat {
  label = 'ARMOR';
  gradient = ['#3d0000', '#922b21'] as const;
  extract(s: GameState) { return { value: String(s.player.armor), subtitle: '' }; }
}
