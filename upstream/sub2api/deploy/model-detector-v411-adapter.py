#!/usr/bin/env python3
"""Private Sub2API adapter for the commercially authorized v4.1.1 detector."""

from __future__ import annotations

import hmac
import json
import math
import os
import sys
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from typing import Any

VERSION = "4.1.1"
SUPPORTED_MODELS = ("gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna")
PROFILE_REQUESTS = {"low": 19, "medium": 49, "high": 158}
MAX_BODY_BYTES = 64 << 10
VENDOR_ROOT = Path("/app/vendor/gpt56-v411")


def authorized(actual: str, expected: str) -> bool:
    return bool(expected) and hmac.compare_digest(actual, expected)


def _bounded_similarity(value: Any) -> dict[str, float]:
    if not isinstance(value, dict):
        return {}
    result: dict[str, float] = {}
    for model in SUPPORTED_MODELS:
        raw = value.get(model)
        if not isinstance(raw, (int, float)) or isinstance(raw, bool):
            continue
        score = float(raw)
        if math.isfinite(score) and 0 <= score <= 1:
            result[model] = score
    return result


def report_to_sidecar_response(report: dict[str, Any], profile: str, declared_model: str) -> dict[str, Any]:
    profile = profile if profile in PROFILE_REQUESTS else "low"
    network = report.get("network_summary") if isinstance(report.get("network_summary"), dict) else {}
    fingerprint = report.get("fingerprint_summary") if isinstance(report.get("fingerprint_summary"), dict) else {}
    planned = int(network.get("logical_tasks") or PROFILE_REQUESTS[profile])
    planned = min(max(planned, 0), PROFILE_REQUESTS["high"])
    valid = int(network.get("successful") or 0)
    valid = min(max(valid, 0), planned)
    juice_status = str(report.get("juice_verdict_state") or "insufficient")
    fingerprint_status = str(report.get("fingerprint_verdict_state") or fingerprint.get("fingerprint_status") or "unclear")
    similarity = _bounded_similarity(fingerprint.get("fingerprint_match") or report.get("fingerprint_match"))
    candidate = str(report.get("fingerprint_model") or fingerprint.get("fingerprint_model") or "").strip()
    if fingerprint_status != "strong_match" or candidate not in SUPPORTED_MODELS:
        candidate = ""
    complete = planned > 0 and valid >= math.ceil(planned * 0.9)
    if juice_status == "mismatch" or (candidate and candidate != declared_model):
        status = "abnormal"
    elif juice_status == "pass" and fingerprint_status == "strong_match" and candidate == declared_model and complete:
        status = "normal"
    else:
        status = "insufficient"
    return {
        "status": status,
        "profile": profile,
        "planned_requests": planned,
        "valid_samples": valid,
        "evidence_state": "complete" if complete else "insufficient",
        "juice_status": juice_status if juice_status in {"pass", "mismatch", "insufficient", "possible_non_gpt"} else "insufficient",
        "fingerprint_status": fingerprint_status if fingerprint_status in {"strong_match", "unclear"} else "unavailable",
        "fingerprint_candidate": candidate,
        "fingerprint_similarity": similarity,
        "detector_version": VERSION,
        "juice_summary": {"source": "gpt56_api_detector", "verdict": juice_status, "planned_requests": planned, "valid_samples": valid},
    }


def unavailable_response(profile: str, code: str) -> dict[str, Any]:
    profile = profile if profile in PROFILE_REQUESTS else "low"
    return {
        "status": "insufficient", "profile": profile, "planned_requests": PROFILE_REQUESTS[profile], "valid_samples": 0,
        "evidence_state": "unavailable", "juice_status": "insufficient", "fingerprint_status": "unavailable",
        "detector_version": VERSION, "error_code": code,
    }


def run_v411(request: dict[str, Any]) -> dict[str, Any]:
    profile = str(request.get("profile") or "low")
    declared_model = str(request.get("declared_model") or "").strip()
    request_model = str(request.get("request_model") or declared_model).strip()
    base_url = str(request.get("base_url") or "").strip()
    api_key = str(request.get("api_key") or "").strip()
    if profile not in PROFILE_REQUESTS or declared_model not in SUPPORTED_MODELS or not request_model:
        return unavailable_response(profile, "unsupported_detector_request")
    if not base_url or not api_key:
        return unavailable_response(profile, "invalid_detector_request")
    sys.path.insert(0, str(VENDOR_ROOT))
    try:
        from gpt56_vnext.detector import DetectorSession
        from gpt56_vnext.presets import get_preset

        config = get_preset("single", profile)
        config["workers"] = 1
        with tempfile.TemporaryDirectory(prefix="sub2api-v411-") as directory:
            session = DetectorSession(base_url=base_url, claimed_model=declared_model, request_model=request_model, api_key=api_key, config=config, directory=directory, retention_enabled=False)
            try:
                report = session.run_single()
            finally:
                session.close()
        return report_to_sidecar_response(report, profile, declared_model)
    except ValueError:
        return unavailable_response(profile, "detector_request_rejected")
    except Exception:
        return unavailable_response(profile, "detector_execution_failed")
    finally:
        if sys.path and sys.path[0] == str(VENDOR_ROOT):
            sys.path.pop(0)


class Handler(BaseHTTPRequestHandler):
    token = os.environ.get("SUB2API_MODEL_DETECTOR_TOKEN", "")

    def log_message(self, _format: str, *_args: Any) -> None:
        return

    def _send(self, status: int, payload: dict[str, Any]) -> None:
        body = json.dumps(payload, separators=(",", ":"), allow_nan=False).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Cache-Control", "no-store")
        self.end_headers()
        self.wfile.write(body)

    def _authorized(self) -> bool:
        prefix = "Bearer "
        header = self.headers.get("Authorization", "")
        return header.startswith(prefix) and authorized(header[len(prefix):], self.token)

    def _body(self) -> dict[str, Any]:
        length = int(self.headers.get("Content-Length", "0"))
        if length < 1 or length > MAX_BODY_BYTES:
            raise ValueError
        value = json.loads(self.rfile.read(length))
        if not isinstance(value, dict):
            raise ValueError
        return value

    def do_GET(self) -> None:
        if self.path == "/healthz":
            self._send(200, {"status": "ok", "version": VERSION})
            return
        if self.path != "/v1/catalog":
            self._send(404, {"error": "not_found"})
            return
        if not self._authorized():
            self._send(401, {"error": "unauthorized"})
            return
        self._send(200, {"version": VERSION, "models": [{"id": model, "supported": True} for model in SUPPORTED_MODELS]})

    def do_POST(self) -> None:
        if self.path != "/v1/detect":
            self._send(404, {"error": "not_found"})
            return
        if not self._authorized():
            self._send(401, {"error": "unauthorized"})
            return
        try:
            self._send(200, run_v411(self._body()))
        except (ValueError, json.JSONDecodeError):
            self._send(400, {"error": "invalid_request"})


def main() -> None:
    raw = os.environ.get("MODEL_DETECTOR_LISTEN_ADDRESS", ":8090")
    host, separator, port = raw.rpartition(":")
    if not separator or not port.isdigit() or not Handler.token:
        raise SystemExit("MODEL_DETECTOR_LISTEN_ADDRESS and SUB2API_MODEL_DETECTOR_TOKEN are required")
    ThreadingHTTPServer((host or "0.0.0.0", int(port)), Handler).serve_forever()


if __name__ == "__main__":
    main()
