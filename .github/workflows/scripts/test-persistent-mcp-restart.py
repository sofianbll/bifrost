#!/usr/bin/env python3
"""Synthetic HTTP/OAuth restart probe; never accepts a historical-fix claim."""
import argparse
import base64
from contextlib import contextmanager
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


HTTP = urllib.request.build_opener(NoRedirect, urllib.request.ProxyHandler({}))


def request(url, method="GET", data=None, headers=None):
    headers = dict(headers or {})
    if isinstance(data, dict):
        data = json.dumps(data).encode()
        headers["Content-Type"] = "application/json"
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        response = HTTP.open(req, timeout=5)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        body = response.read()
        content = response.headers.get("Content-Type", "")
        if "text/event-stream" in content:
            body = next(line[6:] for line in body.splitlines() if line.startswith(b"data: "))
        return response.status, response.headers, json.loads(body) if body else None


def require_status(status, expected, stage):
    if status != expected:
        raise AssertionError(f"{stage}: expected HTTP {expected}, received {status}")


def free_port():
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


def fixture(token):
    sessions = set()
    counts = {"authorized_requests": 0, "refused_requests": 0, "initializations": 0, "calls": 0}

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass

        def answer(self, status, body=None, session=None):
            raw = json.dumps(body).encode() if body is not None else b""
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            if session:
                self.send_header("Mcp-Session-Id", session)
            self.end_headers()
            self.wfile.write(raw)

        def do_GET(self):
            self.answer(405)

        def do_DELETE(self):
            sessions.discard(self.headers.get("Mcp-Session-Id"))
            self.answer(200)

        def do_POST(self):
            if self.headers.get("Authorization") != "Bearer " + token:
                counts["refused_requests"] += 1
                self.answer(401, {"error": "invalid_token"})
                return
            counts["authorized_requests"] += 1
            msg = json.loads(self.rfile.read(int(self.headers.get("Content-Length", "0"))))
            method = msg["method"]
            if "id" not in msg:
                self.answer(202)
                return
            session = self.headers.get("Mcp-Session-Id")
            if method == "initialize":
                session = secrets.token_hex(12)
                sessions.add(session)
                counts["initializations"] += 1
                result = {"protocolVersion": msg["params"]["protocolVersion"],
                          "capabilities": {"tools": {}},
                          "serverInfo": {"name": "persistent-http-fixture", "version": "1"}}
            elif session not in sessions:
                self.answer(404)
                return
            elif method == "tools/list":
                result = {"tools": [{"name": "echo", "description": "Echo a synthetic marker",
                                     "inputSchema": {"type": "object", "properties": {"text": {"type": "string"}},
                                                     "required": ["text"]}}]}
            elif method == "tools/call":
                counts["calls"] += 1
                result = {"content": [{"type": "text", "text": msg["params"]["arguments"]["text"]}]}
            elif method == "ping":
                result = {}
            else:
                self.answer(200, {"jsonrpc": "2.0", "id": msg["id"],
                                  "error": {"code": -32601, "message": "method not found"}}, session)
                return
            self.answer(200, {"jsonrpc": "2.0", "id": msg["id"], "result": result}, session)

    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server, counts


def oauth_token(base, vk, admin):
    redirect = "http://127.0.0.1:49999/callback"
    status, _, registration = request(base + "/oauth2/register", "POST", {
        "client_name": "restart-ci-probe", "redirect_uris": [redirect]})
    require_status(status, 201, "OAuth registration")
    verifier = secrets.token_urlsafe(40)
    challenge = base64.urlsafe_b64encode(hashlib.sha256(verifier.encode()).digest()).decode().rstrip("=")
    query = urllib.parse.urlencode({"client_id": registration["client_id"], "redirect_uri": redirect,
                                   "response_type": "code", "code_challenge": challenge,
                                   "code_challenge_method": "S256", "scope": "mcp",
                                   "resource": base + "/mcp", "state": "restart-ci"})
    status, headers, _ = request(base + "/oauth2/authorize?" + query)
    require_status(status, 302, "OAuth authorize")
    location = urllib.parse.urlsplit(headers["Location"])
    flow = urllib.parse.parse_qs(location.query)["flow"][0]
    status, _, consent = request(base + "/api/oauth2/consent/flows/" + flow, "PUT",
                                {"mode": "vk", "value": vk}, admin)
    require_status(status, 200, "OAuth consent")
    callback = urllib.parse.parse_qs(urllib.parse.urlsplit(consent["redirect_url"]).query)
    if callback.get("state") != ["restart-ci"]:
        raise AssertionError("OAuth callback state mismatch")
    body = urllib.parse.urlencode({"grant_type": "authorization_code", "client_id": registration["client_id"],
                                   "code": callback["code"][0], "code_verifier": verifier,
                                   "redirect_uri": redirect, "resource": base + "/mcp"}).encode()
    status, _, result = request(base + "/oauth2/token", "POST", body,
                               {"Content-Type": "application/x-www-form-urlencoded"})
    require_status(status, 200, "OAuth token")
    return result["access_token"]


