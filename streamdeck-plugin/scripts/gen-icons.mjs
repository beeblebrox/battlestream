// Generates plugin-icon.png, category.png, and all 20 action icons.
// Run with: node scripts/gen-icons.mjs

import { createCanvas } from '@napi-rs/canvas';
import { writeFileSync, mkdirSync } from 'fs';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ROOT = join(__dirname, '..');

function save(canvas, path) {
  writeFileSync(path, canvas.toBuffer('image/png'));
  console.log('wrote', path);
}

// ── Plugin icon (72×72 and 144×144) ─────────────────────────────────────────
// Dark card with a stylised lightning bolt and "BS" monogram
function drawPluginIcon(size) {
  const canvas = createCanvas(size, size);
  const ctx = canvas.getContext('2d');
  const s = size / 72; // scale factor

  // Background gradient
  const bg = ctx.createLinearGradient(0, 0, size, size);
  bg.addColorStop(0, '#0d1117');
  bg.addColorStop(1, '#1a2236');
  ctx.fillStyle = bg;
  ctx.roundRect(0, 0, size, size, 10 * s);
  ctx.fill();

  // Accent ring
  ctx.strokeStyle = '#f7821b';
  ctx.lineWidth = 2.5 * s;
  ctx.roundRect(3 * s, 3 * s, size - 6 * s, size - 6 * s, 8 * s);
  ctx.stroke();

  // Lightning bolt (centred)
  const cx = size / 2;
  const cy = size / 2;
  ctx.beginPath();
  ctx.moveTo(cx + 4 * s,  cy - 20 * s);
  ctx.lineTo(cx - 8 * s,  cy + 2 * s);
  ctx.lineTo(cx,          cy + 2 * s);
  ctx.lineTo(cx - 4 * s,  cy + 20 * s);
  ctx.lineTo(cx + 8 * s,  cy - 2 * s);
  ctx.lineTo(cx,          cy - 2 * s);
  ctx.closePath();
  const bolt = ctx.createLinearGradient(cx - 8 * s, cy - 20 * s, cx + 8 * s, cy + 20 * s);
  bolt.addColorStop(0, '#f7821b');
  bolt.addColorStop(1, '#ffb347');
  ctx.fillStyle = bolt;
  ctx.fill();

  return canvas;
}

save(drawPluginIcon(72),  join(ROOT, 'imgs/plugin-icon.png'));
save(drawPluginIcon(144), join(ROOT, 'imgs/plugin-icon@2x.png'));

// ── Category icon (same design, smaller bolt) ────────────────────────────────
save(drawPluginIcon(72),  join(ROOT, 'imgs/category.png'));
save(drawPluginIcon(144), join(ROOT, 'imgs/category@2x.png'));

// ── Action icons (72×72 solid-colour per stat) ───────────────────────────────
const ACTIONS = [
  { name: 'health',           gradient: ['#7b0000', '#c0392b'], label: '♥' },
  { name: 'armor',            gradient: ['#3d0000', '#922b21'], label: '🛡' },
  { name: 'tavern-tier',      gradient: ['#1a3a00', '#27ae60'], label: '⬆' },
  { name: 'gold',             gradient: ['#5c4a00', '#d4a017'], label: '⬡' },
  { name: 'triples',          gradient: ['#2d0060', '#8e44ad'], label: '3×' },
  { name: 'win-streak',       gradient: ['#003366', '#2980b9'], label: 'W' },
  { name: 'loss-streak',      gradient: ['#4a2000', '#e67e22'], label: 'L' },
  { name: 'placement',        gradient: ['#00474a', '#16a085'], label: '#' },
  { name: 'spell-power',      gradient: ['#4a004a', '#a93226'], label: '✦' },
  { name: 'turn',             gradient: ['#1a1a3a', '#5d6d7e'], label: 'T' },
  { name: 'phase',            gradient: ['#1a0030', '#6c3483'], label: '⟳' },
  { name: 'minion-count',     gradient: ['#003030', '#1abc9c'], label: '♟' },
  { name: 'buff-atk',         gradient: ['#3a1000', '#cb4335'], label: '⚔' },
  { name: 'buff-hp',          gradient: ['#3a003a', '#c0392b'], label: '❤' },
  { name: 'anomaly',          gradient: ['#1a1a1a', '#566573'], label: '?' },
  { name: 'bloodgem-buff',    gradient: ['#3a1a00', '#e67e22'], label: '◈' },
  { name: 'elemental-buff',   gradient: ['#3a2a00', '#f39c12'], label: '◆' },
  { name: 'spellcraft',       gradient: ['#2a0030', '#9b59b6'], label: '✧' },
  { name: 'tavern-spell-buff',gradient: ['#003030', '#1abc9c'], label: '☽' },
  { name: 'auto-layout',      gradient: ['#4a3800', '#c8960c'], label: '⚡' },
];

mkdirSync(join(ROOT, 'imgs/actions'), { recursive: true });

for (const { name, gradient, label } of ACTIONS) {
  const size = 72;
  const canvas = createCanvas(size, size);
  const ctx = canvas.getContext('2d');

  const bg = ctx.createLinearGradient(0, 0, size, size);
  bg.addColorStop(0, gradient[0]);
  bg.addColorStop(1, gradient[1]);
  ctx.fillStyle = bg;
  ctx.roundRect(0, 0, size, size, 8);
  ctx.fill();

  ctx.fillStyle = 'rgba(255,255,255,0.85)';
  ctx.font = 'bold 28px sans-serif';
  ctx.textAlign = 'center';
  ctx.textBaseline = 'middle';
  ctx.fillText(label, size / 2, size / 2);

  save(canvas, join(ROOT, `imgs/actions/${name}.png`));
}

console.log('Done.');
