# Stream Deck Plugin Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Stream Deck plugin that displays live Hearthstone Battlegrounds stats from the battlestream daemon, with one action per stat, zero configuration, and an auto-layout action that fills the whole profile.

**Architecture:** A TypeScript plugin using `@elgato/streamdeck` v2.1.0 lives in `streamdeck-plugin/` and connects to the battlestream REST/SSE API. A singleton `BattlestreamClient` drives an SSE connection; each SSE event debounces a re-fetch of `/v1/game/current`. A module-level `store` fans game state out to all active action instances, which each render a 144×144 canvas PNG via `@napi-rs/canvas` and push it with `setImage()`. A companion `--no-auth` daemon flag zeroes out the API key requirement for local use.

**Tech Stack:** Go (daemon flag), TypeScript 5.7, `@elgato/streamdeck` 2.1.0, `@napi-rs/canvas` 0.1.x, `eventsource` 3.x, Rollup 4, Jest 29 + ts-jest, Node.js 24+.

---

## JSON Response Reference

`GET /v1/game/current` returns (snake_case keys, Go `encoding/json`):

```json
{
  "game_id": "abc123",
  "phase": "RECRUIT",
  "turn": 8,
  "tavern_tier": 4,
  "placement": 0,
  "player": {
    "name": "...", "health": 40, "max_health": 40, "damage": 8,
    "armor": 5, "current_gold": 7, "max_gold": 10,
    "spell_power": 0, "triple_count": 2,
    "tavern_tier": 4, "win_streak": 3, "loss_streak": 0
  },
  "board": [{ "entity_id": 1, "name": "Murloc", "attack": 3, "health": 2 }],
  "buff_sources": [{ "category": "BLOODGEM", "attack": 3, "health": 2 }],
  "ability_counters": [{ "category": "SPELLCRAFT", "value": 3, "display": "3" }],
  "anomaly_name": "Naga Fin Soup",
  "is_duos": false
}
```

Effective health = `player.health - player.damage`. Health button shows `player.health - player.damage` as value and `/ player.max_health` as subtitle.

---

## File Map

### Modified
- `cmd/battlestream/main.go` — add `--no-auth` flag to `cmdDaemon()`
- `.github/workflows/ci.yml` — add `plugin` job

### Created (plugin)
- `streamdeck-plugin/package.json`
- `streamdeck-plugin/tsconfig.json`
- `streamdeck-plugin/jest.config.js`
- `streamdeck-plugin/rollup.config.mjs`
- `streamdeck-plugin/manifest.json`
- `streamdeck-plugin/.gitignore`
- `streamdeck-plugin/src/types.ts`
- `streamdeck-plugin/src/state.ts` + `src/__tests__/state.test.ts`
- `streamdeck-plugin/src/client.ts` + `src/__tests__/client.test.ts`
- `streamdeck-plugin/src/render.ts` + `src/__tests__/render.test.ts`
- `streamdeck-plugin/src/actions/base.ts` + `src/__tests__/actions/base.test.ts`
- `streamdeck-plugin/src/actions/{health,armor,tavern-tier,gold,triples,win-streak,loss-streak,placement,spell-power,turn,phase,minion-count,buff-atk,buff-hp,anomaly}.ts` (15 files)
- `streamdeck-plugin/src/actions/{bloodgem-buff,elemental-buff,spellcraft,tavern-spell-buff}.ts` (4 XL files)
- `streamdeck-plugin/src/actions/auto-layout.ts` + `src/__tests__/actions/auto-layout.test.ts`
- `streamdeck-plugin/src/plugin.ts`
- `streamdeck-plugin/ui/global-settings.html`
- `streamdeck-plugin/imgs/plugin-icon.png` (placeholder)
- `streamdeck-plugin/imgs/category.png` (placeholder)
- `streamdeck-plugin/imgs/actions/*.png` (20 placeholder action icons)
- `streamdeck-plugin/profiles/` (profile stubs — see Task 14)
- `scripts/build-plugin.sh`
- `Makefile`

---

## Task 1: Backend — `--no-auth` daemon flag

`withAuth` already skips auth when `apiKey == ""`. This task just adds a `--no-auth` flag that zeroes out the key at daemon startup.

**Files:**
- Modify: `cmd/battlestream/main.go` (inside `cmdDaemon()`)
- Modify: `internal/api/rest/server_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/api/rest/server_test.go`:

```go
func TestWithAuth_EmptyKey_AllowsAllRequests(t *testing.T) {
	// Server with empty key already bypasses auth — this is the --no-auth path.
	s := New(nil, "")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil) // no Authorization header
	rw := httptest.NewRecorder()
	handler(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rw.Code)
	}
}

func TestWithAuth_NonEmptyKey_RejectsUnauthenticated(t *testing.T) {
	s := New(nil, "secret")
	handler := s.withAuth(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest("GET", "/", nil)
	rw := httptest.NewRecorder()
	handler(rw, req)
	if rw.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rw.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify one fails (New(nil, ...) may panic — fix if needed)**

```bash
go test ./internal/api/rest/ -run TestWithAuth -v
```

If `New(nil, ...)` panics because it dereferences `grpc`, wrap the handler in a nil-safe way or pass a stub. If both tests already pass, skip to step 4.

- [ ] **Step 3: Add `--no-auth` flag to daemon command**

In `cmd/battlestream/main.go`, inside `cmdDaemon()`, add the flag and override the key:

```go
func cmdDaemon() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Start the battlestream background service",
		Long:  "Starts gRPC + REST + WebSocket servers, tails HS logs, and writes stat files.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(cfgFile)
			if err != nil {
				return fmt.Errorf("loading config: %w", err)
			}
			setupLogging(cfg.Logging)

			// --no-auth overrides any configured API key.
			if noAuth, _ := cmd.Flags().GetBool("no-auth"); noAuth {
				cfg.API.APIKey = ""
			}

			profile, err := cfg.GetProfile(profileFlag)
			// ... rest unchanged
```

After the `RunE` closing brace, bind the flag:

```go
	}
	cmd.Flags().Bool("no-auth", false, "Accept all requests without checking the API key (for local plugin use)")
	return cmd
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/rest/ -run TestWithAuth -v
go vet ./...
```

Expected: both `TestWithAuth_*` tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/battlestream/main.go internal/api/rest/server_test.go
git commit -m "feat(daemon): add --no-auth flag to bypass API key check"
```

---

## Task 2: Plugin scaffold

**Files:** All `streamdeck-plugin/` root config files.

- [ ] **Step 1: Create `streamdeck-plugin/.gitignore`**

```
node_modules/
dist/
*.js.map
```

- [ ] **Step 2: Create `streamdeck-plugin/package.json`**

```json
{
  "name": "@battlestream/streamdeck-plugin",
  "version": "1.0.0",
  "private": true,
  "type": "module",
  "scripts": {
    "build": "rollup -c",
    "test": "node --experimental-vm-modules node_modules/.bin/jest",
    "test:watch": "node --experimental-vm-modules node_modules/.bin/jest --watch"
  },
  "dependencies": {
    "@elgato/streamdeck": "^2.1.0",
    "@napi-rs/canvas": "^0.1.56",
    "eventsource": "^3.0.2"
  },
  "devDependencies": {
    "@rollup/plugin-node-resolve": "^15.3.0",
    "@rollup/plugin-typescript": "^12.1.0",
    "@types/jest": "^29.5.14",
    "@types/node": "^22.0.0",
    "jest": "^29.7.0",
    "rollup": "^4.30.0",
    "rollup-plugin-copy": "^3.5.0",
    "ts-jest": "^29.2.6",
    "typescript": "^5.7.0"
  }
}
```

