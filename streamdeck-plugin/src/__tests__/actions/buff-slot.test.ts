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
    action: { id, setImage: jest.fn().mockResolvedValue(undefined) },
    payload: { coordinates: { row, column: col } },
  };
}

async function appear(inst: DynamicBuffSlotAction, ...actions: ReturnType<typeof makeAction>[]) {
  for (const a of actions) {
    await inst.onWillAppear(a as never);
  }
}

function makeState(
  sources: Array<{ category: string; attack: number; health: number }> = [],
  counters: Array<{ category: string; value: number }> = [],
): GameState {
  return {
    game_id: '', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
    player: {} as never, board: [], placement: 0,
    buff_sources: sources,
    ability_counters: counters.map(c => ({ category: c.category, value: c.value, display: String(c.value) })),
    anomaly_name: '', is_duos: false,
  };
}

describe('DynamicBuffSlotAction.assign() — buff sources', () => {
  test('assigns first active category to position-sorted first slot', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));
    inst.assign(makeState([{ category: 'UNDEAD', attack: 4, health: 4 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    expect(inst.getSlots().has('ctx-2')).toBe(false);
  });

  test('fills multiple slots in row-major position order', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));
    inst.assign(makeState([
      { category: 'UNDEAD', attack: 4, health: 4 },
      { category: 'NOMI',   attack: 2, health: 2 },
    ]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    expect(inst.getSlots().get('ctx-2')?.category).toBe('NOMI');
  });

  test('slot at row 1 is filled after all row 0 slots', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-row1', 1, 0), makeAction('ctx-row0', 0, 2));
    inst.assign(makeState([{ category: 'UNDEAD', attack: 4, health: 4 }]));
    expect(inst.getSlots().get('ctx-row0')?.category).toBe('UNDEAD');
    expect(inst.getSlots().has('ctx-row1')).toBe(false);
  });

  test('evicts least-recently-updated slot when all slots full', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([{ category: 'UNDEAD', attack: 4, health: 4 }]));
    inst.getSlots().get('ctx-1')!.lastUpdated = 1000;
    inst.assign(makeState([
      { category: 'UNDEAD', attack: 4, health: 4 },
      { category: 'NOMI',   attack: 2, health: 2 },
    ]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('NOMI');
  });

  test('clears slot when assigned category drops to 0', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([{ category: 'UNDEAD', attack: 4, health: 4 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    inst.assign(makeState([{ category: 'UNDEAD', attack: 0, health: 0 }]));
    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('raw TAVERN_WIDE source categories (NOMI_ALL, SHOP_BUFF) alone do not get their own slots', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([
      { category: 'NOMI_ALL',  attack: 6, health: 6 },
      { category: 'SHOP_BUFF', attack: 2, health: 2 },
    ]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('TAVERN_WIDE');
  });

  test('TAVERN_WIDE aggregate is assigned to a slot when any constituent is non-zero', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([{ category: 'NOMI_ALL', attack: 4, health: 4 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('TAVERN_WIDE');
  });

  test('TAVERN_WIDE aggregate is not assigned when all constituents are zero', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([{ category: 'NOMI_ALL', attack: 0, health: 0 }]));
    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('TAVERN_SPELL is assigned to a dynamic slot', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([{ category: 'TAVERN_SPELL', attack: 1, health: 2 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('TAVERN_SPELL');
  });

  test('null state clears all slot assignments', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([{ category: 'UNDEAD', attack: 4, health: 4 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('UNDEAD');
    inst.assign(null);
    expect(inst.getSlots().size).toBe(0);
  });
});

describe('DynamicBuffSlotAction.assign() — ability counters', () => {
  test('MINIONS_SOLD with value > 0 is assigned to a slot', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([], [{ category: 'MINIONS_SOLD', value: 6 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('MINIONS_SOLD');
  });

  test('MINIONS_SOLD with value 0 is not assigned', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([], [{ category: 'MINIONS_SOLD', value: 0 }]));
    expect(inst.getSlots().size).toBe(0);
  });

  test('MINIONS_SOLD slot is cleared when counter drops to 0', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([], [{ category: 'MINIONS_SOLD', value: 3 }]));
    expect(inst.getSlots().get('ctx-1')?.category).toBe('MINIONS_SOLD');
    inst.assign(makeState([], [{ category: 'MINIONS_SOLD', value: 0 }]));
    expect(inst.getSlots().has('ctx-1')).toBe(false);
  });

  test('MINIONS_SOLD and a buff source share the slot pool in position order', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0), makeAction('ctx-2', 0, 1));
    inst.assign(makeState(
      [{ category: 'UNDEAD', attack: 4, health: 4 }],
      [{ category: 'MINIONS_SOLD', value: 6 }],
    ));
    const assigned = new Set([
      inst.getSlots().get('ctx-1')?.category,
      inst.getSlots().get('ctx-2')?.category,
    ]);
    expect(assigned.has('UNDEAD')).toBe(true);
    expect(assigned.has('MINIONS_SOLD')).toBe(true);
  });

  test('unknown ability counter category is not assigned (no CATEGORY_META entry)', async () => {
    const inst = new DynamicBuffSlotAction();
    await appear(inst, makeAction('ctx-1', 0, 0));
    inst.assign(makeState([], [{ category: 'UNKNOWN_COUNTER', value: 5 }]));
    expect(inst.getSlots().size).toBe(0);
  });
});
