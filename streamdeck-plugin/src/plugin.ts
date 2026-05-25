import streamDeck from '@elgato/streamdeck';
import { EventSource } from 'eventsource';
import { BattlestreamClient } from './client.js';
import { store } from './state.js';
import type { GlobalSettings } from './types.js';

import { HealthAction }          from './actions/health.js';
import { ArmorAction }           from './actions/armor.js';
import { TavernTierAction }      from './actions/tavern-tier.js';
import { GoldAction }            from './actions/gold.js';
import { TriplesAction }         from './actions/triples.js';
import { WinStreakAction }        from './actions/win-streak.js';
import { LossStreakAction }       from './actions/loss-streak.js';
import { PlacementAction }       from './actions/placement.js';
import { SpellPowerAction }      from './actions/spell-power.js';
import { TurnAction }            from './actions/turn.js';
import { PhaseAction }           from './actions/phase.js';
import { MinionCountAction }     from './actions/minion-count.js';
import { AnomalyAction }         from './actions/anomaly.js';
import { SpellcraftAction }      from './actions/spellcraft.js';
import { MinionsSoldAction }      from './actions/minions-sold.js';
import { SpellsCastAction }      from './actions/spells-cast.js';
import { SpellcraftCastAction }  from './actions/spellcraft-cast.js';
// Buff buttons
import { TavernWideBuffAction }  from './actions/tavern-wide-buff.js';
import { BloodgemBuffAction }    from './actions/bloodgem-buff.js';
import { BgBarrageBuffAction }   from './actions/bg-barrage-buff.js';
import { RightmostBuffAction }   from './actions/rightmost-buff.js';
import { ElementalBuffAction }   from './actions/elemental-buff.js';
import { NomiBuffAction }        from './actions/nomi-buff.js';
import { UndeadBuffAction }      from './actions/undead-buff.js';
import { LightfangBuffAction }   from './actions/lightfang-buff.js';
import { WhelpBuffAction }       from './actions/whelp-buff.js';
import { BeetleBuffAction }      from './actions/beetle-buff.js';
import { VolumizerBuffAction }   from './actions/volumizer-buff.js';
import { ConsumedBuffAction }    from './actions/consumed-buff.js';
import { DynamicBuffSlotAction } from './actions/buff-slot.js';
// Opponent buttons
import { OpponentHealthAction }     from './actions/opponent-health.js';
import { OpponentTavernTierAction } from './actions/opponent-tavern-tier.js';
import { AvailableTribesAction }    from './actions/tribes.js';
import { TotalBuffsAction }         from './actions/total-buffs.js';

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
streamDeck.actions.registerAction(new AnomalyAction());
streamDeck.actions.registerAction(new SpellcraftAction());
streamDeck.actions.registerAction(new MinionsSoldAction());
streamDeck.actions.registerAction(new SpellsCastAction());
streamDeck.actions.registerAction(new SpellcraftCastAction());
// Buff buttons
streamDeck.actions.registerAction(new TavernWideBuffAction());
streamDeck.actions.registerAction(new BloodgemBuffAction());
streamDeck.actions.registerAction(new BgBarrageBuffAction());
streamDeck.actions.registerAction(new RightmostBuffAction());
streamDeck.actions.registerAction(new ElementalBuffAction());
streamDeck.actions.registerAction(new NomiBuffAction());
streamDeck.actions.registerAction(new UndeadBuffAction());
streamDeck.actions.registerAction(new LightfangBuffAction());
streamDeck.actions.registerAction(new WhelpBuffAction());
streamDeck.actions.registerAction(new BeetleBuffAction());
streamDeck.actions.registerAction(new VolumizerBuffAction());
streamDeck.actions.registerAction(new ConsumedBuffAction());
streamDeck.actions.registerAction(new DynamicBuffSlotAction());
// Opponent buttons
streamDeck.actions.registerAction(new OpponentHealthAction());
streamDeck.actions.registerAction(new OpponentTavernTierAction());
streamDeck.actions.registerAction(new AvailableTribesAction());
// Summary buttons
streamDeck.actions.registerAction(new TotalBuffsAction());

function makeClient(host: string, port: number, apiKey: string): BattlestreamClient {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  return new BattlestreamClient(
    { host, port, apiKey },
    { onState: state => store.setState(state), EventSourceImpl: EventSource as any },
  );
}

let client: BattlestreamClient | null = null;

function applySettings(settings: GlobalSettings): void {
  const host = settings.host?.trim() || '127.0.0.1';
  const port = settings.port ?? 8080;
  const apiKey = settings.apiKey ?? '';
  store.setSettings({ host, port, apiKey });
  client?.disconnect();
  client = makeClient(host, port, apiKey);
  client.connect();
}

streamDeck.settings.onDidReceiveGlobalSettings(({ settings }) => {
  applySettings(settings as GlobalSettings);
});

// Connect to Stream Deck first, then fetch persisted settings before
// making any outbound connection — avoids hitting the wrong port on startup.
streamDeck.connect().then(() => {
  streamDeck.settings.getGlobalSettings();
});
