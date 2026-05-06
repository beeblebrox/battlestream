import type { GameState } from './types.js';

type Subscriber = (state: GameState | null) => void;

class StateStore {
  private state: GameState | null = null;
  private subscribers = new Set<Subscriber>();

  getState(): GameState | null {
    return this.state;
  }

  setState(state: GameState | null): void {
    this.state = state;
    for (const sub of this.subscribers) {
      sub(state);
    }
  }

  subscribe(cb: Subscriber): () => void {
    this.subscribers.add(cb);
    return () => this.subscribers.delete(cb);
  }
}

export const store = new StateStore();
