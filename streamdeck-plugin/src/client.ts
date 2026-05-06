import type { ClientConfig, GameState } from './types.js';

interface EventSourceInstance {
  readyState: number;
  onmessage: ((e: { data: string }) => void) | null;
  onerror: ((e: unknown) => void) | null;
  onopen: (() => void) | null;
  close(): void;
}

type EventSourceConstructor = new (url: string, init?: { headers?: Record<string, string> }) => EventSourceInstance;

interface ClientOptions {
  EventSourceImpl?: EventSourceConstructor;
  fetchImpl?: typeof fetch;
  onState?: (state: GameState | null) => void;
}

const BACKOFF_INITIAL = 500;
const BACKOFF_MAX = 30_000;
const POLL_INTERVAL = 5_000;

export class BattlestreamClient {
  private config: ClientConfig;
  private options: ClientOptions;
  private es: EventSourceInstance | null = null;
  private backoff = BACKOFF_INITIAL;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private pollTimer: ReturnType<typeof setInterval> | null = null;
  private destroyed = false;

  constructor(config: ClientConfig, options: ClientOptions = {}) {
    this.config = config;
    this.options = options;
  }

  connect(): void {
    this.destroyed = false;
    this.openSSE();
    this.fetchState();
    this.pollTimer = setInterval(() => { void this.fetchState(); }, POLL_INTERVAL);
  }

  disconnect(): void {
    this.destroyed = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    if (this.pollTimer !== null) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
    this.es?.close();
    this.es = null;
  }

  private baseUrl(): string {
    return `http://${this.config.host}:${this.config.port}`;
  }

  private authHeaders(): Record<string, string> {
    return this.config.apiKey ? { Authorization: `Bearer ${this.config.apiKey}` } : {};
  }

  private openSSE(): void {
    if (this.destroyed) return;
    const ESImpl = this.options.EventSourceImpl ?? (globalThis as any).EventSource;
    const headers = this.authHeaders();
    const url = `${this.baseUrl()}/v1/events`;
    const es = new ESImpl(url, Object.keys(headers).length ? { headers } : undefined);
    this.es = es;

    es.onopen = () => {
      this.backoff = BACKOFF_INITIAL;
    };

    es.onmessage = (_e: { data: string }) => {
      this.fetchState();
    };

    es.onerror = () => {
      es.close();
      this.es = null;
      if (!this.destroyed) {
        this.reconnectTimer = setTimeout(() => {
          this.backoff = Math.min(this.backoff * 2, BACKOFF_MAX);
          this.openSSE();
        }, this.backoff);
      }
    };
  }

  private async fetchState(): Promise<void> {
    const fetchFn = this.options.fetchImpl ?? fetch;
    const headers = this.authHeaders();
    try {
      const res = await fetchFn(`${this.baseUrl()}/v1/game/current`, { headers });
      if (!res.ok) {
        this.options.onState?.(null);
        return;
      }
      const state = (await res.json()) as GameState;
      this.options.onState?.(state);
    } catch {
      this.options.onState?.(null);
    }
  }
}
