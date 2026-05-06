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
