#!/usr/bin/env python3
"""Deterministic, lab-only upstream/payment fixtures. No outbound network calls."""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    provider = "lab-mock-upstream"
    kind = "upstream"
    orders = {}
    outbox = []

    def _send(self, payload, status=200, content_type="application/json"):
        if content_type == "text/event-stream":
            body = payload.encode()
        else:
            body = json.dumps(payload, sort_keys=True).encode()
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("X-Lab-Only", "1")
        self.end_headers()
        self.wfile.write(body)

    def _json_body(self):
        try:
            length = int(self.headers.get("Content-Length", "0"))
            return json.loads(self.rfile.read(length) or b"{}")
        except (ValueError, json.JSONDecodeError):
            return {}

    def do_GET(self):
        if self.path in ("/healthz", "/"):
            self._send({"status": "ok", "lab_only": True, "provider": self.provider})
            return
        if self.kind == "payment" and self.path == "/v1/outbox":
            self._send({"events": self.outbox, "source": "LAB_ONLY"})
            return
        self._send({"error": {"code": "lab_not_found", "message": "fixture not found"}}, 404)

    def do_POST(self):
        body = self._json_body()
        if self.kind == "upstream":
            self._upstream(body)
            return
        self._payment(body)

    def _upstream(self, body):
        scenario = body.get("lab_scenario", "normal")
        if scenario == "upstream_failure":
            self._send({"error": {"code": "lab_upstream_failure", "message": "deterministic upstream failure"}}, 502)
            return
        if scenario == "stream_interrupt" or body.get("stream"):
            payload = "data: {\"id\":\"lab_resp_stream_001\",\"lab_trace_id\":\"lab-upstream-stream-001\"}\n\ndata: {\"error\":{\"code\":\"lab_stream_interrupted\"}}\n\nevent: response.incomplete\ndata: {}\n\n"
            self._send(payload, 200, "text/event-stream")
            return
        self._send({"id": "lab_resp_normal_001", "object": "response", "lab_trace_id": "lab-upstream-normal-001", "usage": {"input_tokens": 12, "output_tokens": 8, "total_tokens": 20}, "output": [{"type": "message", "content": [{"type": "output_text", "text": "lab fixture response"}]}]})

    def _payment(self, body):
        if self.path == "/v1/notifications":
            event_type = str(body.get("type", "")).strip()
            if not event_type:
                self._send({"error": {"code": "lab_notification_type_required"}}, 400)
                return
            event = {"sequence": len(self.outbox) + 1, "type": event_type, "source": "LAB_ONLY", "payload": {k: ("[REDACTED]" if any(s in k.lower() for s in ("token", "secret", "password", "api_key", "authorization", "cookie")) else v) for k, v in body.items() if k != "type"}}
            self.outbox.append(event)
            self._send({"accepted": True, "outbox_sequence": event["sequence"], "source": "LAB_ONLY"}, 202)
            return
        if self.path == "/v1/orders":
            webhook = body.get("webhook_url")
            if webhook and (not webhook.startswith("http://admin-lab-api:") or "admin-lab" not in webhook):
                self._send({"error": {"code": "lab_webhook_target_rejected"}}, 400)
                return
            scenario = body.get("lab_scenario", "success")
            order_id = "lab_pay_failed_001" if scenario == "failed" else "lab_pay_success_001"
            order = {"id": order_id, "status": "created", "amount_cny": str(body.get("amount_cny", "10.00")), "lab_only": True}
            self.orders[order_id] = {**order, "scenario": scenario}
            self._send(order, 201)
            return
        if self.path.startswith("/v1/orders/") and self.path.endswith("/confirm"):
            order_id = self.path.split("/")[3]
            order = self.orders.get(order_id)
            if not order:
                self._send({"error": {"code": "lab_order_not_found"}}, 404)
                return
            status = "failed" if order["scenario"] == "failed" else "succeeded"
            payload = {**order, "status": status, "webhook": {"target": "http://admin-lab-api:8080/api/v1/payment/webhook/mock", "event": "payment_" + status}}
            self.orders[order_id] = payload
            self._send(payload)
            return
        if self.path.startswith("/v1/orders/") and self.path.endswith("/refund"):
            order_id = self.path.split("/")[3]
            if order_id not in self.orders:
                self._send({"error": {"code": "lab_order_not_found"}}, 404)
                return
            self._send({"id": order_id, "status": "refund_pending", "lab_only": True}, 202)
            return
        self._send({"error": {"code": "lab_not_found"}}, 404)

    def log_message(self, *_args):
        return


def main():
    port = int(sys.argv[1])
    Handler.kind = sys.argv[2]
    Handler.provider = "lab-mock-payment" if Handler.kind == "payment" else "lab-mock-upstream"
    Handler.orders = {}
    Handler.outbox = []
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
