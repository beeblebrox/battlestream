# Bundled Profiles

These `.sdProfile` directories are bundled with the plugin so users can switch to
a curated full-panel layout by pressing the Auto-Layout button.

## How to generate

1. Build and install the plugin: `make build-plugin`, then double-click
   `dist/com.battlestream.streamdeck.sdPlugin` (macOS) or drag it into Stream Deck (Windows).
2. In Stream Deck software, open the profile you want to edit.
3. Drag each stat action from the Battlestream category onto the desired key.
4. File → Export Profile → save to `streamdeck-plugin/profiles/<Name>.sdProfile`.
5. Repeat for each device type:
   - **Battlestream Standard** — 5×3 layout, all 15 core stat actions
   - **Battlestream XL** — 8×4 layout, all 15 core + 4 XL bonus actions
   - **Battlestream Mini** — 3×2 layout, 6 essential stats (Health, Tier, Gold, Turn, Triples, Phase)
   - **Battlestream Plus** — 4×3 layout, 12 stat actions

6. Commit the exported `.sdProfile` directories.

## Device type codes (manifest.json reference)

| DeviceType | Device |
|---|---|
| 0 | Stream Deck (5×3) |
| 1 | Stream Deck Mini (3×2) |
| 2 | Stream Deck XL (8×4) |
| 7 | Stream Deck + (4×3) |
