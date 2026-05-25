jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import type { GameState } from '../../types.js';
import { AvailableTribesAction } from '../../actions/tribes.js';

const base: GameState = {
  game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: {} as never, board: [], placement: 0,
  buff_sources: [], ability_counters: [], anomaly_name: '', is_duos: false,
};

describe('AvailableTribesAction', () => {
  test('no available_tribes field: value=0, subtitle=NONE', () => {
    const a = new AvailableTribesAction();
    const result = a.extract(base);
    expect(result.value).toBe('0');
    expect(result.subtitle).toBe('NONE');
  });

  test('empty tribes array: value=0, subtitle=NONE', () => {
    const a = new AvailableTribesAction();
    const s: GameState = { ...base, available_tribes: [] };
    const result = a.extract(s);
    expect(result.value).toBe('0');
    expect(result.subtitle).toBe('NONE');
  });

  test('three tribes: correct abbreviation join', () => {
    const a = new AvailableTribesAction();
    const s: GameState = { ...base, available_tribes: ['Murloc', 'Beast', 'Dragon'] };
    const result = a.extract(s);
    expect(result.value).toBe('3');
    expect(result.subtitle).toBe('MRL,BST,DRG');
  });

  test('six tribes: subtitle truncated at 14 chars with ellipsis', () => {
    const a = new AvailableTribesAction();
    const s: GameState = {
      ...base,
      available_tribes: ['Murloc', 'Beast', 'Naga', 'Undead', 'Dragon', 'Quilboar'],
    };
    const result = a.extract(s);
    expect(result.value).toBe('6');
    // Full would be: MRL,BST,NGA,UND,DRG,QLB (23 chars) — truncated to 13 + '…'
    expect(result.subtitle.length).toBe(14);
    expect(result.subtitle.endsWith('…')).toBe(true);
  });

  test('unknown tribe falls back to first 3 chars uppercased', () => {
    const a = new AvailableTribesAction();
    const s: GameState = { ...base, available_tribes: ['Fungus'] };
    const result = a.extract(s);
    expect(result.value).toBe('1');
    expect(result.subtitle).toBe('FUN');
  });
});

describe('TRIBE_ABBR spot-checks', () => {
  const action = new AvailableTribesAction();

  const cases: Array<[string, string]> = [
    ['Murloc',   'MRL'],
    ['Beast',    'BST'],
    ['Naga',     'NGA'],
    ['Undead',   'UND'],
    ['Dragon',   'DRG'],
    ['Quilboar', 'QLB'],
  ];

  test.each(cases)('%s abbreviates to %s', (tribe, expected) => {
    const s: GameState = { ...base, available_tribes: [tribe] };
    expect(action.extract(s).subtitle).toBe(expected);
  });
});
