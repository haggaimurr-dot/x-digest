#!/usr/bin/env python3
"""Append posts into data/raw_posts.json with hard dedup by post `id`.

Existing ids are loaded first; any incoming post whose `id` is already present
is skipped (not appended, not updated). Schema is unchanged: one JSON array of
objects, author fields live inside each object.
"""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path
from typing import Any, Iterable

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_PATH = ROOT / "data" / "raw_posts.json"


def load_raw_posts(path: Path) -> list[dict[str, Any]]:
    if not path.exists():
        return []
    data = json.loads(path.read_text(encoding="utf-8"))
    if data is None:
        return []
    if not isinstance(data, list):
        raise ValueError(f"{path} must be a JSON array")
    return data


def existing_ids(posts: Iterable[dict[str, Any]]) -> set[str]:
    ids: set[str] = set()
    for post in posts:
        pid = post.get("id")
        if pid is None or pid == "":
            raise ValueError("raw post missing required non-empty id")
        ids.add(str(pid))
    return ids


def append_raw_posts(
    incoming: list[dict[str, Any]],
    *,
    path: Path = DEFAULT_PATH,
) -> dict[str, int]:
    """Merge incoming posts into path. Skip any whose id already exists.

    Returns counts: {"added": n, "skipped": n, "total": n}.
    """
    current = load_raw_posts(path)
    seen = existing_ids(current)
    added = 0
    skipped = 0
    for post in incoming:
        if not isinstance(post, dict):
            raise ValueError("each incoming post must be an object")
        pid = post.get("id")
        if pid is None or pid == "":
            raise ValueError("incoming post missing required non-empty id")
        pid = str(pid)
        if pid in seen:
            skipped += 1
            continue
        current.append(post)
        seen.add(pid)
        added += 1
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(
        json.dumps(current, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
    return {"added": added, "skipped": skipped, "total": len(current)}


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "input",
        nargs="?",
        help="JSON file: array of posts, or {\"posts\": [...]}. Default: stdin",
    )
    parser.add_argument(
        "--path",
        type=Path,
        default=DEFAULT_PATH,
        help=f"raw_posts.json path (default: {DEFAULT_PATH})",
    )
    args = parser.parse_args(argv)
    raw = Path(args.input).read_text(encoding="utf-8") if args.input else sys.stdin.read()
    payload = json.loads(raw)
    if isinstance(payload, dict) and "posts" in payload:
        incoming = payload["posts"]
    elif isinstance(payload, list):
        incoming = payload
    else:
        raise SystemExit("input must be a JSON array or {\"posts\": [...]}")
    stats = append_raw_posts(incoming, path=args.path)
    print(json.dumps(stats, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