- [ ] **Step 3: Create `streamdeck-plugin/tsconfig.json`**

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ES2022",
    "moduleResolution": "bundler",
    "strict": true,
    "esModuleInterop": true,
    "experimentalDecorators": true,
    "emitDecoratorMetadata": true,
    "outDir": "dist",
    "rootDir": "src",
    "declaration": true
  },
  "include": ["src/**/*.ts"],
  "exclude": ["node_modules", "dist", "src/**/*.test.ts"]
}
```

- [ ] **Step 4: Create `streamdeck-plugin/jest.config.js`**

```js
export default {
  preset: 'ts-jest/presets/default-esm',
  testEnvironment: 'node',
  testMatch: ['**/src/__tests__/**/*.test.ts'],
  transform: {
    '^.+\\.ts$': ['ts-jest', { useESM: true, tsconfig: { module: 'ES2022' } }],
  },
  moduleNameMapper: {
    '^(\\.{1,2}/.*)\\.js$': '$1',
  },
};
```

- [ ] **Step 5: Create `streamdeck-plugin/rollup.config.mjs`**

```js
import typescript from '@rollup/plugin-typescript';
import { nodeResolve } from '@rollup/plugin-node-resolve';
import copy from 'rollup-plugin-copy';

const DIST = 'dist/com.battlestream.streamdeck.sdPlugin';

export default {
  input: 'src/plugin.ts',
  output: {
    file: `${DIST}/bin/plugin.js`,
    format: 'esm',
    sourcemap: true,
  },
  external: ['@napi-rs/canvas'],
  plugins: [
    nodeResolve({ preferBuiltins: true }),
    typescript({ tsconfig: './tsconfig.json' }),
    copy({
      targets: [
        { src: 'manifest.json', dest: DIST },
        { src: 'ui', dest: DIST },
        { src: 'imgs', dest: DIST },
        { src: 'profiles', dest: DIST },
      ],
      hook: 'writeBundle',
    }),
  ],
};
```

- [ ] **Step 6: Create `streamdeck-plugin/manifest.json`**

```json
{
  "$schema": "https://schemas.elgato.com/streamdeck/plugins/manifest.json",
  "SDKVersion": 3,
  "Version": "1.0.0.0",
  "Name": "Battlestream",
  "Author": "battlestream",
  "Description": "Live Hearthstone Battlegrounds stats on your Stream Deck.",
  "Category": "Battlestream",
  "CategoryIcon": "imgs/category",
  "CodePath": "bin/plugin.js",
  "Icon": "imgs/plugin-icon",
  "UUID": "com.battlestream.streamdeck",
  "OS": [
    { "Platform": "mac", "MinimumVersion": "10.15" },
    { "Platform": "windows", "MinimumVersion": "10" }
  ],
  "Software": { "MinimumVersion": "7.1" },
  "Nodejs": { "Version": "24", "Debug": "enabled" },
  "PropertyInspectorPath": "ui/global-settings.html",
  "Profiles": [
    { "Name": "Battlestream Standard", "DeviceType": 0, "ReadOnly": false, "DontAutoSwitchWhenInstalled": false },
    { "Name": "Battlestream XL",       "DeviceType": 2, "ReadOnly": false, "DontAutoSwitchWhenInstalled": false },
    { "Name": "Battlestream Mini",     "DeviceType": 1, "ReadOnly": false, "DontAutoSwitchWhenInstalled": false },
    { "Name": "Battlestream Plus",     "DeviceType": 7, "ReadOnly": false, "DontAutoSwitchWhenInstalled": false }
  ],
  "Actions": [
    { "UUID": "com.battlestream.streamdeck.health",           "Name": "Health",           "Icon": "imgs/actions/health",           "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/health"}] },
    { "UUID": "com.battlestream.streamdeck.armor",            "Name": "Armor",            "Icon": "imgs/actions/armor",            "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/armor"}] },
    { "UUID": "com.battlestream.streamdeck.tavern-tier",      "Name": "Tavern Tier",      "Icon": "imgs/actions/tavern-tier",      "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/tavern-tier"}] },
    { "UUID": "com.battlestream.streamdeck.gold",             "Name": "Gold",             "Icon": "imgs/actions/gold",             "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/gold"}] },
    { "UUID": "com.battlestream.streamdeck.triples",          "Name": "Triples",          "Icon": "imgs/actions/triples",          "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/triples"}] },
    { "UUID": "com.battlestream.streamdeck.win-streak",       "Name": "Win Streak",       "Icon": "imgs/actions/win-streak",       "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/win-streak"}] },
    { "UUID": "com.battlestream.streamdeck.loss-streak",      "Name": "Loss Streak",      "Icon": "imgs/actions/loss-streak",      "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/loss-streak"}] },
    { "UUID": "com.battlestream.streamdeck.placement",        "Name": "Placement",        "Icon": "imgs/actions/placement",        "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/placement"}] },
    { "UUID": "com.battlestream.streamdeck.spell-power",      "Name": "Spell Power",      "Icon": "imgs/actions/spell-power",      "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/spell-power"}] },
    { "UUID": "com.battlestream.streamdeck.turn",             "Name": "Turn",             "Icon": "imgs/actions/turn",             "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/turn"}] },
    { "UUID": "com.battlestream.streamdeck.phase",            "Name": "Phase",            "Icon": "imgs/actions/phase",            "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/phase"}] },
    { "UUID": "com.battlestream.streamdeck.minion-count",     "Name": "Minion Count",     "Icon": "imgs/actions/minion-count",     "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/minion-count"}] },
    { "UUID": "com.battlestream.streamdeck.buff-atk",         "Name": "Buff ATK",         "Icon": "imgs/actions/buff-atk",         "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/buff-atk"}] },
    { "UUID": "com.battlestream.streamdeck.buff-hp",          "Name": "Buff HP",          "Icon": "imgs/actions/buff-hp",          "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/buff-hp"}] },
    { "UUID": "com.battlestream.streamdeck.anomaly",          "Name": "Anomaly",          "Icon": "imgs/actions/anomaly",          "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/anomaly"}] },
    { "UUID": "com.battlestream.streamdeck.bloodgem-buff",    "Name": "Bloodgem Buff",    "Icon": "imgs/actions/bloodgem-buff",    "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/bloodgem-buff"}] },
    { "UUID": "com.battlestream.streamdeck.elemental-buff",   "Name": "Elemental Buff",   "Icon": "imgs/actions/elemental-buff",   "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/elemental-buff"}] },
    { "UUID": "com.battlestream.streamdeck.spellcraft",       "Name": "Spellcraft",       "Icon": "imgs/actions/spellcraft",       "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/spellcraft"}] },
    { "UUID": "com.battlestream.streamdeck.tavern-spell-buff","Name": "Tavern Spell Buff","Icon": "imgs/actions/tavern-spell-buff","Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/tavern-spell-buff"}] },
    { "UUID": "com.battlestream.streamdeck.auto-layout",      "Name": "Auto-Layout",      "Icon": "imgs/actions/auto-layout",      "Controllers": ["Keypad"], "States": [{"Image": "imgs/actions/auto-layout"}] }
  ]
}
```

- [ ] **Step 7: Create placeholder icons**

Stream Deck requires PNG icons at 72×72 (standard) and 144×144 (2x). For now, create a script that generates solid-color placeholders:

```bash
cd streamdeck-plugin
mkdir -p imgs/actions

