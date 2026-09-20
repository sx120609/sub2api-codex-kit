#!/usr/bin/env python3
"""Harvest local 292 x-codex-turn-state tokens and upload them to Sub2API."""
from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.request
from concurrent.futures import ThreadPoolExecutor, as_completed
from pathlib import Path

DEFAULT_UPSTREAM = "https://chatgpt.com/backend-api/codex/responses"
DEFAULT_SERVER = os.environ.get("SUB2API_URL", "http://155.103.66.233:8081")
DEFAULT_MODEL = os.environ.get("CODEX_TURNSTATE_MODEL", "gpt-5.6-sol")


def load_local_auth(path: Path) -> tuple[str, str]:
    data = json.loads(path.read_text(encoding="utf-8"))
    tokens = data.get("tokens") or {}
    access = (tokens.get("access_token") or "").strip()
    account = (tokens.get("account_id") or "").strip()
    if not access or not account:
        raise SystemExit(f"{path} missing tokens.access_token or tokens.account_id")
    return access, account


def probe(access: str, account: str, model: str, proxy: str | None) -> dict:
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
    req = urllib.request.Request(DEFAULT_UPSTREAM, data=body, method="POST")
    req.add_header("Authorization", f"Bearer {access}")
    req.add_header("ChatGPT-Account-ID", account)
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "text/event-stream")
    req.add_header("OpenAI-Beta", "responses=experimental")
    req.add_header("User-Agent", "codex-turnstate-upload/0.1")
    opener = urllib.request.build_opener()
    if proxy:
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({"http": proxy, "https": proxy}))
    try:
        with opener.open(req, timeout=20) as resp:
            token = (resp.headers.get("x-codex-turn-state") or "").strip()
            status = resp.status
            resp.read(512)
    except urllib.error.HTTPError as exc:
        snippet = exc.read(180).decode("utf-8", "replace") if exc.fp else ""
        return {"ok": False, "status": exc.code, "error": snippet[:180]}
    except Exception as exc:
        return {"ok": False, "error": f"{type(exc).__name__}: {str(exc)[:160]}"}
    return {
        "ok": bool(token.startswith("gAAAAA")),
        "status": status,
        "token": token,
        "len": len(token),
        "quality_292": abs(len(token) - 292) <= 4,
        "degraded": 308 <= len(token) <= 316,
    }


def api(server: str, method: str, path: str, token: str | None, payload: dict | None = None) -> dict:
    url = server.rstrip("/") + path
    data = None if payload is None else json.dumps(payload).encode()
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            raw = resp.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        body = exc.read().decode("utf-8", "replace")
        raise SystemExit(f"{method} {path} -> {exc.code} {body[:400]}") from exc
    parsed = json.loads(raw) if raw else {}
    if isinstance(parsed, dict) and "data" in parsed:
        return parsed["data"]
    return parsed


def login(server: str, email: str, password: str) -> str:
    data = api(server, "POST", "/api/v1/auth/login", None, {"email": email, "password": password})
    if data.get("requires_2fa"):
        raise SystemExit("admin 开了 2FA，请用 --jwt 传入 access_token")
    token = data.get("access_token") or ""
    if not token:
        raise SystemExit(f"login response missing access_token: {sorted(data.keys())}")
    return token


def main() -> int:
    parser = argparse.ArgumentParser(description="Harvest local 292 Turn-State tokens and upload to Sub2API")
    parser.add_argument("--auth", default=str(Path.home() / ".codex" / "auth.json"))
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--probes", type=int, default=6)
    parser.add_argument("--proxy", default=os.environ.get("HTTPS_PROXY") or os.environ.get("https_proxy") or "")
    parser.add_argument("--server", default=DEFAULT_SERVER)
    parser.add_argument("--email", default=os.environ.get("SUB2API_EMAIL", "admin@kit.local"))
    parser.add_argument("--password", default=os.environ.get("SUB2API_PASSWORD", ""))
    parser.add_argument("--jwt", default=os.environ.get("SUB2API_JWT", ""))
    parser.add_argument("--account-id", type=int, default=0, help="Sub2API OpenAI OAuth account id")
    parser.add_argument("--harvest-only", action="store_true")
    args = parser.parse_args()

    access, chatgpt_account = load_local_auth(Path(args.auth))
    print(f"harvest model={args.model} probes={args.probes} chatgpt_account_id={chatgpt_account}")
    results = []
    with ThreadPoolExecutor(max_workers=max(1, args.probes)) as pool:
        futs = [pool.submit(probe, access, chatgpt_account, args.model, args.proxy or None) for _ in range(args.probes)]
        for fut in as_completed(futs):
            item = fut.result()
            results.append(item)
            print(
                {
                    "ok": item.get("ok"),
                    "status": item.get("status"),
                    "len": item.get("len"),
                    "quality_292": item.get("quality_292"),
                    "degraded": item.get("degraded"),
                    "error": item.get("error"),
                }
            )
    quality = [r["token"] for r in results if r.get("quality_292") and r.get("token")]
    print(f"summary ok={sum(1 for r in results if r.get('ok'))}/{len(results)} len292={len(quality)}")
    if args.harvest_only:
        return 0 if quality else 1
    if not quality:
        print("no 292 tokens to upload")
        return 1

    jwt = args.jwt
    if not jwt:
        if not args.password:
            raise SystemExit("need --password or --jwt to upload")
        jwt = login(args.server, args.email, args.password)
    payload = {
        "model": args.model,
        "tokens": quality,
        "chatgpt_account_id": chatgpt_account,
        "source": "local-upload",
    }
    path = "/api/v1/admin/openai/codex-state-kit/tokens"
    if args.account_id > 0:
        path = f"/api/v1/admin/openai/accounts/{args.account_id}/codex-state-kit/tokens"
    result = api(args.server, "POST", path, jwt, payload)
    print(
        {
            "accepted": result.get("accepted"),
            "skipped": result.get("skipped"),
            "degraded": result.get("degraded"),
            "status": result.get("status"),
            "len": result.get("len"),
            "accountId": result.get("accountId"),
        }
    )
    return 0 if result.get("accepted") else 1


if __name__ == "__main__":
    sys.exit(main())
