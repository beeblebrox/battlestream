import { store } from '../state.js';
import type { GameState } from '../types.js';

const mockState: GameState = {
  game_id: 'test-1',
  phase: 'RECRUIT',
  turn: 5,
  tavern_tier: 3,
  player: { name: 'Hero', hero_card_id: 'BG_Hero_01', health: 40, max_health: 40,
    damage: 0, armor: 0, current_gold: 7, max_gold: 10, spell_power: 0,
    triple_count: 0, tavern_tier: 3, win_streak: 0, loss_streak: 0 },
  board: [],
  placement: 0,
  buff_sources: [],
  ability_counters: [],
  anomaly_name: '',
  is_duos: false,
};

beforeEach(() => {
  store.setState(null); // reset between tests
});

test('initial state is null', () => {
  expect(store.getState()).toBeNull();
});

test('setState updates current state', () => {
  store.setState(mockState);
  expect(store.getState()).toEqual(mockState);
});

test('setState(null) clears state', () => {
  store.setState(mockState);
  store.setState(null);
  expect(store.getState()).toBeNull();
});

test('subscriber is called on setState', () => {
  const cb = jest.fn();
  const unsub = store.subscribe(cb);
  store.setState(mockState);
  expect(cb).toHaveBeenCalledWith(mockState);
  unsub();
});

test('unsubscribed listener is not called', () => {
  const cb = jest.fn();
  const unsub = store.subscribe(cb);
  unsub();
  store.setState(mockState);
  expect(cb).not.toHaveBeenCalled();
});

test('multiple subscribers all notified', () => {
  const cb1 = jest.fn();
  const cb2 = jest.fn();
  const u1 = store.subscribe(cb1);
  const u2 = store.subscribe(cb2);
  store.setState(mockState);
  expect(cb1).toHaveBeenCalledWith(mockState);
  expect(cb2).toHaveBeenCalledWith(mockState);
  u1(); u2();
});