# Generate a 72x72 placeholder PNG using Node.js (no extra tools needed)
node -e "
const { createCanvas } = require('@napi-rs/canvas');
// We'll generate real icons in Task 13. For now, skip — SD will use default.
"
```

Placeholder approach: just create empty `imgs/` dirs and a `imgs/plugin-icon.png` and `imgs/category.png` using any image tool, or copy a 72×72 solid grey PNG manually. The Stream Deck software shows a default icon if the file is missing. **Create an `imgs/README.md`** noting that real icons should be 72×72 PNG, and that they will be generated when the plugin build produces canvas-rendered buttons (the per-key image is set dynamically via `setImage()`; the `imgs/actions/` icons are only used in the Stream Deck action list).

For CI purposes, create empty stub files:

```bash
cd streamdeck-plugin
mkdir -p imgs/actions
for name in plugin-icon category health armor tavern-tier gold triples win-streak \
  loss-streak placement spell-power turn phase minion-count buff-atk buff-hp \
  anomaly bloodgem-buff elemental-buff spellcraft tavern-spell-buff auto-layout; do
  touch imgs/actions/${name}.png
done
touch imgs/plugin-icon.png imgs/category.png
```

**Real icons** should be created separately (solid colour matching the button gradient) and committed before shipping.

- [ ] **Step 8: Install dependencies**

```bash
cd streamdeck-plugin
npm install
```

Expected: `node_modules/` populated, no errors.

- [ ] **Step 9: Commit**

```bash
git add streamdeck-plugin/
git commit -m "chore(streamdeck): scaffold plugin project (package.json, tsconfig, rollup, manifest)"
```

---

## Task 3: TypeScript types

**Files:**
- Create: `streamdeck-plugin/src/types.ts`

- [ ] **Step 1: Create `streamdeck-plugin/src/types.ts`**

```typescript
// Matches JSON field names from the battlestream REST API (snake_case).

export interface PlayerState {
  name: string;
  hero_card_id: string;
  health: number;
  max_health: number;
  damage: number;
  armor: number;
  current_gold: number;
  max_gold: number;
  spell_power: number;
  triple_count: number;
  tavern_tier: number;
  win_streak: number;
  loss_streak: number;
}

export interface MinionState {
  entity_id: number;
  card_id: string;
  name: string;
  attack: number;
  health: number;
  minion_type: string;
  buff_attack: number;
  buff_health: number;
}

export interface BuffSource {
  category: string;
  attack: number;
  health: number;
}

export interface AbilityCounter {
  category: string;
  value: number;
  display: string;
}

export interface GameState {
  game_id: string;
  phase: string;           // "RECRUIT" | "COMBAT" | "GAME_OVER"
  turn: number;
  tavern_tier: number;
  player: PlayerState;
  board: MinionState[];
  placement: number;       // 0 while game is live, 1–8 at game over
  buff_sources: BuffSource[];
  ability_counters: AbilityCounter[];
  anomaly_name: string;
  is_duos: boolean;
}

export interface ClientConfig {
  host: string;
  port: number;
  apiKey: string;
}

export interface GlobalSettings {
  host?: string;
  port?: number;
  apiKey?: string;
}
```

- [ ] **Step 2: Verify TypeScript compiles (no test needed for a type-only file)**

```bash
cd streamdeck-plugin
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add streamdeck-plugin/src/types.ts
git commit -m "feat(streamdeck): add TypeScript types matching battlestream REST API"
```

---

## Task 4: State store

**Files:**
- Create: `streamdeck-plugin/src/state.ts`
- Create: `streamdeck-plugin/src/__tests__/state.test.ts`

- [ ] **Step 1: Create test file**

```typescript
// streamdeck-plugin/src/__tests__/state.test.ts
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

test('getState returns null initially', () => {
  expect(store.getState()).toBeNull();
});

test('setState updates current state', () => {
  store.setState(mockState);
  expect(store.getState()).toBe(mockState);
});

test('subscribe listener is called on setState', () => {
  const fn = jest.fn();
  store.subscribe(fn);
  store.setState(mockState);
  expect(fn).toHaveBeenCalledWith(mockState);
  expect(fn).toHaveBeenCalledTimes(1);
});

test('unsubscribe prevents future calls', () => {
  const fn = jest.fn();
  const unsub = store.subscribe(fn);
  unsub();
  store.setState(mockState);
  expect(fn).not.toHaveBeenCalled();
});

test('multiple listeners are all notified', () => {
  const a = jest.fn();
  const b = jest.fn();
  store.subscribe(a);
  store.subscribe(b);
  store.setState(mockState);
  expect(a).toHaveBeenCalledWith(mockState);
  expect(b).toHaveBeenCalledWith(mockState);
});
```

- [ ] **Step 2: Run to confirm failure**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=state
```

Expected: fails with "Cannot find module '../state.js'".

- [ ] **Step 3: Create `streamdeck-plugin/src/state.ts`**

```typescript
import type { GameState } from './types.js';

type Listener = (state: GameState | null) => void;
const listeners = new Set<Listener>();
let current: GameState | null = null;

export const store = {
  getState(): GameState | null {
    return current;
  },
  setState(state: GameState | null): void {
    current = state;
    for (const fn of listeners) fn(state);
  },
  subscribe(fn: Listener): () => void {
    listeners.add(fn);
    return () => listeners.delete(fn);
  },
};
```

- [ ] **Step 4: Run tests**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=state
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/state.ts streamdeck-plugin/src/__tests__/state.test.ts
git commit -m "feat(streamdeck): state store with subscribe/unsubscribe fan-out"
```

---

## Task 5: SSE client

**Files:**
- Create: `streamdeck-plugin/src/client.ts`
- Create: `streamdeck-plugin/src/__tests__/client.test.ts`

- [ ] **Step 1: Create test file**

```typescript
// streamdeck-plugin/src/__tests__/client.test.ts
import { BattlestreamClient } from '../client.js';
import type { GameState } from '../types.js';

const mockState: GameState = {
  game_id: 'g1', phase: 'RECRUIT', turn: 1, tavern_tier: 1,
  player: { name: 'Hero', hero_card_id: '', health: 40, max_health: 40,
    damage: 0, armor: 0, current_gold: 10, max_gold: 10, spell_power: 0,
    triple_count: 0, tavern_tier: 1, win_streak: 0, loss_streak: 0 },
  board: [], placement: 0, buff_sources: [], ability_counters: [],
  anomaly_name: '', is_duos: false,
};

