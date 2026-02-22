"""mitmproxy addon for agent-memory hint injection (best effort, fail-open).

Usage example:
  AGENT_MEMORY_API_URL=http://127.0.0.1:45229 \
  mitmdump -s tools/agent-memory/mitm_addon.py
"""

from __future__ import annotations

import base64
import json
import os
import urllib.request
import urllib.error
from typing import Any, Dict

try:
    from mitmproxy import http, ctx
except Exception:  # pragma: no cover
    http = None
    ctx = None

API_URL = os.environ.get("AGENT_MEMORY_API_URL", "http://127.0.0.1:45229").rstrip("/")
API_VERSION = os.environ.get("AGENT_MEMORY_PROXY_API_VERSION", "v2").lower()
REPO_PATH = os.environ.get("AGENT_MEMORY_REPO_PATH", "")
TARGET_HOSTS = ("openai.com", "anthropic.com")


def _post_json(path: str, payload: Dict[str, Any]) -> Dict[str, Any]:
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        API_URL + path,
        data=data,
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=2.0) as resp:
        return json.loads(resp.read().decode("utf-8"))


def _emit_event(flow: "http.HTTPFlow", injectable: bool, injected: bool, reason: str, fallback_used: bool = False, error: str = "") -> None:
    payload = {
        "provider": _provider(flow.request.pretty_host),
        "host": flow.request.pretty_host,
        "route": flow.request.path,
        "injectable": bool(injectable),
        "injected": bool(injected),
        "fallback_used": bool(fallback_used),
        "reason": reason,
        "error_redacted": error,
    }
    try:
        _post_json("/v1/proxy/event", payload)
    except Exception as exc:
        if ctx:
            ctx.log.warn(f"agent-memory event logging failed: {exc}")


def _provider(host: str) -> str:
    host = (host or "").lower()
    if "openai" in host:
        return "openai"
    if "anthropic" in host:
        return "anthropic"
    return ""


def request(flow: "http.HTTPFlow") -> None:
    host = (flow.request.pretty_host or "").lower()
    if not any(h in host for h in TARGET_HOSTS):
        return

    ctype = flow.request.headers.get("content-type", "")
    if "application/json" not in ctype.lower():
        _emit_event(flow, injectable=False, injected=False, reason="unsupported_content_type")
        return

    provider = _provider(host)
    body = flow.request.raw_content or b""
    payload = {
        "provider": provider,
        "host": flow.request.pretty_host,
        "path": flow.request.path,
        "method": flow.request.method,
        "headers": {k: v for k, v in flow.request.headers.items()},
        "body_b64": base64.b64encode(body).decode("ascii"),
    }
    if REPO_PATH:
        payload["repo_path"] = REPO_PATH

    try:
        transform_path = "/v2/proxy/transform" if API_VERSION == "v2" else "/v1/proxy/transform"
        resp = _post_json(transform_path, payload)
    except urllib.error.URLError as exc:
        _emit_event(flow, injectable=False, injected=False, reason="daemon_unreachable", error=str(exc))
        return
    except Exception as exc:
        _emit_event(flow, injectable=False, injected=False, reason="transform_call_failed", error=str(exc))
        return

    mutated = bool(resp.get("mutated"))
    injectable = bool(resp.get("injectable"))
    fallback_used = bool(resp.get("fallback_used"))
    reason = str(resp.get("reason") or "")

    if mutated:
        out_b64 = resp.get("body_b64")
        if isinstance(out_b64, str) and out_b64:
            try:
                flow.request.raw_content = base64.b64decode(out_b64)
            except Exception as exc:
                _emit_event(flow, injectable=False, injected=False, reason="invalid_transform_body", error=str(exc))
                return

    _emit_event(flow, injectable=injectable, injected=mutated, reason=reason, fallback_used=fallback_used)
