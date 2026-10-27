#!/usr/bin/env python3
"""Sanity checks for raw_posts id dedup (no network)."""
from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "scripts"))
from append_raw_posts import append_raw_posts, load_raw_posts  # noqa: E402


def _post(pid: str, text: str = "hello") -> dict:
    return {
        "id": pid,
        "url": f"https://x.com/u/status/{pid}",
        "author_id": "1",
        "author_username": "u",
        "author_name": "U",
        "created_at": "2026-01-01T00:00:00Z",
        "lang": "en",
        "text": text,
        "note_tweet": None,
        "long_text": None,
        "conversation_id": pid,
        "in_reply_to_id": None,
        "referenced_tweets": [],
        "media": [],
        "public_metrics": {
            "like": 0,
            "reply": 0,
            "repost": 0,
            "quote": 0,
            "impression": 0,
        },
        "fetched_at": "2026-10-27T00:00:00Z",
        "source": "builtin-x",
    }


class RawPostsDedupTest(unittest.TestCase):
    def test_same_id_written_twice_stored_once(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "raw_posts.json"
            path.write_text("[]\n", encoding="utf-8")
            first = append_raw_posts([_post("100", "first")], path=path)
            second = append_raw_posts([_post("100", "second-should-skip")], path=path)
            posts = load_raw_posts(path)
            self.assertEqual(first["added"], 1)
            self.assertEqual(first["skipped"], 0)
            self.assertEqual(second["added"], 0)
            self.assertEqual(second["skipped"], 1)
            self.assertEqual(len(posts), 1)
            self.assertEqual(posts[0]["id"], "100")
            self.assertEqual(posts[0]["text"], "first")  # not overwritten
            ids = [p["id"] for p in posts]
            self.assertEqual(len(ids), len(set(ids)))

    def test_distinct_ids_all_kept(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "raw_posts.json"
            path.write_text("[]\n", encoding="utf-8")
            stats = append_raw_posts(
                [_post("1"), _post("2"), _post("3"), _post("2")],
                path=path,
            )
            posts = load_raw_posts(path)
            self.assertEqual(stats["added"], 3)
            self.assertEqual(stats["skipped"], 1)
            self.assertEqual(stats["total"], 3)
            self.assertEqual({p["id"] for p in posts}, {"1", "2", "3"})
            self.assertEqual(len(posts), len({p["id"] for p in posts}))


if __name__ == "__main__":
    unittest.main()