let mockFetch: jest.Mock;
let mockEsInstance: { onmessage: ((e: MessageEvent) => void) | null; onerror: ((e: Event) => void) | null; close: jest.Mock };
let MockEventSource: jest.Mock;

beforeEach(() => {
  jest.useFakeTimers();

  mockFetch = jest.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ game: mockState }),
  } as unknown as Response);
  global.fetch = mockFetch;

  mockEsInstance = { onmessage: null, onerror: null, close: jest.fn() };
  MockEventSource = jest.fn(() => mockEsInstance);
  // Inject mock EventSource into the module — we'll spy on the import in client.ts
  // by passing it via dependency injection in the constructor.
});

afterEach(() => {
  jest.useRealTimers();
  jest.restoreAllMocks();
});

test('connect fetches initial state immediately', async () => {
  const client = new BattlestreamClient({ EventSourceImpl: MockEventSource as never });
  const listener = jest.fn();
  client.connect({ host: '127.0.0.1', port: 8080, apiKey: '' }, listener);

  await Promise.resolve(); // flush fetch
  expect(mockFetch).toHaveBeenCalledWith(
    'http://127.0.0.1:8080/v1/game/current',
    expect.objectContaining({ headers: {} }),
  );
  expect(listener).toHaveBeenCalledWith(mockState);
});

test('connect includes Authorization header when apiKey is set', async () => {
  const client = new BattlestreamClient({ EventSourceImpl: MockEventSource as never });
  const listener = jest.fn();
  client.connect({ host: '127.0.0.1', port: 8080, apiKey: 'secret' }, listener);

  await Promise.resolve();
  expect(mockFetch).toHaveBeenCalledWith(
    expect.any(String),
    expect.objectContaining({ headers: { Authorization: 'Bearer secret' } }),
  );
});

test('SSE message debounces a state re-fetch', async () => {
  const client = new BattlestreamClient({ EventSourceImpl: MockEventSource as never });
  const listener = jest.fn();
  client.connect({ host: '127.0.0.1', port: 8080, apiKey: '' }, listener);
  await Promise.resolve(); // initial fetch

  mockFetch.mockClear();
  mockEsInstance.onmessage!(new MessageEvent('message', { data: '{}' }));
  mockEsInstance.onmessage!(new MessageEvent('message', { data: '{}' }));
  mockEsInstance.onmessage!(new MessageEvent('message', { data: '{}' }));

  jest.advanceTimersByTime(100);
  await Promise.resolve();
  expect(mockFetch).toHaveBeenCalledTimes(1); // debounced to 1
});

test('disconnect closes EventSource and cancels timers', () => {
  const client = new BattlestreamClient({ EventSourceImpl: MockEventSource as never });
  client.connect({ host: '127.0.0.1', port: 8080, apiKey: '' }, jest.fn());
  client.disconnect();
  expect(mockEsInstance.close).toHaveBeenCalled();
});

test('SSE error triggers listener(null) and schedules retry', async () => {
  const client = new BattlestreamClient({ EventSourceImpl: MockEventSource as never });
  const listener = jest.fn();
  client.connect({ host: '127.0.0.1', port: 8080, apiKey: '' }, listener);
  await Promise.resolve();
  listener.mockClear();

  mockEsInstance.onerror!(new Event('error'));
  expect(listener).toHaveBeenCalledWith(null);

  // Retry after 500ms
  expect(MockEventSource).toHaveBeenCalledTimes(1);
  jest.advanceTimersByTime(500);
  expect(MockEventSource).toHaveBeenCalledTimes(2);
});
```

- [ ] **Step 2: Run to confirm failure**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=client
```

Expected: fails with "Cannot find module '../client.js'".

- [ ] **Step 3: Create `streamdeck-plugin/src/client.ts`**

The `EventSourceImpl` option lets tests inject a mock; production uses the real `eventsource` package.

```typescript
import EventSource from 'eventsource';
import type { ClientConfig, GameState } from './types.js';

type Listener = (state: GameState | null) => void;

interface ClientOptions {
  EventSourceImpl?: typeof EventSource;
}

export class BattlestreamClient {
  private es?: InstanceType<typeof EventSource>;
  private retryMs = 500;
  private retryTimer?: ReturnType<typeof setTimeout>;
  private debounceTimer?: ReturnType<typeof setTimeout>;
  private listener?: Listener;
  private config?: ClientConfig;
  private readonly ESImpl: typeof EventSource;

  constructor(opts: ClientOptions = {}) {
    this.ESImpl = opts.EventSourceImpl ?? EventSource;
  }

  connect(config: ClientConfig, listener: Listener): void {
    this.config = config;
    this.listener = listener;
    this.openSSE();
  }

  reconnect(config: ClientConfig): void {
    this.disconnect();
    this.retryMs = 500;
    this.connect(config, this.listener!);
  }

  disconnect(): void {
    this.es?.close();
    clearTimeout(this.retryTimer);
    clearTimeout(this.debounceTimer);
    this.es = undefined;
  }

  private headers(): Record<string, string> {
    const key = this.config?.apiKey;
    return key ? { Authorization: `Bearer ${key}` } : {};
  }

  private baseUrl(): string {
    const { host, port } = this.config!;
    return `http://${host}:${port}`;
  }

  private async fetchState(): Promise<GameState | null> {
    try {
      const res = await fetch(`${this.baseUrl()}/v1/game/current`, {
        headers: this.headers(),
      });
      if (!res.ok) return null;
      const data = await res.json() as { game?: GameState };
      return data.game ?? null;
    } catch {
      return null;
    }
  }

  private openSSE(): void {
    const url = `${this.baseUrl()}/v1/events`;
    this.es = new this.ESImpl(url, { headers: this.headers() });

    this.fetchState().then(s => this.listener?.(s));

    this.es.onmessage = () => {
      clearTimeout(this.debounceTimer);
      this.debounceTimer = setTimeout(async () => {
        const state = await this.fetchState();
        this.listener?.(state);
      }, 100);
    };

    this.es.onerror = () => {
      this.es?.close();
      this.listener?.(null);
      this.retryTimer = setTimeout(() => {
        this.retryMs = Math.min(this.retryMs * 2, 30_000);
        this.openSSE();
      }, this.retryMs);
    };
  }
}
```

- [ ] **Step 4: Run tests**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=client
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/client.ts streamdeck-plugin/src/__tests__/client.test.ts
git commit -m "feat(streamdeck): SSE client with debounce, retry backoff, and DI for testing"
```

---

## Task 6: Button renderer

**Files:**
- Create: `streamdeck-plugin/src/render.ts`
- Create: `streamdeck-plugin/src/__tests__/render.test.ts`

- [ ] **Step 1: Create test file**

