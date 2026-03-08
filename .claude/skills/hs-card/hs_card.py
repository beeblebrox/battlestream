#!/usr/bin/env python3
"""Hearthstone card lookup via HearthstoneJSON API."""

import argparse
import json
import re
import sys
import time
import urllib.request
from pathlib import Path

API_URL = "https://api.hearthstonejson.com/v1/latest/enUS/cards.json"
IMAGE_URL = "https://art.hearthstonejson.com/v1/render/latest/enUS/256x/{id}.png"

CACHE_DIR = Path(__file__).parent / "cache"
CACHE_MAX_AGE = 7 * 24 * 3600  # 7 days before re-checking remote

FIELDS = (
    "id", "name", "type", "set", "cardClass", "classes", "rarity",
    "attack", "health", "cost", "techLevel", "text", "mechanics",
    "race", "races", "spellSchool",
    "isBattlegroundsPoolMinion", "isBattlegroundsPoolSpell", "isBattlegroundsBuddy",
)

# Valid values for enum filters
TRIBES    = ("all", "beast", "demon", "dragon", "elemental", "mechanical",
             "murloc", "naga", "pirate", "quilboar", "undead")
TYPES     = ("minion", "spell", "battleground_spell", "battleground_trinket",
             "game_mode_button")
CLASSES   = ("deathknight", "demonhunter", "druid", "hunter", "mage", "neutral",
             "paladin", "rogue", "shaman", "warlock", "warrior")
RARITIES  = ("common", "epic", "free", "legendary", "rare")
KEYWORDS  = ("aura", "avenge", "battlecry", "choose_one", "deathrattle", "discover",
             "divine_shield", "end_of_turn_trigger", "magnetic", "poisonous", "reborn",
             "start_of_combat", "stealth", "taunt", "venomous", "windfury")


def get_cached_image(card_id: str) -> Path:
    """Return path to locally cached card image, downloading/refreshing as needed."""
    CACHE_DIR.mkdir(exist_ok=True)
    cache_file = CACHE_DIR / f"{card_id}.png"
    url = IMAGE_URL.format(id=card_id)

    if cache_file.exists():
        age = time.time() - cache_file.stat().st_mtime
        if age < CACHE_MAX_AGE:
            return cache_file
        # Stale: HEAD-check content-length; re-download only if changed
        try:
            req = urllib.request.Request(url, method="HEAD",
                                         headers={"User-Agent": "Mozilla/5.0"})
            with urllib.request.urlopen(req) as resp:
                remote_size = int(resp.headers.get("Content-Length") or 0)
            if remote_size == cache_file.stat().st_size:
                cache_file.touch()  # reset mtime, still fresh
                return cache_file
        except Exception:
            return cache_file  # network error: serve stale

    try:
        req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0"})
        with urllib.request.urlopen(req) as resp:
            cache_file.write_bytes(resp.read())
    except urllib.error.HTTPError:
        return None
    return cache_file


def fetch_cards() -> list[dict]:
    req = urllib.request.Request(API_URL, headers={"User-Agent": "Mozilla/5.0"})
    with urllib.request.urlopen(req) as resp:
        return json.load(resp)


def clean_text(text: str | None) -> str:
    if not text:
        return ""
    text = re.sub(r"<[^>]+>", "", text)
    text = re.sub(r"\[x\]", "", text)
    return " ".join(text.split())


def card_races(c: dict) -> list[str]:
    return c.get("races") or ([c["race"]] if c.get("race") else [])


def card_classes(c: dict) -> list[str]:
    return c.get("classes") or ([c["cardClass"]] if c.get("cardClass") else [])


def fmt_card(c: dict, show_image: bool = False) -> str:
    parts = [f"## {c['name']}"]
    tags = []
    if c.get("set"):
        tags.append(c["set"])
    if c.get("techLevel") is not None:
        tags.append(f"Tier {c['techLevel']}")
    if c.get("isBattlegroundsBuddy"):
        tags.append("BUDDY")
    if tags:
        parts[0] += "  [" + "]  [".join(tags) + "]"

    meta = []
    if c.get("type"):
        meta.append(f"Type: {c['type']}")
    classes = card_classes(c)
    if classes:
        meta.append(f"Class: {', '.join(classes)}")
    if c.get("rarity"):
        meta.append(f"Rarity: {c['rarity']}")
    if meta:
        parts.append("  |  ".join(meta))

    races = card_races(c)
    if races:
        parts.append("Tribe: " + ", ".join(r.title() for r in races))

    if c.get("spellSchool"):
        parts.append(f"Spell School: {c['spellSchool']}")

    stats = []
    if c.get("cost") is not None:
        stats.append(f"{c['cost']} mana")
    if c.get("attack") is not None and c.get("health") is not None:
        stats.append(f"{c['attack']}/{c['health']}")
    if stats:
        parts.append("Stats: " + "  ".join(stats))

    txt = clean_text(c.get("text"))
    if txt:
        parts.append(f"Text: {txt}")

    mechs = c.get("mechanics")
    if mechs:
        readable = [m.replace("_", " ").title() for m in mechs
                    if m not in ("TRIGGER_VISUAL", "InvisibleDeathrattle")]
        if readable:
            parts.append("Keywords: " + ", ".join(readable))

    flags = []
    if c.get("isBattlegroundsPoolMinion"):
        flags.append("Pool Minion")
    if c.get("isBattlegroundsPoolSpell"):
        flags.append("Pool Spell")
    if flags:
        parts.append("BG Flags: " + ", ".join(flags))

    parts.append(f"ID: {c['id']}")
    if show_image:
        local_path = get_cached_image(c['id'])
        if local_path:
            parts.append(f"Image: {local_path.resolve()}")
    return "\n".join(parts)


