# Bundled Profiles

Pre-built layouts for all four Stream Deck device sizes. The Auto-Layout button uses
these to fill the whole panel with Battlestream stat actions in one press.

## Layouts

| File | Device | Grid | Actions |
|---|---|---|---|
| `Battlestream XL.json` | Stream Deck XL (8×4) | 32 keys | All 20 (4 slots empty) |
| `Battlestream Standard.json` | Stream Deck (5×3) | 15 keys | 15 core stats |
| `Battlestream Mini.json` | Stream Deck Mini (3×2) | 6 keys | Health, Tier, Gold, Turn, Triples, Phase |
| `Battlestream Plus.json` | Stream Deck + (4×3) | 12 keys | 12 core stats |

## Regenerating

```bash
cd streamdeck-plugin
node scripts/gen-profiles.mjs
```

## Installing (OpenDeck on Linux)

`make install-plugin` copies the plugin **and** all four profiles to OpenDeck automatically:

```
make install-plugin
```

Profiles land in every `sd-*` device directory under
`~/.var/app/me.amankhanna.opendeck/config/opendeck/profiles/`.
Restart OpenDeck to pick them up.

## Official Stream Deck software (macOS / Windows)

The `.sdProfile` directories alongside the JSON files are stubs for the official SDK's
`switchToProfile()` mechanism. To populate them for official Stream Deck software, export
profiles manually from the Stream Deck app and replace the stubs.
