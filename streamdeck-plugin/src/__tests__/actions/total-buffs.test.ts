jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import type { GameState } from '../../types.js';
import { TotalBuffsAction } from '../../actions/total-buffs.js';

const base: GameState = {
  game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: {} as never, board: [], placement: 0,
  buff_sources: [], ability_counters: [], anomaly_name: '', is_duos: false,
};

describe('TotalBuffsAction', () => {
  test('returns — when buff_sources is absent', () => {
    const a = new TotalBuffsAction();
    const s = { ...base } as GameState;
    // Remove buff_sources entirely to simulate missing field
    delete (s as Partial<GameState>).buff_sources;
    const result = a.extract(s as GameState);
    expect(result.value).toBe('—');
    expect(result.subtitle).toBe('all buffs');
  });

  test('returns — when buff_sources is empty array', () => {
    const a = new TotalBuffsAction();
    const result = a.extract(base);
    expect(result.value).toBe('—');
    expect(result.subtitle).toBe('all buffs');
  });

  test('sums ATK and HP across two buff sources', () => {
    const a = new TotalBuffsAction();
    const s: GameState = {
      ...base,
      buff_sources: [
        { category: 'BLOODGEM', attack: 12, health: 15 },
        { category: 'UNDEAD',   attack: 16, health: 16 },
      ],
    };
    const result = a.extract(s);
    expect(result.value).toBe('+28/+31');
    expect(result.subtitle).toBe('all buffs');
  });

  test('single source: still sums correctly', () => {
    const a = new TotalBuffsAction();
    const s: GameState = {
      ...base,
      buff_sources: [{ category: 'NOMI', attack: 6, health: 8 }],
    };
    const result = a.extract(s);
    expect(result.value).toBe('+6/+8');
    expect(result.subtitle).toBe('all buffs');
  });
});
