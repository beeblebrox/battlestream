import { SingletonAction, type WillAppearEvent, type WillDisappearEvent } from '@elgato/streamdeck';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { store } from '../state.js';
import { renderButton } from '../render.js';
import type { GameState } from '../types.js';

const IMGS_DIR = path.join(path.dirname(fileURLToPath(import.meta.url)), '..', 'imgs', 'actions');

interface ImageSettable {
  setImage(image: string): Promise<void>;
  id?: string;
}

export abstract class BaseStat extends SingletonAction<Record<string, never>> {
  protected abstract label: string;
  protected abstract gradient: readonly [string, string];
  protected abstract extract(state: GameState): { value: string; subtitle: string };

  private readonly contexts = new Set<ImageSettable>();
  private unsub?: () => void;
  private readonly lastRenderKeys = new Map<ImageSettable, string>();

  override async onWillAppear({ action }: WillAppearEvent<Record<string, never>>): Promise<void> {
    if (this.contexts.size === 0) {
      this.unsub = store.subscribe(state => void this.updateAll(state));
    }
    this.contexts.add(action as unknown as ImageSettable);
    await this.renderOne(action as unknown as ImageSettable, store.getState(), true);
  }

  override async onWillDisappear({ action }: WillDisappearEvent<Record<string, never>>): Promise<void> {
    for (const ctx of this.contexts) {
      if ((ctx as unknown as { id: string }).id === (action as unknown as { id: string }).id) {
        this.contexts.delete(ctx);
        this.lastRenderKeys.delete(ctx);
        break;
      }
    }
    if (this.contexts.size === 0) {
      this.unsub?.();
      this.unsub = undefined;
    }
  }

  private async updateAll(state: GameState | null): Promise<void> {
    await Promise.all([...this.contexts].map(a => this.renderOne(a, state, false)));
  }

  private async renderOne(action: ImageSettable, state: GameState | null, force: boolean): Promise<void> {
    let value: string;
    let subtitle: string;
    if (state === null) {
      value = '—'; subtitle = 'OFFLINE';
    } else {
      try {
        ({ value, subtitle } = this.extract(state));
      } catch {
        value = 'ERR'; subtitle = '';
      }
    }
    const key = `${value}|${subtitle}|${state === null}`;
    if (!force && key === this.lastRenderKeys.get(action)) return;
    this.lastRenderKeys.set(action, key);
    const iconName = this.manifestId?.split('.').pop();
    const iconPath = iconName ? path.join(IMGS_DIR, `${iconName}.png`) : undefined;
    const image = await renderButton({
      label: this.label,
      value,
      subtitle,
      gradient: this.gradient,
      offline: state === null,
      iconPath,
    });
    await action.setImage(image);
  }
}