def pick_fields(c: dict) -> dict:
    return {k: c.get(k) for k in FIELDS}


def fmt_values(label: str, values: tuple[str, ...]) -> str:
    return f"  {label}: {', '.join(values)}"


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Look up Hearthstone cards.",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="\n".join([
            "Filter value reference:",
            fmt_values("--tribe  ", TRIBES),
            fmt_values("--type   ", TYPES),
            fmt_values("--class  ", CLASSES),
            fmt_values("--rarity ", RARITIES),
            fmt_values("--keyword", KEYWORDS),
            "",
            "Examples:",
            "  hs_card.py murloc warleader",
            "  hs_card.py --bg --tier 7",
            "  hs_card.py --tribe quilboar --tier 7",
            "  hs_card.py --tribe murloc --keyword divine_shield",
            "  hs_card.py --class neutral --keyword taunt --tier 3",
            "  hs_card.py --buddy",
            "  hs_card.py --pool --tribe beast",
            "  hs_card.py --id BG25_034",
        ]),
    )
    parser.add_argument("name", nargs="*",
                        help="Partial card name, case-insensitive")
    parser.add_argument("--bg", action="store_true",
                        help="Battlegrounds cards only (implied by --tier/--tribe/--buddy/--pool)")
    parser.add_argument("--tier", type=int, metavar="N",
                        help="BG tech level 1-7")
    parser.add_argument("--tribe", metavar="TRIBE",
                        help=f"Tribe: {', '.join(TRIBES)}")
    parser.add_argument("--type", dest="card_type", metavar="TYPE",
                        help=f"Card type: {', '.join(TYPES)}")
    parser.add_argument("--class", dest="card_class", metavar="CLASS",
                        help=f"Class: {', '.join(CLASSES)}")
    parser.add_argument("--rarity", metavar="RARITY",
                        help=f"Rarity: {', '.join(RARITIES)}")
    parser.add_argument("--keyword", metavar="KEYWORD",
                        help=f"Mechanic/keyword: {', '.join(KEYWORDS)}")
    parser.add_argument("--buddy", action="store_true",
                        help="BG buddy minions only (implies --bg)")
    parser.add_argument("--pool", action="store_true",
                        help="BG pool cards only (implies --bg)")
    parser.add_argument("--id", dest="card_id", metavar="ID",
                        help="Exact card ID lookup")
    parser.add_argument("--image", action="store_true",
                        help="Include card image in output")
    parser.add_argument("--limit", type=int, default=10,
                        help="Max results shown (default 10)")
    args = parser.parse_args()

    cards = fetch_cards()
    query = " ".join(args.name).lower().strip()

    if args.card_id:
        matches = [pick_fields(c) for c in cards if c.get("id") == args.card_id]
        if not matches:
            print(f"No card found with ID '{args.card_id}'.", file=sys.stderr)
            sys.exit(1)
        print(fmt_card(matches[0], show_image=args.image))
        return

    tribe_filter   = args.tribe.upper() if args.tribe else None
    type_filter    = args.card_type.upper() if args.card_type else None
    class_filter   = args.card_class.upper() if args.card_class else None
    rarity_filter  = args.rarity.upper() if args.rarity else None
    keyword_filter = args.keyword.upper() if args.keyword else None

    bg_mode = args.bg or args.tier is not None or tribe_filter or args.buddy or args.pool

    results = []
    for c in cards:
        name = (c.get("name") or "").lower()

        if query and query not in name:
            continue

        is_golden = (c.get("id") or "").endswith("_G")

        if bg_mode:
            if c.get("set") != "BATTLEGROUNDS":
                continue
            if args.tier is not None:
                if c.get("techLevel") != args.tier:
                    continue
                if is_golden:
                    continue
            else:
                if not isinstance(c.get("techLevel"), int) and not args.buddy:
                    continue
            if is_golden and (tribe_filter or args.pool):
                continue

        if tribe_filter:
            if tribe_filter not in card_races(c):
                continue

        if type_filter:
            if (c.get("type") or "").upper() != type_filter:
                continue

        if class_filter:
            if class_filter not in [cl.upper() for cl in card_classes(c)]:
                continue

        if rarity_filter:
            if (c.get("rarity") or "").upper() != rarity_filter:
                continue

        if keyword_filter:
            mechs = [m.upper() for m in (c.get("mechanics") or [])]
            if keyword_filter not in mechs:
                continue

        if args.buddy:
            if not c.get("isBattlegroundsBuddy"):
                continue

        if args.pool:
            if not c.get("isBattlegroundsPoolMinion") and not c.get("isBattlegroundsPoolSpell"):
                continue

        results.append(pick_fields(c))

    if bg_mode:
        results.sort(key=lambda c: (c.get("techLevel") or 99, c.get("name") or ""))
    else:
        results.sort(key=lambda c: c.get("name") or "")

    total = len(results)
    shown = results[: args.limit]

    if not shown:
        tip = "Try a shorter search term or fewer filters"
        print(f"No cards found. {tip}.", file=sys.stderr)
        sys.exit(1)

    print("\n\n".join(fmt_card(c, show_image=args.image) for c in shown))

    if total > args.limit:
        print(f"\n({total} total matches — showing {args.limit}. Use --limit N or refine your search.)")


if __name__ == "__main__":
    main()
