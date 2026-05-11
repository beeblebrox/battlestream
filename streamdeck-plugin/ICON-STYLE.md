# Stream Deck Icon Style Guidelines

Icons live in `imgs/actions/`. The render pipeline overlays white text on top of each icon,
so icons must never contain text or numbers of their own.

## Visual Style

- Flat minimal design — bold silhouettes or simple geometric shapes
- NO photorealism, NO gradients, NO drop shadows, NO 3D effects
- NO text, NO numbers, NO labels of any kind
- One or two bold flat colors max, high contrast against black

## Background

- Solid pure black (#000000) only — no transparency, no vignettes

## Size / Clarity

- Icons are rendered at 144×144px on stream deck buttons
- Use thick strokes and simple shapes — fine detail disappears at small sizes
- Center the symbol with breathing room at the edges (~10–15% padding)

## ChatGPT / DALL-E Prompt Template

```
"[Icon name]" stream deck icon — [description of symbol, e.g. "circular arrows"]. [Color(s)] on solid black (#000000) background. Flat minimal, bold silhouette, NO text, NO numbers, NO gradients, NO photorealism. Square composition, centered with breathing room.
```

### Example prompts

- **turn**: `"Turn" stream deck icon — circular arrows. White on solid black. Flat minimal, bold silhouette, NO text, NO numbers, NO gradients. Square, centered.`
- **gold**: `"Gold" stream deck icon — gold coin stack. Gold/yellow on solid black. Flat minimal, bold silhouette, NO text, NO numbers, NO gradients. Square, centered.`
- **tavern-tier**: `"Tavern Tier" stream deck icon — medieval castle silhouette with three small stars below. White and gold on solid black. Flat minimal, NO text, NO numbers. Square, centered.`

## Existing Icon Inventory

| File | Subject |
|---|---|
| `anomaly.png` | swirling vortex / anomaly |
| `armor.png` | shield |
| `auto-layout.png` | grid / layout |
| `bg-barrage-buff.png` | barrage / crossbow bolts |
| `beetle-buff.png` | beetle |
| `bloodgem-buff.png` | gem / crystal |
| `buff-atk.png` | sword |
| `buff-hp.png` | heart |
| `consumed-buff.png` | flame |
| `elemental-buff.png` | fire / lightning |
| `free-refresh.png` | recycle / refresh arrows |
| `gold.png` | coin stack |
| `gold-next-turn.png` | coin + arrow |
| `health.png` | heart / health cross |
| `lightfang-buff.png` | fang / tooth |
| `loss-streak.png` | downward trend / skull |
| `minion-count.png` | 3 creature silhouettes |
| `nomi-buff.png` | chef hat / cooking pot |
| `phase.png` | moon phases / cycle |
| `placement.png` | trophy / podium |
| `rightmost-buff.png` | arrow pointing right |
| `spell-power.png` | lightning bolt / star |
| `spellcraft.png` | spellbook / magic circle |
| `tavern-spell-buff.png` | scroll |
| `tavern-tier.png` | castle + 3 stars |
| `tavern-wide-buff.png` | banner / tavern sign |
| `triples.png` | 3 identical cards / triple symbol |
| `turn.png` | circular arrows |
| `undead-buff.png` | skull |
| `volumizer-buff.png` | speaker / volume wave |
| `whelp-buff.png` | dragon whelp |
| `win-streak.png` | upward trend / flame |
