jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import type { GameState } from '../../types.js';
import { TavernWideBuffAction } from '../../actions/tavern-wide-buff.js';

const base: GameState = {
  game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: {} as never, board: [], placement: 0,
  buff_sources: [], ability_counters: [], anomaly_name: '', is_duos: false,
};

describe('TavernWideBuffAction', () => {
  test('sums NOMI_ALL + TAVERN_SPELL + SHOP_BUFF + GENERAL, excludes others', () => {
    const a = new TavernWideBuffAction();
    const s: GameState = { ...base, buff_sources: [
      { category: 'NOMI_ALL',    attack: 4, health: 4 },
      { category: 'TAVERN_SPELL', attack: 8, health: 4 },
      { category: 'SHOP_BUFF',   attack: 2, health: 2 },
      { category: 'BLOODGEM',    attack: 3, health: 0 },
    ]};
    expect(a.extract(s).value).toBe('+14/+10');
  });

  test('returns +0/+0 when no tavern-wide sources present', () => {
    const a = new TavernWideBuffAction();
    expect(a.extract(base).value).toBe('+0/+0');
  });
});