def build_config(base, upstream_port, client_id, upstream_token, vk, denied_vk, setup):
    return {"setup_token": setup, "providers": {},
            "client": {"enable_logging": False, "enforce_governance_header": True,
                       "enforce_auth_on_inference": True, "mcp_server_auth_mode": "both",
                       "oauth2_server_config": {"issuer_url": base, "access_token_ttl": 1200}},
            "mcp": {"client_configs": [{"client_id": client_id, "name": "persistent_http",
                                        "endpoint_slug": "persistent-http", "connection_type": "http",
                                        "connection_string": f"http://127.0.0.1:{upstream_port}/mcp",
                                        "auth_type": "headers", "headers": {"Authorization": "Bearer " + upstream_token},
                                        "needs_session_stickiness": False, "allow_by_default": False,
                                        "is_ping_available": True, "tools_to_execute": ["*"],
                                        "tools_to_auto_execute": ["*"]}]},
            "governance": {"virtual_keys": [
                {"id": "restart-allowed", "name": "Restart allowed", "value": vk, "is_active": True,
                 "mcp_configs": [{"mcp_client_name": "persistent_http", "tools_to_execute": ["*"]}]},
                {"id": "restart-denied", "name": "Restart denied", "value": denied_vk, "is_active": True,
                 "mcp_configs": []}]}}


@contextmanager
def gateway(binary, directory, port):
    with (directory / "private-gateway.log").open("ab") as log:
        process = subprocess.Popen([binary, "-host", "127.0.0.1", "-port", str(port),
                                    "-app-dir", str(directory), "-log-level", "warn"], stdout=log, stderr=log)
        try:
            deadline = time.monotonic() + 60
            while time.monotonic() < deadline:
                if process.poll() is not None:
                    raise AssertionError(f"gateway boot exited {process.returncode}; private logs not exported")
                try:
                    if request(f"http://127.0.0.1:{port}/health")[0] == 200:
                        break
                except (OSError, ValueError):
                    pass
                time.sleep(.2)
            else:
                raise AssertionError("gateway health timeout")
            yield
        finally:
            if process.poll() is None:
                process.terminate()
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)


def client_row(base, admin, client_id):
    status, _, clients = request(base + "/api/mcp/clients", headers=admin)
    require_status(status, 200, "client projection")
    return next((c for c in clients.get("clients", [])
                 if c.get("config", {}).get("client_id") == client_id), {})


def rpc(base, method, credential, params=None, session=None):
    headers = {"Authorization": "Bearer " + credential,
               "Accept": "application/json, text/event-stream", "MCP-Protocol-Version": "2025-03-26"}
    if session:
        headers["Mcp-Session-Id"] = session
    message = {"jsonrpc": "2.0", "method": method, "params": params or {}}
    if not method.startswith("notifications/"):
        message["id"] = 1
    return request(base + "/mcp/persistent-http", "POST", message, headers)


def assert_authorized(base, credential, marker):
    status, headers, initialized = rpc(base, "initialize", credential, {
        "protocolVersion": "2025-03-26", "capabilities": {},
        "clientInfo": {"name": "restart-ci", "version": "1"}})
    require_status(status, 200, marker + " initialize")
    if "result" not in initialized:
        raise AssertionError(marker + " initialize returned JSON-RPC error")
    session = headers.get("Mcp-Session-Id")
    status, _, _ = rpc(base, "notifications/initialized", credential, session=session)
    require_status(status, 202, marker + " initialized notification")
    status, _, listed = rpc(base, "tools/list", credential, session=session)
    require_status(status, 200, marker + " tools/list")
    tools = listed.get("result", {}).get("tools", [])
    tool = next((t["name"] for t in tools if t["name"].endswith("echo")), None)
    if not tool:
        raise AssertionError(marker + " tools/list omitted granted echo")
    status, _, called = rpc(base, "tools/call", credential, {"name": tool, "arguments": {"text": marker}}, session)
    require_status(status, 200, marker + " tools/call")
    result = called.get("result", {})
    if result.get("isError") or not any(c.get("text") == marker for c in result.get("content", [])):
        raise AssertionError(marker + " tools/call did not echo the exact marker")
    return {"initialize": 200, "tools_list": 200, "tools_call": 200, "marker_echoed": True}


def negative_control(base, denied_vk, iteration):
    status, _, body = rpc(base, "initialize", denied_vk, {
        "protocolVersion": "2025-03-26", "capabilities": {},
        "clientInfo": {"name": "negative-control", "version": "1"}})
    require_status(status, 403, "deliberately ungranted virtual key")
    if "access_denied" not in json.dumps(body):
        raise AssertionError("negative control did not return exact access_denied signal")
    try:
        assert_authorized(base, denied_vk, "negative-control")
    except AssertionError:
        return {"restart": iteration, "status": status, "error": "access_denied",
                "authorized_request_assertion_rejected": True,
                "kind": "deliberate missing grant, not historical reproduction"}
    raise AssertionError("authorized-request assertion accepted a denied key")


