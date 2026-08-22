#!/usr/bin/env python3
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    provider = "upstream"

    def _send(self, payload, status=200):
        body = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path == "/healthz":
            self._send({"status": "ok", "lab_only": True, "provider": self.provider})
            return
        self._send({"status": "ok", "lab_only": True, "provider": self.provider})

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        if length:
            self.rfile.read(length)
        self._send({"status": "ok", "lab_only": True, "provider": self.provider, "request": "accepted"})

    def log_message(self, *_args):
        return


def main():
    port = int(sys.argv[1])
    Handler.provider = sys.argv[2]
    ThreadingHTTPServer(("0.0.0.0", port), Handler).serve_forever()


if __name__ == "__main__":
    main()
