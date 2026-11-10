"""Drive the packaged sidecar over JSON Lines with a fake Archery server.

Reproduces the DBX lifecycle: host connect carries only connection.id, then the
workbench invokes a method without any connectionId. Both must work.

Usage:
  python scripts/probe-sidecar.py --exe backend/bin/dbx-plugin-archery.exe
"""

import argparse
import json
import subprocess
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class FakeArchery(BaseHTTPRequestHandler):
    def log_message(self, *args):  # silence
        pass

    def _send(self, payload, code=200, cookies=(), content_type="application/json"):
        body = json.dumps(payload).encode()
        self.send_response(code)
        self.send_header("Content-Type", content_type)
        for cookie in cookies:
            self.send_header("Set-Cookie", cookie)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.startswith("/login/"):
            self._send(
                {},
                content_type="text/html",
                cookies=("csrftoken=csrf; Path=/",),
            )
            return
        if self.path.startswith("/api/info"):
            self._send({"archery": {"version": "1.14.0"}})
            return
        if self.path.startswith("/group/user_all_instances/"):
            self._send(
                {
                    "status": 0,
                    "msg": "ok",
                    "data": [
                        {
                            "id": 1,
                            "instance_name": "fake-mysql",
                            "type": "master",
                            "db_type": "mysql",
                        }
                    ],
                }
            )
            return
        self._send({"detail": "not found"}, 404)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0))
        self.rfile.read(length)
        if self.path.startswith("/authenticate/"):
            self._send(
                {"status": 0, "msg": "ok", "data": None},
                cookies=("sessionid=sess; Path=/",),
            )
            return
        self._send({"status": 0, "msg": "ok", "data": {}})


def call(proc, message):
    proc.stdin.write(json.dumps(message) + "\n")
    proc.stdin.flush()
    line = proc.stdout.readline().strip()
    return json.loads(line) if line else {}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--exe", required=True)
    args = parser.parse_args()

    server = ThreadingHTTPServer(("127.0.0.1", 0), FakeArchery)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    port = server.server_address[1]

    proc = subprocess.Popen(
        [args.exe],
        stdin=subprocess.PIPE,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    try:
        init = call(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "plugin/initialize",
                "params": {"host": {"protocolVersions": [1]}},
            },
        )
        print("initialize:", json.dumps(init, ensure_ascii=False))

        connect = call(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 2,
                "method": "connection/connect",
                "params": {
                    "connection": {
                        "id": "conn-1",
                        "name": "Archery",
                        "host": "127.0.0.1",
                        "port": port,
                        "username": "tester",
                        "password": "secret",
                        "external_config": {"scheme": "http"},
                    }
                },
            },
        )
        print("host connect (connection.id only):", json.dumps(connect, ensure_ascii=False))

        ui_connect = call(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 3,
                "method": "connection/connect",
                "params": {},
            },
        )
        print("ui connect (no id):", json.dumps(ui_connect, ensure_ascii=False))

        instances = call(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 4,
                "method": "archery/instances",
                "params": {},
            },
        )
        print("instances (no id):", json.dumps(instances, ensure_ascii=False))

        ok = (
            "result" in connect
            and connect["result"].get("success") is True
            and "result" in ui_connect
            and "result" in instances
            and len(instances["result"].get("instances", [])) == 1
        )
        print("PROBE", "PASS" if ok else "FAIL")
        raise SystemExit(0 if ok else 1)
    finally:
        proc.stdin.close()
        proc.terminate()
        server.shutdown()


if __name__ == "__main__":
    main()