def probe(args, report):
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise AssertionError("application execution is restricted to GitHub Actions")
    upstream_token, vk, denied_vk, setup = ("sk-bf-" + secrets.token_hex(16) for _ in range(4))
    upstream, counts = fixture(upstream_token)
    port = free_port()
    base = f"http://127.0.0.1:{port}"
    admin = {"X-Bifrost-Setup-Token": setup}
    client_id = "00000000-0000-4000-8000-000000000011"
    report.update({"route": "/mcp/persistent-http", "authentication": ["VK header", "Bifrost-issued OAuth JWT"],
                   "sequence": ["create SQLite store with HTTP sticky=false", "authorize and call",
                                "PUT sticky=true", "restart same process binary and store", "authorize and call twice",
                                "restart same store again", "authorize and call twice"],
                   "historical_incident": "not_reproduced_yet", "checkpoints": [],
                   "upstream_auth": "synthetic static Bearer header; live upstream OAuth not exercised"})
    try:
        with tempfile.TemporaryDirectory(prefix="bifrost-persistent-ci-") as tmp:
            directory = Path(tmp)
            config = build_config(base, upstream.server_port, client_id, upstream_token, vk, denied_vk, setup)
            config_path = directory / "config.json"
            config_path.write_text(json.dumps(config))
            original_config = config_path.read_bytes()
            with gateway(args.binary, directory, port):
                # Health is earlier than MCP discovery; readiness polling is separate from authorization assertions.
                deadline = time.monotonic() + 30
                while not client_row(base, admin, client_id).get("tools"):
                    if time.monotonic() >= deadline:
                        raise AssertionError("fixture tools were not discovered before probe")
                    time.sleep(.2)
                token = oauth_token(base, vk, admin)
                report["checkpoints"].append({"phase": "before", "header": assert_authorized(base, vk, "before-header"),
                                               "oauth": assert_authorized(base, token, "before-oauth")})
                status, _, _ = request(base + "/api/mcp/client/" + client_id, "PUT",
                                       {"needs_session_stickiness": True}, admin)
                require_status(status, 200, "enable persistent connection")
            store = directory / "config.db"
            if not store.is_file():
                raise AssertionError("initial boot did not create the disposable SQLite store")
            inode = store.stat().st_ino
            if config_path.read_bytes() != original_config:
                raise AssertionError("API update unexpectedly changed the config file; store hydration would be unproven")
            for iteration in range(1, 3):
                with gateway(args.binary, directory, port):
                    if store.stat().st_ino != inode:
                        raise AssertionError("restart replaced the existing SQLite store")
                    row = client_row(base, admin, client_id)
                    if row.get("config", {}).get("needs_session_stickiness") is not True:
                        raise AssertionError("persistent flag was not restored from the existing store")
                    report["checkpoints"].append({"phase": f"after-restart-{iteration}", "persistent_flag": True,
                        "header": assert_authorized(base, vk, f"restart-{iteration}-header"),
                        "oauth": assert_authorized(base, token, f"restart-{iteration}-oauth")})
                    report.setdefault("negative_controls", []).append(negative_control(base, denied_vk, iteration))
            if counts["refused_requests"] or counts["calls"] != 6:
                raise AssertionError("fixture did not receive all six authorized calls with its synthetic credential")
            report["existing_store"] = {"same_sqlite_file": True, "original_config_unchanged": True}
            report["upstream_fixture_counts"] = counts
            report["historical_incident"] = "not_reproduced_in_synthetic_scenario"
            report["result"] = "pass"
    finally:
        upstream.shutdown()
        upstream.server_close()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    parser.add_argument("--commit", required=True)
    parser.add_argument("--variant", choices=("stock", "fork"), required=True)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    report = {"commit": args.commit, "variant": args.variant, "tested_version": "2.2.6",
              "validation": "automated_synthetic_http_oauth", "codex_verification": "not_run",
              "production": "not_accessed", "result": "fail"}
    began = time.monotonic()
    interrupted = {"signal": 0}

    def interrupt(signum, _frame):
        interrupted["signal"] = signum
        # Ignore a repeated signal while context managers stop children and remove private files.
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        raise KeyboardInterrupt

    signal.signal(signal.SIGTERM, interrupt)
    signal.signal(signal.SIGINT, interrupt)
    try:
        probe(args, report)
    except KeyboardInterrupt:
        report["result"] = "interrupted"
        report["signal"] = interrupted["signal"]
    except (AssertionError, OSError, ValueError, KeyError, StopIteration) as error:
        # Errors expose stage/status only; credential-bearing bodies and private process logs are never emitted.
        report["failure"] = str(error) if isinstance(error, AssertionError) else type(error).__name__
    report["elapsed_seconds"] = round(time.monotonic() - began, 3)
    output = Path(args.output)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))
    return 128 + interrupted["signal"] if interrupted["signal"] else (0 if report["result"] == "pass" else 1)


if __name__ == "__main__":
    raise SystemExit(main())
