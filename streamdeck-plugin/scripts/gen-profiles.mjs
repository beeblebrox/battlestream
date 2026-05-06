// Generates OpenDeck-format profile JSON files for all 4 device layouts.
// Run with: node scripts/gen-profiles.mjs

import { writeFileSync, mkdirSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const PROFILES_DIR = join(__dirname, '..', 'profiles');
const PLUGIN = 'com.battlestream.streamdeck.sdPlugin';

function key(uuid, name, icon, index) {
  const imgPath = `plugins/${PLUGIN}/imgs/actions/${icon}.png`;
  const state = {
    alignment: 'middle',
    background_colour: '#000000',
    colour: '#FFFFFF',
    family: 'Liberation Sans',
    image: imgPath,
    image_scale: 100,
    name: '',
    show: true,
    size: 16,
    stroke_colour: '#000000',
    stroke_size: 3,
    style: 'Regular',
    text: '',
    underline: false,
  };
  return {
    action: {
      controllers: ['Keypad'],
      disable_automatic_states: false,
      icon: imgPath,
      name,
      plugin: PLUGIN,
      property_inspector: `plugins/${PLUGIN}/ui/global-settings.html`,
      states: [state],
      supported_in_multi_actions: true,
      tooltip: '',
      uuid: `com.battlestream.streamdeck.${uuid}`,
      visible_in_action_list: true,
    },
    children: null,
    context: `Keypad.${index}.0`,
    current_state: 0,
    settings: {},
    states: [state],
  };
}

function profile(keys) {
  return JSON.stringify({ keys, sliders: [] }, null, 2);
}

// ── XL  8×4 = 32 keys ────────────────────────────────────────────────────────
// Row 0: Health Armor Tier Gold Triples WinStr LossStr Placement
// Row 1: SpellPwr Turn Phase Minions BuffATK BuffHP Anomaly (empty)
// Row 2: Bloodgem Elemental Spellcraft TavernSpell (empty×4)
// Row 3: (empty×7) AutoLayout
const xl = Array(32).fill(null);
[
  ['health',            'Health',           'health',           0],
  ['armor',             'Armor',            'armor',            1],
  ['tavern-tier',       'Tavern Tier',      'tavern-tier',      2],
  ['gold',              'Gold',             'gold',             3],
  ['triples',           'Triples',          'triples',          4],
  ['win-streak',        'Win Streak',       'win-streak',       5],
  ['loss-streak',       'Loss Streak',      'loss-streak',      6],
  ['placement',         'Placement',        'placement',        7],
  ['spell-power',       'Spell Power',      'spell-power',      8],
  ['turn',              'Turn',             'turn',             9],
  ['phase',             'Phase',            'phase',            10],
  ['minion-count',      'Minion Count',     'minion-count',     11],
  ['buff-atk',          'Buff ATK',         'buff-atk',         12],
  ['buff-hp',           'Buff HP',          'buff-hp',          13],
  ['anomaly',           'Anomaly',          'anomaly',          14],
  ['bloodgem-buff',     'Bloodgem Buff',    'bloodgem-buff',    16],
  ['elemental-buff',    'Elemental Buff',   'elemental-buff',   17],
  ['spellcraft',        'Spellcraft',       'spellcraft',       18],
  ['tavern-spell-buff', 'Tavern Spell Buff','tavern-spell-buff',19],
  ['auto-layout',       'Auto-Layout',      'auto-layout',      31],
].forEach(([uuid, name, icon, idx]) => { xl[idx] = key(uuid, name, icon, idx); });

// ── Standard  5×3 = 15 keys ──────────────────────────────────────────────────
// Row 0: Health Armor Tier Gold Triples
// Row 1: WinStr LossStr Placement SpellPwr Turn
// Row 2: Phase Minions BuffATK BuffHP Anomaly
const std = [
  key('health',       'Health',       'health',       0),
  key('armor',        'Armor',        'armor',        1),
  key('tavern-tier',  'Tavern Tier',  'tavern-tier',  2),
  key('gold',         'Gold',         'gold',         3),
  key('triples',      'Triples',      'triples',      4),
  key('win-streak',   'Win Streak',   'win-streak',   5),
  key('loss-streak',  'Loss Streak',  'loss-streak',  6),
  key('placement',    'Placement',    'placement',    7),
  key('spell-power',  'Spell Power',  'spell-power',  8),
  key('turn',         'Turn',         'turn',         9),
  key('phase',        'Phase',        'phase',        10),
  key('minion-count', 'Minion Count', 'minion-count', 11),
  key('buff-atk',     'Buff ATK',     'buff-atk',     12),
  key('buff-hp',      'Buff HP',      'buff-hp',      13),
  key('anomaly',      'Anomaly',      'anomaly',      14),
];

// ── Mini  3×2 = 6 keys ───────────────────────────────────────────────────────
// Row 0: Health Tier Gold
// Row 1: Turn Triples Phase
const mini = [
  key('health',      'Health',      'health',      0),
  key('tavern-tier', 'Tavern Tier', 'tavern-tier', 1),
  key('gold',        'Gold',        'gold',        2),
  key('turn',        'Turn',        'turn',        3),
  key('triples',     'Triples',     'triples',     4),
  key('phase',       'Phase',       'phase',       5),
];

// ── Plus  4×3 = 12 keys ──────────────────────────────────────────────────────
// Row 0: Health Armor Tier Gold
// Row 1: WinStr LossStr Triples Placement
// Row 2: Turn Phase Minions BuffATK
const plus = [
  key('health',       'Health',       'health',       0),
  key('armor',        'Armor',        'armor',        1),
  key('tavern-tier',  'Tavern Tier',  'tavern-tier',  2),
  key('gold',         'Gold',         'gold',         3),
  key('win-streak',   'Win Streak',   'win-streak',   4),
  key('loss-streak',  'Loss Streak',  'loss-streak',  5),
  key('triples',      'Triples',      'triples',      6),
  key('placement',    'Placement',    'placement',    7),
  key('turn',         'Turn',         'turn',         8),
  key('phase',        'Phase',        'phase',        9),
  key('minion-count', 'Minion Count', 'minion-count', 10),
  key('buff-atk',     'Buff ATK',     'buff-atk',     11),
];

mkdirSync(PROFILES_DIR, { recursive: true });

const files = [
  ['Battlestream XL.json',       profile(xl)],
  ['Battlestream Standard.json', profile(std)],
  ['Battlestream Mini.json',     profile(mini)],
  ['Battlestream Plus.json',     profile(plus)],
];

for (const [name, content] of files) {
  const path = join(PROFILES_DIR, name);
  writeFileSync(path, content);
  console.log('wrote', path);
}

console.log('Done.');
