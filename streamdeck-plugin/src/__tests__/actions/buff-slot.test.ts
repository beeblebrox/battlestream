jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => Promise.resolve('data:image/png;base64,FAKE')),
}));
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));
jest.mock('../../state.js', () => ({
  store: { subscribe: jest.fn(() => () => {}), getState: jest.fn(() => null) },
}));

import { DynamicBuffSlotAction } from '../../actions/buff-slot.js';
import type { GameState } from '../../types.js';

function makeAction(id: string, row = 0, col = 0) {
  return {
    id,
    coordinates: { row, column: col },
    setImage: jest.fn().mockResolvedValue(undefined),
  };
}

async function appear(inst: DynamicBuffSlotAction, ...actions: ReturnType<typeof makeAction>[]) {
  for (const a of actions) {
    await inst.onWillAppear({ action: a } as never);
  }
}

function makeState(...sources: Array<{ category: string; attack: number; health: number }>): GameState {
  return {
    game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
    player: {} as never, board: [], placement: 0,
    buff_sources: sources, ability_counters: [], anomaly_name: '', is_duos: false,
  };
}

describe('DynamicBuffSlotAction.assign()', () => {
  test('assigns first active category to position-sorted first slot', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));
    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    expect(inst.getSlots().has('ctx-2')).toBe(false);
  });

  test('fills multiple slots in row-major position order', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));
    inst.assign(makeState(
      { category: 'UNDEAD', attack: 4, health: 4 },
      { category: 'NOMI',   attack: 2, health: 2 },
    ));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    expect(inst.getSlots().get('ctx-2')?.category).toBe('NOMI');
  });

  test('slot at row 1 is filled after all row 0 slots', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-row1', 1, 0), makeAction('ctx-row0', 0, 2));
    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    expect(inst.getSlots().get('ctx-row0')?.category).toBe('UNDEAD');
    expect(inst.getSlots().has('ctx-row1')).toBe(false);
  });

  test('evicts least-recently-updated slot when all slots full', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    inst.getSlots().get('ctx-1')!.lastUpdated = 1000;
    inst.assign(makeState(
      { category: 'UNDEAD', attack: 4, health: 4 },
      { category: 'NOMI',   attack: 2, health: 2 },
    ));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('NOMI');
  });

  test('clears slot when assigned category drops to 0', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    inst.assign(makeState({ category: 'UNDEAD', attack: 0, health: 0 }));
    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('TAVERN_WIDE categories are never assigned to slots', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState(
      { category: 'NOMI_ALL',     attack: 6, health: 6 },
      { category: 'TAVERN_SPELL', attack: 4, health: 2 },
      { category: 'SHOP_BUFF',    attack: 2, health: 2 },
    ));
    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('null state clears all slot assignments', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState({ category: 'UNDEAD', attack: 4, health: 4 }));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    inst.assign(null);
    expect(inst.getSlots().size).toBe(0);
  });
});
