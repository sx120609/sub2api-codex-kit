#!/usr/bin/env python3
"""One-shot local probe: can this machine get 292-length x-codex-turn-state?"""
from __future__ import annotations

import json
import os
import sys
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

AUTH = Path.home() / ".codex" / "auth.json"
URL = "https://chatgpt.com/backend-api/codex/responses"
MODEL = os.environ.get("CODEX_TURNSTATE_MODEL", "gpt-5.6-sol")
N = int(os.environ.get("CODEX_TURNSTATE_PROBES", "4"))


def load_creds() -> tuple[str, str]:
    data = json.loads(AUTH.read_text(encoding="utf-8"))
    tokens = data.get("tokens") or {}
    access = (tokens.get("access_token") or "").strip()
    account = (tokens.get("account_id") or "").strip()
    if not access or not account:
        raise SystemExit("auth.json missing access_token or account_id")
    return access, account


def probe(access: str, account: str, model: str) -> dict:
    body = json.dumps(
        {
            "model": model,
            "store": False,
            "stream": True,
            "input": [
                {
                    "type": "message",
                    "role": "user",
                    "content": [{"type": "input_text", "text": "."}],
                }
            ],
        }
    ).encode()
    req = urllib.request.Request(URL, data=body, method="POST")
    req.add_header("Authorization", f"Bearer {access}")
    req.add_header("ChatGPT-Account-ID", account)
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "text/event-stream")
    req.add_header("OpenAI-Beta", "responses=experimental")
    req.add_header("User-Agent", "codex-turnstate-upload/0.1")
    try:
        with urllib.request.urlopen(req, timeout=20) as resp:
            token = (resp.headers.get("x-codex-turn-state") or "").strip()
            status = resp.status
            resp.read(256)
    except Exception as exc:
        return {"ok": False, "error": type(exc).__name__, "detail": str(exc)[:180]}
    return {
        "ok": bool(token),
        "status": status,
        "len": len(token),
        "prefix": token[:6] if token else "",
        "degraded": 308 <= len(token) <= 316,
        "quality_292": abs(len(token) - 292) <= 4,
        "quality_332": abs(len(token) - 332) <= 4,
    }


def main() -> int:
    if not AUTH.exists():
        print("no ~/.codex/auth.json")
        return 2
    access, account = load_creds()
    print(f"model={MODEL} probes={N} account_id_len={len(account)}")
    results = []
    with ThreadPoolExecutor(max_workers=N) as pool:
        futs = [pool.submit(probe, access, account, MODEL) for _ in range(N)]
        for fut in as_completed(futs):
            results.append(fut.result())
            print(results[-1])
    quality = [r for r in results if r.get("quality_292")]
    print(f"summary total={len(results)} ok={sum(1 for r in results if r.get('ok'))} len292={len(quality)}")
    return 0 if quality else 1


if __name__ == "__main__":
    sys.exit(main())