```typescript
// streamdeck-plugin/src/__tests__/render.test.ts
import { renderButton } from '../render.js';

test('returns a valid base64 PNG data URL', () => {
  const result = renderButton({
    label: 'HEALTH',
    value: '32',
    subtitle: '/ 40',
    gradient: ['#7b0000', '#c0392b'],
    offline: false,
  });
  expect(result).toMatch(/^data:image\/png;base64,/);
  expect(result.length).toBeGreaterThan(100);
});

test('offline flag uses desaturated gradient', () => {
  const online = renderButton({ label: 'HEALTH', value: '32', subtitle: '', gradient: ['#7b0000', '#c0392b'], offline: false });
  const offline = renderButton({ label: 'HEALTH', value: '32', subtitle: '', gradient: ['#7b0000', '#c0392b'], offline: true });
  // Different image bytes when offline
  expect(online).not.toEqual(offline);
});

test('empty subtitle produces no subtitle text region', () => {
  expect(() =>
    renderButton({ label: 'TURN', value: '8', subtitle: '', gradient: ['#1a1a3a', '#5d6d7e'], offline: false })
  ).not.toThrow();
});
```

Note: these tests run the real `@napi-rs/canvas` against the actual native binary. They verify structural correctness (data URL format), not pixel-perfect rendering.

- [ ] **Step 2: Run to confirm failure**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=render
```

Expected: fails with "Cannot find module '../render.js'".

- [ ] **Step 3: Create `streamdeck-plugin/src/render.ts`**

```typescript
import { createCanvas } from '@napi-rs/canvas';

export interface RenderOptions {
  label: string;
  value: string;
  subtitle: string;
  gradient: readonly [string, string];
  offline: boolean;
}

export function renderButton(opts: RenderOptions): string {
  const SIZE = 144;
  const canvas = createCanvas(SIZE, SIZE);
  const ctx = canvas.getContext('2d');

  const [c1, c2] = opts.offline ? (['#2a2a2a', '#444444'] as const) : opts.gradient;
  const grd = ctx.createLinearGradient(0, 0, SIZE, SIZE);
  grd.addColorStop(0, c1);
  grd.addColorStop(1, c2);
  ctx.fillStyle = grd;
  ctx.fillRect(0, 0, SIZE, SIZE);

  // Label — top 25%
  ctx.fillStyle = 'rgba(255,255,255,0.70)';
  ctx.font = 'bold 17px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(opts.label.toUpperCase(), SIZE / 2, 30);

  // Value — center
  ctx.fillStyle = '#ffffff';
  ctx.font = 'bold 52px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(opts.value, SIZE / 2, 82);

  // Subtitle — bottom 20%
  if (opts.subtitle) {
    ctx.fillStyle = 'rgba(255,255,255,0.55)';
    ctx.font = '14px sans-serif';
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(opts.subtitle, SIZE / 2, 122);
  }

  return `data:image/png;base64,${canvas.toBuffer('image/png').toString('base64')}`;
}
```

- [ ] **Step 4: Run tests**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=render
```

Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/render.ts streamdeck-plugin/src/__tests__/render.test.ts
git commit -m "feat(streamdeck): canvas button renderer (gradient + label/value/subtitle)"
```

---

## Task 7: BaseStat action class

**Files:**
- Create: `streamdeck-plugin/src/actions/base.ts`
- Create: `streamdeck-plugin/src/__tests__/actions/base.test.ts`

- [ ] **Step 1: Create test file**

```typescript
// streamdeck-plugin/src/__tests__/actions/base.test.ts
import { store } from '../../state.js';
import type { GameState } from '../../types.js';

// Mock the renderer so tests don't require native canvas
jest.mock('../../render.js', () => ({
  renderButton: jest.fn(() => 'data:image/png;base64,FAKE'),
}));
// Mock @elgato/streamdeck
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
}));

import { renderButton } from '../../render.js';
import { BaseStat } from '../../actions/base.js';
import type { RenderOptions } from '../../render.js';

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
```

- [ ] **Step 2: Run to confirm failure**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=base
```

Expected: fails with "Cannot find module '../../actions/base.js'".

- [ ] **Step 3: Create `streamdeck-plugin/src/actions/base.ts`**

```typescript
import { SingletonAction, type WillAppearEvent, type WillDisappearEvent, type Action } from '@elgato/streamdeck';
import { store } from '../state.js';
import { renderButton } from '../render.js';
import type { GameState } from '../types.js';

export abstract class BaseStat extends SingletonAction<Record<string, never>> {
  protected abstract label: string;
  protected abstract gradient: readonly [string, string];
  protected abstract extract(state: GameState): { value: string; subtitle: string };

  private readonly contexts = new Set<Action>();
  private unsub?: () => void;

  override async onWillAppear({ action }: WillAppearEvent<Record<string, never>>): Promise<void> {
    if (this.contexts.size === 0) {
      this.unsub = store.subscribe(state => void this.updateAll(state));
    }
    this.contexts.add(action);
    await this.renderOne(action, store.getState());
  }

  override async onWillDisappear({ action }: WillDisappearEvent<Record<string, never>>): Promise<void> {
    this.contexts.delete(action);
    if (this.contexts.size === 0) {
      this.unsub?.();
      this.unsub = undefined;
    }
  }

  private async updateAll(state: GameState | null): Promise<void> {
    await Promise.all([...this.contexts].map(a => this.renderOne(a, state)));
  }

  private async renderOne(action: Action, state: GameState | null): Promise<void> {
    const { value, subtitle } = state
      ? this.extract(state)
      : { value: '—', subtitle: 'OFFLINE' };
    const image = renderButton({
      label: this.label,
      value,
      subtitle,
      gradient: this.gradient,
      offline: state === null,
    });
    await action.setImage(image);
  }
}
```

- [ ] **Step 4: Run tests**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=base
```

Expected: all 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/actions/base.ts streamdeck-plugin/src/__tests__/actions/base.test.ts
git commit -m "feat(streamdeck): BaseStat action class with multi-context subscribe/render"
```

---

## Task 8: Core stat actions (15 files)

**Files:** Create all 15 files in `streamdeck-plugin/src/actions/`.

No per-action tests — `BaseStat` is the tested unit; each action only adds constants.

- [ ] **Step 1: Create all 15 stat action files**

**`health.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.health' })
export class HealthAction extends BaseStat {
  label = 'HEALTH';
  gradient = ['#7b0000', '#c0392b'] as const;
  extract(s: GameState) {
    const hp = s.player.health - s.player.damage;
    return { value: String(hp), subtitle: `/ ${s.player.max_health}` };
  }
}
```

**`armor.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.armor' })
export class ArmorAction extends BaseStat {
  label = 'ARMOR';
  gradient = ['#3d0000', '#922b21'] as const;
  extract(s: GameState) { return { value: String(s.player.armor), subtitle: '' }; }
}
```

**`tavern-tier.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.tavern-tier' })
export class TavernTierAction extends BaseStat {
  label = 'TIER';
  gradient = ['#1a3a00', '#27ae60'] as const;
  extract(s: GameState) { return { value: String(s.tavern_tier), subtitle: '' }; }
}
```

**`gold.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.gold' })
export class GoldAction extends BaseStat {
  label = 'GOLD';
  gradient = ['#5c4a00', '#d4a017'] as const;
  extract(s: GameState) {
    return { value: String(s.player.current_gold), subtitle: `/ ${s.player.max_gold}` };
  }
}
```

**`triples.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.triples' })
export class TriplesAction extends BaseStat {
  label = 'TRIPLES';
  gradient = ['#2d0060', '#8e44ad'] as const;
  extract(s: GameState) { return { value: String(s.player.triple_count), subtitle: '' }; }
}
```

