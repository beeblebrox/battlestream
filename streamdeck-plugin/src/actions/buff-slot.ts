import { action, SingletonAction, type WillAppearEvent, type WillDisappearEvent } from '@elgato/streamdeck';
import { store } from '../state.js';
import { renderButton } from '../render.js';
import { CATEGORY_META, DYNAMIC_CATEGORIES, categoryIconPath } from '../categories.js';
import type { GameState } from '../types.js';

export interface SlotState {
  category: string;
  lastUpdated: number;
}

interface ActionLike {
  id: string;
  setImage(image: string): Promise<void>;
  coordinates?: { row: number; column: number };
}

@action({ UUID: 'com.battlestream.streamdeck.buff-slot' })
export class DynamicBuffSlotAction extends SingletonAction<Record<string, never>> {
  private readonly slots    = new Map<string, SlotState>();
  private readonly coords   = new Map<string, { row: number; col: number }>();
  private readonly actionMap = new Map<string, ActionLike>();
  private unsub?: () => void;

  getSlots(): Map<string, SlotState> { return this.slots; }

  override async onWillAppear(ev: WillAppearEvent<Record<string, never>>): Promise<void> {
    const a = ev.action as unknown as ActionLike;
    const { row = 0, column: col = 0 } = (ev.payload as { coordinates?: { row: number; column: number } }).coordinates ?? {};
    this.coords.set(a.id, { row, col });
    this.actionMap.set(a.id, a);

    if (this.actionMap.size === 1) {
      this.unsub = store.subscribe(state => void this.onStateUpdate(state));
    }

    await this.onStateUpdate(store.getState());
  }

  override async onWillDisappear({ action }: WillDisappearEvent<Record<string, never>>): Promise<void> {
    const a = action as unknown as ActionLike;
    this.slots.delete(a.id);
    this.coords.delete(a.id);
    this.actionMap.delete(a.id);

    if (this.actionMap.size === 0) {
      this.unsub?.();
      this.unsub = undefined;
    }
  }

  private async onStateUpdate(state: GameState | null): Promise<void> {
    this.assign(state);
    await this.renderAll(state);
  }

  assign(state: GameState | null): void {
    if (state === null) {
      this.slots.clear();
      return;
    }

    const now = Date.now();

    const active = new Map<string, true>();
    for (const bs of state.buff_sources ?? []) {
      if (DYNAMIC_CATEGORIES.has(bs.category) && (bs.attack !== 0 || bs.health !== 0)) {
        active.set(bs.category, true);
      }
    }

    // Clear slots whose category is no longer active.
    for (const [id, slot] of this.slots) {
      if (!active.has(slot.category)) this.slots.delete(id);
    }

    // Refresh lastUpdated for still-active assigned categories.
    for (const slot of this.slots.values()) {
      if (active.has(slot.category)) slot.lastUpdated = now;
    }

    // Assign newly-active categories to free or LRU slots.
    const assigned = new Set([...this.slots.values()].map(s => s.category));
    for (const cat of active.keys()) {
      if (assigned.has(cat)) continue;

      const sorted = this.sortedIds();
      const freeId = sorted.find(id => !this.slots.has(id));

      if (freeId !== undefined) {
        this.slots.set(freeId, { category: cat, lastUpdated: now });
      } else {
        let lruId: string | undefined;
        let lruTime = Infinity;
        for (const [id, slot] of this.slots) {
          if (slot.lastUpdated < lruTime) { lruTime = slot.lastUpdated; lruId = id; }
        }
        if (lruId !== undefined) {
          this.slots.set(lruId, { category: cat, lastUpdated: now });
        }
      }
      assigned.add(cat);
    }
  }

  private sortedIds(): string[] {
    return [...this.actionMap.keys()].sort((a, b) => {
      const ca = this.coords.get(a) ?? { row: 0, col: 0 };
      const cb = this.coords.get(b) ?? { row: 0, col: 0 };
      return (ca.row * 1000 + ca.col) - (cb.row * 1000 + cb.col);
    });
  }

  private async renderAll(state: GameState | null): Promise<void> {
    await Promise.all([...this.actionMap.entries()].map(([id, a]) => this.renderOne(id, a, state)));
  }

  private async renderOne(id: string, a: ActionLike, state: GameState | null): Promise<void> {
    const slot = this.slots.get(id);

    if (!slot) {
      const img = await renderButton({
        label: '', value: '', subtitle: '',
        gradient: ['#000000', '#000000'],
        offline: false,
      });
      await a.setImage(img);
      return;
    }

    const meta = CATEGORY_META[slot.category];
    // state is non-null here: assign() clears slots when state is null
    const bs   = state!.buff_sources?.find(b => b.category === slot.category);
    const img  = await renderButton({
      label:    meta?.displayName ?? slot.category,
      value:    bs ? `+${bs.attack}/+${bs.health}` : '+0/+0',
      subtitle: '',
      gradient: meta?.gradient ?? ['#120a20', '#4a3070'],
      offline:  false,
      iconPath: categoryIconPath(slot.category),
    });
    await a.setImage(img);
  }
}
