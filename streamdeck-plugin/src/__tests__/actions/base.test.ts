import { store } from '../../state.js';
import type { GameState } from '../../types.js';

jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => 'data:image/png;base64,FAKE'),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import { renderButton } from '../../render.js';
import { BaseStat } from '../../actions/base.js';

const mockState: GameState = {
  game_id: 'g1', phase: 'RECRUIT', turn: 4, tavern_tier: 3,
  player: { name: 'Hero', hero_card_id: '', health: 40, max_health: 40,
    damage: 8, armor: 5, current_gold: 7, max_gold: 10, spell_power: 0,
    triple_count: 2, tavern_tier: 3, win_streak: 1, loss_streak: 0 },
  board: Array(5).fill(null),
  placement: 0, buff_sources: [{ category: 'BLOODGEM', attack: 3, health: 2 }],
  ability_counters: [], anomaly_name: 'Test Anomaly', is_duos: false,
};

function makeMockAction() {
  return { setImage: jest.fn().mockResolvedValue(undefined) };
}

class TestStat extends BaseStat {
  label = 'TEST';
  gradient: readonly [string, string] = ['#000', '#111'];
  extract(state: GameState) {
    return { value: String(state.turn), subtitle: '' };
  }
}

beforeEach(() => {
  store.setState(null);
  jest.clearAllMocks();
});

test('onWillAppear subscribes and renders current state', async () => {
  store.setState(mockState);
  const stat = new TestStat();
  const mockAction = makeMockAction();
  await stat.onWillAppear({ action: mockAction } as never);

  expect(renderButton).toHaveBeenCalledWith(
    expect.objectContaining({ label: 'TEST', value: '4', offline: false }),
  );
  expect(mockAction.setImage).toHaveBeenCalledWith('data:image/png;base64,FAKE');
});

test('onWillAppear with null state renders offline', async () => {
  const stat = new TestStat();
  const mockAction = makeMockAction();
  await stat.onWillAppear({ action: mockAction } as never);

  expect(renderButton).toHaveBeenCalledWith(
    expect.objectContaining({ offline: true }),
  );
});

test('state update re-renders all visible actions', async () => {
  const stat = new TestStat();
  const a1 = makeMockAction();
  const a2 = makeMockAction();
  await stat.onWillAppear({ action: a1 } as never);
  await stat.onWillAppear({ action: a2 } as never);

  jest.clearAllMocks();
  store.setState(mockState);
  await Promise.resolve();

  expect(a1.setImage).toHaveBeenCalled();
  expect(a2.setImage).toHaveBeenCalled();
});

test('onWillDisappear stops re-renders for that action', async () => {
  const stat = new TestStat();
  const a1 = makeMockAction();
  await stat.onWillAppear({ action: a1 } as never);
  await stat.onWillDisappear({ action: a1 } as never);

  jest.clearAllMocks();
  store.setState(mockState);
  await Promise.resolve();

  expect(a1.setImage).not.toHaveBeenCalled();
});
