import { BattlestreamClient } from '../client.js';
import type { GameState } from '../types.js';

const mockState: GameState = {
  game_id: 'g1', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: { name: 'H', hero_card_id: '', health: 40, max_health: 40,
    damage: 0, armor: 0, current_gold: 0, max_gold: 0,
    spell_power: 0, triple_count: 0, tavern_tier: 1, win_streak: 0, loss_streak: 0 },
  board: [], placement: 0, buff_sources: [], ability_counters: [],
  anomaly_name: '', is_duos: false,
};

// Minimal EventSource stub
class MockEventSource {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSED = 2;
  readyState = MockEventSource.OPEN;
  onmessage: ((e: { data: string }) => void) | null = null;
  onerror: ((e: unknown) => void) | null = null;
  onopen: (() => void) | null = null;
  close = jest.fn();
  simulateMessage(data: string) { this.onmessage?.({ data }); }
  simulateError() { this.onerror?.(new Error('connection failed')); }
}

let mockEs: MockEventSource;
const MockESConstructor = jest.fn((url: string, init?: unknown) => {
  mockEs = new MockEventSource();
  return mockEs;
});

let fetchMock: jest.Mock;

beforeEach(() => {
  jest.useFakeTimers();
  MockESConstructor.mockClear();
  fetchMock = jest.fn().mockResolvedValue({
    ok: true,
    json: async () => mockState,
  });
});

afterEach(() => {
  jest.useRealTimers();
});

test('connect() opens EventSource and fetches initial state', async () => {
  const client = new BattlestreamClient(
    { host: '127.0.0.1', port: 8080, apiKey: '' },
    { EventSourceImpl: MockESConstructor as any, fetchImpl: fetchMock }
  );
  client.connect();
  await Promise.resolve(); // flush fetch
  await Promise.resolve();
  expect(MockESConstructor).toHaveBeenCalledTimes(1);
  expect(fetchMock).toHaveBeenCalledWith(
    'http://127.0.0.1:8080/v1/game/current',
    expect.objectContaining({ headers: {} })
  );
  client.disconnect();
});

test('connect() includes Authorization header when apiKey is set', async () => {
  const client = new BattlestreamClient(
    { host: '127.0.0.1', port: 8080, apiKey: 'tok' },
    { EventSourceImpl: MockESConstructor as any, fetchImpl: fetchMock }
  );
  client.connect();
  await Promise.resolve();
  await Promise.resolve();
  expect(fetchMock).toHaveBeenCalledWith(
    expect.any(String),
    expect.objectContaining({ headers: { Authorization: 'Bearer tok' } })
  );
  client.disconnect();
});

test('SSE message triggers state update callback', async () => {
  const onState = jest.fn();
  const client = new BattlestreamClient(
    { host: '127.0.0.1', port: 8080, apiKey: '' },
    { EventSourceImpl: MockESConstructor as any, fetchImpl: fetchMock, onState }
  );
  client.connect();
  await Promise.resolve();
  await Promise.resolve();
  // Simulate SSE event — should trigger another fetch
  mockEs.simulateMessage('ping');
  await Promise.resolve();
  await Promise.resolve();
  expect(fetchMock).toHaveBeenCalledTimes(2); // initial + SSE-triggered
  client.disconnect();
});

test('disconnect() closes EventSource', () => {
  const client = new BattlestreamClient(
    { host: '127.0.0.1', port: 8080, apiKey: '' },
    { EventSourceImpl: MockESConstructor as any, fetchImpl: fetchMock }
  );
  client.connect();
  client.disconnect();
  expect(mockEs.close).toHaveBeenCalled();
});
