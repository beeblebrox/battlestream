import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

const TRIBE_ABBR: Record<string, string> = {
  Murloc:    'MRL',
  Beast:     'BST',
  Naga:      'NGA',
  Undead:    'UND',
  Dragon:    'DRG',
  Quilboar:  'QLB',
  Demon:     'DMN',
  Elemental: 'ELM',
  Mech:      'MCH',
  Pirate:    'PRT',
  All:       'ALL',
};

function abbr(tribe: string): string {
  return TRIBE_ABBR[tribe] ?? tribe.slice(0, 3).toUpperCase();
}

/** Max char length for the subtitle before truncation with '…'. */
const SUBTITLE_MAX = 14;

@action({ UUID: 'com.battlestream.streamdeck.tribes' })
export class AvailableTribesAction extends BaseStat {
  label = 'TRIBES';
  gradient = ['#1a1a2e', '#8e44ad'] as const;

  extract(s: GameState) {
    const tribes = s.available_tribes;
    if (!tribes || tribes.length === 0) {
      return { value: '0', subtitle: 'NONE' };
    }

    const value = String(tribes.length);
    const parts = tribes.map(abbr);
    const full = parts.join(',');

    let subtitle: string;
    if (full.length <= SUBTITLE_MAX) {
      subtitle = full;
    } else {
      // Truncate to SUBTITLE_MAX-1 chars and append '…'
      subtitle = full.slice(0, SUBTITLE_MAX - 1) + '…';
    }

    return { value, subtitle };
  }
}
