jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import type { GameState } from '../../types.js';
import { OpponentHealthAction }     from '../../actions/opponent-health.js';
import { OpponentTavernTierAction } from '../../actions/opponent-tavern-tier.js';

const base: GameState = {
  game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: {} as never, board: [], placement: 0,
  buff_sources: [], ability_counters: [], anomaly_name: '', is_duos: false,
};

describe('OpponentHealthAction', () => {
  test('returns — when no opponent present', () => {
    const a = new OpponentHealthAction();
    const result = a.extract(base);
    expect(result.value).toBe('—');
    expect(result.subtitle).toBe('');
  });

  test('returns — when max_health is 0', () => {
    const a = new OpponentHealthAction();
    const s: GameState = {
      ...base,
      opponent: { name: 'Opp', hero_card_id: '', health: 0, max_health: 0,
        damage: 0, armor: 0, current_gold: 0, max_gold: 0, spell_power: 0,
        triple_count: 0, tavern_tier: 1, win_streak: 0, loss_streak: 0 },
    };
    const result = a.extract(s);
    expect(result.value).toBe('—');
    expect(result.subtitle).toBe('');
  });

  test('shows effective HP (health - damage) with max_health subtitle', () => {
    const a = new OpponentHealthAction();
    const s: GameState = {
      ...base,
      opponent: { name: 'Opp', hero_card_id: '', health: 25, max_health: 40,
        damage: 5, armor: 0, current_gold: 0, max_gold: 0, spell_power: 0,
        triple_count: 0, tavern_tier: 3, win_streak: 0, loss_streak: 0 },
    };
    const result = a.extract(s);
    expect(result.value).toBe('20');
    expect(result.subtitle).toBe('/ 40');
  });
});

describe('OpponentTavernTierAction', () => {
  test('returns — when no opponent present', () => {
    const a = new OpponentTavernTierAction();
    const result = a.extract(base);
    expect(result.value).toBe('—');
    expect(result.subtitle).toBe('');
  });

  test('returns — when tavern_tier is 0', () => {
    const a = new OpponentTavernTierAction();
    const s: GameState = {
      ...base,
      opponent: { name: 'Opp', hero_card_id: '', health: 30, max_health: 40,
        damage: 0, armor: 0, current_gold: 0, max_gold: 0, spell_power: 0,
        triple_count: 0, tavern_tier: 0, win_streak: 0, loss_streak: 0 },
    };
    expect(a.extract(s).value).toBe('—');
  });

  test('shows tier as string when tier is 4', () => {
    const a = new OpponentTavernTierAction();
    const s: GameState = {
      ...base,
      opponent: { name: 'Opp', hero_card_id: '', health: 30, max_health: 40,
        damage: 0, armor: 0, current_gold: 0, max_gold: 0, spell_power: 0,
        triple_count: 0, tavern_tier: 4, win_streak: 0, loss_streak: 0 },
    };
    expect(a.extract(s).value).toBe('4');
  });
});
