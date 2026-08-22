#!/usr/bin/env python3
"""Focused black-box contract test for the isolated admin-lab mock services."""
import json
import pathlib
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]
SERVER = ROOT / "tools/admin-lab/mock_server.py"


def request(port, method, path, payload=None):
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        f"http://127.0.0.1:{port}{path}", data=data, method=method,
        headers={"Content-Type": "application/json"},
    )
    try:
        with urllib.request.urlopen(req, timeout=3) as response:
            return response.status, response.headers, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()


def wait_healthy(port, provider):
    deadline = time.time() + 5
    while time.time() < deadline:
        try:
            status, _, body = request(port, "GET", "/healthz")
            payload = json.loads(body)
            if status == 200 and payload["lab_only"] is True and payload["provider"] == provider:
                return
        except OSError:
            pass
        time.sleep(0.05)
    raise AssertionError(f"mock server on {port} did not become healthy")


def main():
    upstream = subprocess.Popen([sys.executable, str(SERVER), "18091", "upstream"])
    payment = subprocess.Popen([sys.executable, str(SERVER), "18092", "payment"])
    try:
        wait_healthy(18091, "lab-mock-upstream")
        wait_healthy(18092, "lab-mock-payment")

        status, _, body = request(18091, "POST", "/v1/responses", {"lab_scenario": "normal"})
        normal = json.loads(body)
        assert status == 200 and normal["id"] == "lab_resp_normal_001"
        assert normal["lab_trace_id"] == "lab-upstream-normal-001"
        assert normal["usage"] == {"input_tokens": 12, "output_tokens": 8, "total_tokens": 20}

        status, _, body = request(18091, "POST", "/v1/responses", {"lab_scenario": "upstream_failure"})
        failed = json.loads(body)
        assert status == 502 and failed["error"]["code"] == "lab_upstream_failure"

        status, headers, body = request(18091, "POST", "/v1/responses", {"stream": True, "lab_scenario": "stream_interrupt"})
        assert status == 200 and headers["Content-Type"].startswith("text/event-stream")
        assert b"response.incomplete" in body and b"lab_stream_interrupted" in body

        status, _, body = request(18092, "POST", "/v1/orders", {"lab_scenario": "success", "amount_cny": "10.00"})
        order = json.loads(body)
        assert status == 201 and order["id"] == "lab_pay_success_001" and order["status"] == "created"

        status, _, body = request(18092, "POST", "/v1/orders/lab_pay_success_001/confirm")
        confirmed = json.loads(body)
        assert status == 200 and confirmed["status"] == "succeeded"
        assert confirmed["webhook"]["target"] == "http://admin-lab-api:8080/api/v1/payment/webhook/mock"

        status, _, body = request(18092, "POST", "/v1/orders", {"lab_scenario": "failed"})
        failed_order = json.loads(body)
        assert status == 201 and failed_order["id"] == "lab_pay_failed_001"
        status, _, body = request(18092, "POST", "/v1/orders/lab_pay_failed_001/confirm")
        assert status == 200 and json.loads(body)["status"] == "failed"

        status, _, body = request(18092, "POST", "/v1/orders/lab_pay_success_001/refund")
        assert status == 202 and json.loads(body)["status"] == "refund_pending"

        status, _, body = request(18092, "POST", "/v1/notifications", {"type": "payment.succeeded", "token": "redact-me"})
        assert status == 202 and json.loads(body)["outbox_sequence"] == 1
        status, _, body = request(18092, "GET", "/v1/outbox")
        outbox = json.loads(body)
        assert status == 200 and outbox["events"][0]["payload"]["token"] == "[REDACTED]"

        status, _, body = request(18092, "POST", "/v1/orders", {"webhook_url": "https://real.example/webhook"})
        assert status == 400 and json.loads(body)["error"]["code"] == "lab_webhook_target_rejected"
    finally:
        for process in (upstream, payment):
            process.terminate()
        for process in (upstream, payment):
            process.wait(timeout=5)


if __name__ == "__main__":
    main()