**`win-streak.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.win-streak' })
export class WinStreakAction extends BaseStat {
  label = 'WIN STR.';
  gradient = ['#003366', '#2980b9'] as const;
  extract(s: GameState) { return { value: String(s.player.win_streak), subtitle: '' }; }
}
```

**`loss-streak.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.loss-streak' })
export class LossStreakAction extends BaseStat {
  label = 'LOSS STR.';
  gradient = ['#4a2000', '#e67e22'] as const;
  extract(s: GameState) { return { value: String(s.player.loss_streak), subtitle: '' }; }
}
```

**`placement.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.placement' })
export class PlacementAction extends BaseStat {
  label = 'PLACE';
  gradient = ['#00474a', '#16a085'] as const;
  extract(s: GameState) {
    const v = s.placement > 0 ? `#${s.placement}` : '—';
    return { value: v, subtitle: '' };
  }
}
```

**`spell-power.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.spell-power' })
export class SpellPowerAction extends BaseStat {
  label = 'SPELL PWR';
  gradient = ['#4a004a', '#a93226'] as const;
  extract(s: GameState) { return { value: String(s.player.spell_power), subtitle: '' }; }
}
```

**`turn.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.turn' })
export class TurnAction extends BaseStat {
  label = 'TURN';
  gradient = ['#1a1a3a', '#5d6d7e'] as const;
  extract(s: GameState) { return { value: String(s.turn), subtitle: '' }; }
}
```

**`phase.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.phase' })
export class PhaseAction extends BaseStat {
  label = 'PHASE';
  gradient = ['#1a0030', '#6c3483'] as const;
  extract(s: GameState) {
    const short: Record<string, string> = { RECRUIT: 'BUY', COMBAT: 'FIGHT', GAME_OVER: 'END' };
    return { value: short[s.phase] ?? s.phase, subtitle: '' };
  }
}
```

**`minion-count.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.minion-count' })
export class MinionCountAction extends BaseStat {
  label = 'MINIONS';
  gradient = ['#003030', '#1abc9c'] as const;
  extract(s: GameState) { return { value: String(s.board.length), subtitle: '/ 7' }; }
}
```

**`buff-atk.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.buff-atk' })
export class BuffAtkAction extends BaseStat {
  label = 'BUFF ATK';
  gradient = ['#3a1000', '#cb4335'] as const;
  extract(s: GameState) {
    const total = s.buff_sources.reduce((acc, b) => acc + b.attack, 0);
    return { value: `+${total}`, subtitle: '' };
  }
}
```

**`buff-hp.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.buff-hp' })
export class BuffHpAction extends BaseStat {
  label = 'BUFF HP';
  gradient = ['#3a003a', '#c0392b'] as const;
  extract(s: GameState) {
    const total = s.buff_sources.reduce((acc, b) => acc + b.health, 0);
    return { value: `+${total}`, subtitle: '' };
  }
}
```

**`anomaly.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.anomaly' })
export class AnomalyAction extends BaseStat {
  label = 'ANOMALY';
  gradient = ['#1a1a1a', '#566573'] as const;
  extract(s: GameState) {
    const name = s.anomaly_name || 'None';
    return { value: name.slice(0, 10), subtitle: name.length > 10 ? name.slice(10, 20) : '' };
  }
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd streamdeck-plugin
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add streamdeck-plugin/src/actions/
git commit -m "feat(streamdeck): all 15 core stat action classes"
```

---

## Task 9: XL bonus stat actions (4 files)

**Files:** `streamdeck-plugin/src/actions/{bloodgem-buff,elemental-buff,spellcraft,tavern-spell-buff}.ts`

- [ ] **Step 1: Create 4 XL action files**

**`bloodgem-buff.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.bloodgem-buff' })
export class BloodgemBuffAction extends BaseStat {
  label = 'BLOODGEM';
  gradient = ['#3a1a00', '#e67e22'] as const;
  extract(s: GameState) {
    const bs = s.buff_sources.find(b => b.category === 'BLOODGEM');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`elemental-buff.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.elemental-buff' })
export class ElementalBuffAction extends BaseStat {
  label = 'ELEMENTAL';
  gradient = ['#3a2a00', '#f39c12'] as const;
  extract(s: GameState) {
    const bs = s.buff_sources.find(b => b.category === 'ELEMENTAL');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

**`spellcraft.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.spellcraft' })
export class SpellcraftAction extends BaseStat {
  label = 'SPELLCRAFT';
  gradient = ['#2a0030', '#9b59b6'] as const;
  extract(s: GameState) {
    const ac = s.ability_counters.find(a => a.category === 'SPELLCRAFT');
    return { value: ac ? String(ac.value) : '0', subtitle: '' };
  }
}
```

**`tavern-spell-buff.ts`**
```typescript
import { action } from '@elgato/streamdeck';
import { BaseStat } from './base.js';
import type { GameState } from '../types.js';

@action({ UUID: 'com.battlestream.streamdeck.tavern-spell-buff' })
export class TavernSpellBuffAction extends BaseStat {
  label = 'TVN SPELL';
  gradient = ['#003030', '#1abc9c'] as const;
  extract(s: GameState) {
    const bs = s.buff_sources.find(b => b.category === 'TAVERN_SPELL');
    return { value: bs ? `+${bs.attack}/+${bs.health}` : '+0/+0', subtitle: '' };
  }
}
```

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd streamdeck-plugin
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 3: Commit**

```bash
git add streamdeck-plugin/src/actions/bloodgem-buff.ts streamdeck-plugin/src/actions/elemental-buff.ts \
  streamdeck-plugin/src/actions/spellcraft.ts streamdeck-plugin/src/actions/tavern-spell-buff.ts
git commit -m "feat(streamdeck): XL bonus stat actions (bloodgem, elemental, spellcraft, tavern-spell)"
```

---

## Task 10: Auto-Layout action

**Files:**
- Create: `streamdeck-plugin/src/actions/auto-layout.ts`
- Create: `streamdeck-plugin/src/__tests__/actions/auto-layout.test.ts`

- [ ] **Step 1: Create test file**

```typescript
// streamdeck-plugin/src/__tests__/actions/auto-layout.test.ts
jest.mock('@elgato/streamdeck', () => ({
  action: () => (cls: unknown) => cls,
  SingletonAction: class {},
  streamDeck: {
    profiles: { switchToProfile: jest.fn().mockResolvedValue(undefined) },
  },
}));

import { streamDeck } from '@elgato/streamdeck';
import { AutoLayoutAction } from '../../actions/auto-layout.js';

const cases: Array<[number, string]> = [
  [0, 'Battlestream Standard'],
  [1, 'Battlestream Mini'],
  [2, 'Battlestream XL'],
  [7, 'Battlestream Plus'],
  [99, 'Battlestream Standard'], // unknown → standard fallback
];

test.each(cases)('device type %i → profile "%s"', async (deviceType, expectedProfile) => {
  const action = new AutoLayoutAction();
  const mockEv = {
    action: { device: { type: deviceType, id: 'dev-1' } },
  };
  await action.onKeyDown(mockEv as never);

  expect(streamDeck.profiles.switchToProfile).toHaveBeenCalledWith(
    'dev-1',
    expectedProfile,
  );
  jest.clearAllMocks();
});
```

- [ ] **Step 2: Run to confirm failure**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=auto-layout
```

- [ ] **Step 3: Create `streamdeck-plugin/src/actions/auto-layout.ts`**

```typescript
import { action, SingletonAction, streamDeck, type KeyDownEvent } from '@elgato/streamdeck';

const PROFILES: Record<number, string> = {
  0: 'Battlestream Standard', // Stream Deck (5×3)
  1: 'Battlestream Mini',     // Stream Deck Mini (3×2)
  2: 'Battlestream XL',       // Stream Deck XL (8×4)
  7: 'Battlestream Plus',     // Stream Deck + (4×3)
};

@action({ UUID: 'com.battlestream.streamdeck.auto-layout' })
export class AutoLayoutAction extends SingletonAction<Record<string, never>> {
  override async onKeyDown({ action }: KeyDownEvent<Record<string, never>>): Promise<void> {
    const { type, id } = action.device;
    const profile = PROFILES[type] ?? 'Battlestream Standard';
    await streamDeck.profiles.switchToProfile(id, profile);
  }
}
```

- [ ] **Step 4: Run tests**

```bash
cd streamdeck-plugin
npm test -- --testPathPattern=auto-layout
```

Expected: all 5 test cases PASS.

- [ ] **Step 5: Commit**

```bash
git add streamdeck-plugin/src/actions/auto-layout.ts streamdeck-plugin/src/__tests__/actions/auto-layout.test.ts
git commit -m "feat(streamdeck): auto-layout action switches to bundled profile for current device"
```

---

## Task 11: Plugin entry point + global settings UI

**Files:**
- Create: `streamdeck-plugin/src/plugin.ts`
- Create: `streamdeck-plugin/ui/global-settings.html`

- [ ] **Step 1: Create `streamdeck-plugin/src/plugin.ts`**

```typescript
import streamDeck from '@elgato/streamdeck';
import { BattlestreamClient } from './client.js';
import { store } from './state.js';
import type { GlobalSettings } from './types.js';

// Stat actions
import { HealthAction } from './actions/health.js';
import { ArmorAction } from './actions/armor.js';
import { TavernTierAction } from './actions/tavern-tier.js';
import { GoldAction } from './actions/gold.js';
import { TriplesAction } from './actions/triples.js';
import { WinStreakAction } from './actions/win-streak.js';
import { LossStreakAction } from './actions/loss-streak.js';
import { PlacementAction } from './actions/placement.js';
import { SpellPowerAction } from './actions/spell-power.js';
import { TurnAction } from './actions/turn.js';
import { PhaseAction } from './actions/phase.js';
import { MinionCountAction } from './actions/minion-count.js';
import { BuffAtkAction } from './actions/buff-atk.js';
import { BuffHpAction } from './actions/buff-hp.js';
import { AnomalyAction } from './actions/anomaly.js';
import { BloodgemBuffAction } from './actions/bloodgem-buff.js';
import { ElementalBuffAction } from './actions/elemental-buff.js';
import { SpellcraftAction } from './actions/spellcraft.js';
import { TavernSpellBuffAction } from './actions/tavern-spell-buff.js';
import { AutoLayoutAction } from './actions/auto-layout.js';

// Register all actions before connecting
streamDeck.actions.registerAction(new HealthAction());
streamDeck.actions.registerAction(new ArmorAction());
streamDeck.actions.registerAction(new TavernTierAction());
streamDeck.actions.registerAction(new GoldAction());
streamDeck.actions.registerAction(new TriplesAction());
streamDeck.actions.registerAction(new WinStreakAction());
streamDeck.actions.registerAction(new LossStreakAction());
streamDeck.actions.registerAction(new PlacementAction());
streamDeck.actions.registerAction(new SpellPowerAction());
streamDeck.actions.registerAction(new TurnAction());
streamDeck.actions.registerAction(new PhaseAction());
streamDeck.actions.registerAction(new MinionCountAction());
streamDeck.actions.registerAction(new BuffAtkAction());
streamDeck.actions.registerAction(new BuffHpAction());
streamDeck.actions.registerAction(new AnomalyAction());
streamDeck.actions.registerAction(new BloodgemBuffAction());
streamDeck.actions.registerAction(new ElementalBuffAction());
streamDeck.actions.registerAction(new SpellcraftAction());
streamDeck.actions.registerAction(new TavernSpellBuffAction());
streamDeck.actions.registerAction(new AutoLayoutAction());

const client = new BattlestreamClient();

function applySettings(settings: GlobalSettings): void {
  const config = {
    host: settings.host?.trim() || '127.0.0.1',
    port: settings.port ?? 8080,
    apiKey: settings.apiKey ?? '',
  };
  client.reconnect(config);
}

// Apply default config immediately (before global settings arrive)
client.connect({ host: '127.0.0.1', port: 8080, apiKey: '' }, state => store.setState(state));

// Re-apply whenever settings change
streamDeck.settings.onDidReceiveGlobalSettings(({ settings }) => {
  applySettings(settings as GlobalSettings);
});

streamDeck.connect();
```

- [ ] **Step 2: Create `streamdeck-plugin/ui/global-settings.html`**

```html
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Battlestream Settings</title>
  <style>
    * { box-sizing: border-box; margin: 0; padding: 0; }
    body { font-family: system-ui, sans-serif; font-size: 13px; color: #ccc;
           background: #1e1e1e; padding: 12px; }
    .field { margin-bottom: 10px; }
    label { display: block; margin-bottom: 4px; color: #999; font-size: 11px;
            text-transform: uppercase; letter-spacing: 0.5px; }
    input { width: 100%; background: #2a2a2a; border: 1px solid #444; border-radius: 4px;
            color: #fff; padding: 6px 8px; font-size: 13px; }
    input:focus { outline: none; border-color: #5a7fd4; }
    button { width: 100%; margin-top: 12px; background: #3a5cbf; color: #fff;
             border: none; border-radius: 4px; padding: 8px; font-size: 13px;
             cursor: pointer; }
    button:hover { background: #4a6cd4; }
    .hint { margin-top: 8px; font-size: 11px; color: #666; line-height: 1.4; }
  </style>
</head>
<body>
  <div class="field">
    <label>Host</label>
    <input type="text" id="host" placeholder="127.0.0.1">
  </div>
  <div class="field">
    <label>Port</label>
    <input type="number" id="port" placeholder="8080" min="1" max="65535">
  </div>
  <div class="field">
    <label>API Key</label>
    <input type="password" id="apiKey" placeholder="Leave blank if using --no-auth">
  </div>
  <button id="saveBtn">Save Settings</button>
  <p class="hint">Run <code>battlestream daemon --no-auth</code> to skip the API key entirely.</p>

  <script>
    let ws;
    let uuid;

    window.connectElgatoStreamDeckSocket = function(port, inUUID, registerEvent) {
      uuid = inUUID;
      ws = new WebSocket('ws://127.0.0.1:' + port);

      ws.onopen = function() {
        ws.send(JSON.stringify({ event: registerEvent, uuid }));
        ws.send(JSON.stringify({ event: 'getGlobalSettings', context: uuid }));
      };

      ws.onmessage = function(evt) {
        const msg = JSON.parse(evt.data);
        if (msg.event === 'didReceiveGlobalSettings') {
          const s = msg.payload.settings || {};
          document.getElementById('host').value = s.host || '127.0.0.1';
          document.getElementById('port').value = s.port || 8080;
          document.getElementById('apiKey').value = s.apiKey || '';
        }
      };
    };

    document.getElementById('saveBtn').addEventListener('click', function() {
      if (!ws || ws.readyState !== WebSocket.OPEN) return;
      ws.send(JSON.stringify({
        event: 'setGlobalSettings',
        context: uuid,
        payload: {
          host: document.getElementById('host').value.trim() || '127.0.0.1',
          port: parseInt(document.getElementById('port').value, 10) || 8080,
          apiKey: document.getElementById('apiKey').value,
        }
      }));
    });
  </script>
</body>
</html>
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd streamdeck-plugin
npx tsc --noEmit
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
git add streamdeck-plugin/src/plugin.ts streamdeck-plugin/ui/
git commit -m "feat(streamdeck): plugin entry point and global settings UI"
```

---

## Task 12: Build integration

**Files:**
- Create: `scripts/build-plugin.sh`
- Create: `Makefile`
- Modify: `.github/workflows/ci.yml`

- [ ] **Step 1: Create `scripts/build-plugin.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PLUGIN_DIR="$SCRIPT_DIR/../streamdeck-plugin"
DIST="$PLUGIN_DIR/dist/com.battlestream.streamdeck.sdPlugin"

cd "$PLUGIN_DIR"
npm ci
npm run build

# Copy @napi-rs/canvas native module alongside the bundle
# (Rollup externalises it; Node.js resolves it relative to bin/)
BIN_MODULES="$DIST/bin/node_modules"
mkdir -p "$BIN_MODULES"
cp -r node_modules/@napi-rs "$BIN_MODULES/"

echo "Plugin built at: $DIST"
```

```bash
chmod +x scripts/build-plugin.sh
```

- [ ] **Step 2: Create `Makefile`**

```makefile
.PHONY: build build-plugin build-all test vet

build:
	go build ./cmd/battlestream

build-plugin:
	bash scripts/build-plugin.sh

build-all: build build-plugin

test:
	go test -count=1 ./...

vet:
	go vet ./...
```

- [ ] **Step 3: Test the build**

```bash
make build-plugin
```

Expected: `streamdeck-plugin/dist/com.battlestream.streamdeck.sdPlugin/` exists with `bin/plugin.js`, `manifest.json`, `ui/`, `imgs/`, and `bin/node_modules/@napi-rs/canvas/`.

- [ ] **Step 4: Add CI job to `.github/workflows/ci.yml`**

Add the following job after the `lint` job (before `proto`):

```yaml
  plugin:
    name: Stream Deck Plugin
    runs-on: ubuntu-latest

    steps:
      - uses: actions/checkout@v4

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '24'
          cache: 'npm'
          cache-dependency-path: streamdeck-plugin/package-lock.json

      - name: Install dependencies
        working-directory: streamdeck-plugin
        run: npm ci

      - name: Type-check
        working-directory: streamdeck-plugin
        run: npx tsc --noEmit

      - name: Test
        working-directory: streamdeck-plugin
        run: npm test

      - name: Build
        working-directory: streamdeck-plugin
        run: npm run build
```

- [ ] **Step 5: Run the full plugin test suite to confirm all tests pass**

```bash
cd streamdeck-plugin && npm test
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add scripts/build-plugin.sh Makefile .github/workflows/ci.yml
git commit -m "chore(streamdeck): build script, Makefile, and CI job"
```

---

## Task 13: Profile stubs and documentation

Bundled `.sdProfile` files must be created by the Stream Deck software (they are binary zip bundles, not hand-writable JSON). This task creates empty placeholder directories and documents the manual creation process.

**Files:**
- Create: `streamdeck-plugin/profiles/README.md`
- Create: `streamdeck-plugin/profiles/.gitkeep` (×4 per profile dir)

- [ ] **Step 1: Create profile placeholder directories**

```bash
mkdir -p streamdeck-plugin/profiles
cd streamdeck-plugin/profiles

# Create one stub directory per device type
for name in "Battlestream Standard" "Battlestream XL" "Battlestream Mini" "Battlestream Plus"; do
  mkdir -p "${name}.sdProfile"
  touch "${name}.sdProfile/.gitkeep"
done
```

- [ ] **Step 2: Create `streamdeck-plugin/profiles/README.md`**

```markdown
# Bundled Profiles

These `.sdProfile` directories are bundled with the plugin so users can switch to
a curated full-panel layout by pressing the Auto-Layout button.

## How to generate

1. Build and install the plugin: `make build-plugin`, then double-click
   `dist/com.battlestream.streamdeck.sdPlugin` (macOS) or drag it into Stream Deck (Windows).
2. In Stream Deck software, open the profile you want to edit.
3. Drag each stat action from the Battlestream category onto the desired key.
4. File → Export Profile → save to `streamdeck-plugin/profiles/<Name>.sdProfile`.
5. Repeat for each device type:
   - **Battlestream Standard** — 5×3 layout, all 15 core stat actions
   - **Battlestream XL** — 8×4 layout, all 15 core + 4 XL bonus actions
   - **Battlestream Mini** — 3×2 layout, 6 essential stats (Health, Tier, Gold, Turn, Triples, Phase)
   - **Battlestream Plus** — 4×3 layout, 12 stat actions

6. Commit the exported `.sdProfile` directories.

## Device type codes (manifest.json reference)

| DeviceType | Device |
|---|---|
| 0 | Stream Deck (5×3) |
| 1 | Stream Deck Mini (3×2) |
| 2 | Stream Deck XL (8×4) |
| 7 | Stream Deck + (4×3) |
```

- [ ] **Step 3: Commit**

```bash
git add streamdeck-plugin/profiles/
git commit -m "docs(streamdeck): profile stubs and generation instructions"
```

---

## Self-Review Checklist (do not skip)

**Spec coverage:**
- [x] `--no-auth` flag → Task 1
- [x] TypeScript plugin with `@elgato/streamdeck` SDK → Tasks 2–11
- [x] SSE connection + debounce → Task 5
- [x] `BattlestreamClient` singleton → Task 5 + 11
- [x] State store with fan-out → Task 4
- [x] Canvas renderer with gradient/label/value/subtitle → Task 6
- [x] 15 core stat actions, zero config → Task 8
- [x] 4 XL bonus stat actions → Task 9
- [x] Auto-Layout action with device detection → Task 10
- [x] Global settings UI (host/port/apiKey) → Task 11
- [x] Build script + Makefile + CI → Task 12
- [x] Bundled profiles → Task 13
- [x] Offline/no-game state (value `—`, subtitle `OFFLINE`) → BaseStat.renderOne (Task 7)

**Type consistency:** `BattlestreamClient.reconnect()` used in `plugin.ts` — defined in `client.ts` Task 5. ✓ `store.setState(null)` used in tests — defined in `state.ts` Task 4. ✓ `RenderOptions` used in `base.ts` — exported from `render.ts` Task 6. ✓
