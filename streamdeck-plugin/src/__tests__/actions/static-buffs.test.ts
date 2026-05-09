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

import { BgBarrageBuffAction } from '../../actions/bg-barrage-buff.js';
import { RightmostBuffAction } from '../../actions/rightmost-buff.js';

describe('BgBarrageBuffAction', () => {
  test('returns +ATK/+HP for BLOODGEM_BARRAGE category', () => {
    const a = new BgBarrageBuffAction();
    const s: GameState = { ...base, buff_sources: [{ category: 'BLOODGEM_BARRAGE', attack: 3, health: 2 }] };
    expect(a.extract(s).value).toBe('+3/+2');
  });
  test('returns +0/+0 when category absent', () => {
    expect(new BgBarrageBuffAction().extract(base).value).toBe('+0/+0');
  });
});

describe('RightmostBuffAction', () => {
  test('returns +ATK/+HP for RIGHTMOST category', () => {
    const a = new RightmostBuffAction();
    const s: GameState = { ...base, buff_sources: [{ category: 'RIGHTMOST', attack: 2, health: 1 }] };
    expect(a.extract(s).value).toBe('+2/+1');
  });
  test('returns +0/+0 when category absent', () => {
    expect(new RightmostBuffAction().extract(base).value).toBe('+0/+0');
  });
});

import { NomiBuffAction }      from '../../actions/nomi-buff.js';
import { UndeadBuffAction }    from '../../actions/undead-buff.js';
import { LightfangBuffAction } from '../../actions/lightfang-buff.js';
import { WhelpBuffAction }     from '../../actions/whelp-buff.js';
import { BeetleBuffAction }    from '../../actions/beetle-buff.js';
import { VolumizerBuffAction } from '../../actions/volumizer-buff.js';
import { ConsumedBuffAction }  from '../../actions/consumed-buff.js';

type MakeAction = () => { extract(s: GameState): { value: string } };
const TYPE_BUFF_CASES: Array<[string, MakeAction, string]> = [
  ['NomiBuffAction',      () => new NomiBuffAction(),      'NOMI'],
  ['UndeadBuffAction',    () => new UndeadBuffAction(),    'UNDEAD'],
  ['LightfangBuffAction', () => new LightfangBuffAction(), 'LIGHTFANG'],
  ['WhelpBuffAction',     () => new WhelpBuffAction(),     'WHELP'],
  ['BeetleBuffAction',    () => new BeetleBuffAction(),    'BEETLE'],
  ['VolumizerBuffAction', () => new VolumizerBuffAction(), 'VOLUMIZER'],
  ['ConsumedBuffAction',  () => new ConsumedBuffAction(),  'CONSUMED'],
];

test.each(TYPE_BUFF_CASES)('%s returns +ATK/+HP for its category', (_name, makeAction, cat) => {
  const a = makeAction();
  const s: GameState = { ...base, buff_sources: [{ category: cat, attack: 4, health: 2 }] };
  expect(a.extract(s).value).toBe('+4/+2');
});

test.each(TYPE_BUFF_CASES)('%s returns +0/+0 when category absent', (_name, makeAction) => {
  expect(makeAction().extract(base).value).toBe('+0/+0');
});
