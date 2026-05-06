import type { GameState, GlobalSettings } from './types.js';

type Subscriber = (state: GameState | null) => void;

class StateStore {
  private state: GameState | null = null;
  private stateKey: string | null = null;
  private settings: GlobalSettings = { host: '127.0.0.1', port: 8080, apiKey: '' };
  private subscribers = new Set<Subscriber>();

  getState(): GameState | null {
    return this.state;
  }

  setState(state: GameState | null): void {
    const key = state === null ? null : JSON.stringify(state);
    if (key === this.stateKey) return;
    this.stateKey = key;
    this.state = state;
    for (const sub of this.subscribers) {
      sub(state);
    }
  }

  getSettings(): GlobalSettings {
    return this.settings;
  }

  setSettings(s: GlobalSettings): void {
    this.settings = s;
  }

  subscribe(cb: Subscriber): () => void {
    this.subscribers.add(cb);
    return () => this.subscribers.delete(cb);
  }
}

export const store = new StateStore();
