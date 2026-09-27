#!/usr/bin/env python3
"""Qualify Code Mode errors and first-boot VK attachments on an isolated binary.

Uses an already installed official server-debug package, temporary synthetic keys,
and a fresh database. Starts only a loopback gateway and stops its process group.
Reports checks and binary identity without credentials, HTML or full responses.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import time
import urllib.error
import urllib.request


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--node", type=Path, required=True)
    parser.add_argument("--debug-server", type=Path, required=True)
    parser.add_argument("--work-dir", type=Path, required=True)
    parser.add_argument("--port", type=int, default=18095)
    args = parser.parse_args()
    for path in (args.binary, args.node, args.debug_server):
        if not path.is_file():
            parser.error(f"file not found: {path}")
    # Refuse an occupied port; never reuse or stop an existing gateway.
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", args.port))
    args.work_dir.mkdir(mode=0o700, parents=True, exist_ok=False)
    app_dir = args.work_dir / "app"
    app_dir.mkdir(mode=0o700)
    keys = {name: "sk-bf-" + secrets.token_hex(24) for name in ("full", "virtual", "denied")}
    config = {
        "client": {"enable_logging": False, "enforce_governance_header": True,
                   "enforce_auth_on_inference": True},
        "providers": {},
        "mcp": {
            "client_configs": [{
                "client_id": "candidate-debug", "name": "debug", "endpoint_slug": "debug",
                "connection_type": "stdio", "auth_type": "none", "allow_by_default": False,
                "is_code_mode_client": True, "tools_to_execute": ["*"],
                "stdio_config": {"command": str(args.node.resolve()),
                                 "args": [str(args.debug_server.resolve()), "--stdio"]},
            }],
            "tool_manager_config": {"code_mode_binding_level": "server"},
            "virtual_mcps": [{
                "id": 900001, "name": "Candidate Apps", "endpoint_slug": "candidate-apps",
                "tools": [{"mcp_client_name": "debug", "tool_names": ["*"]}],
                "virtual_key_ids": ["vk-full", "vk-virtual"],
            }],
        },
        "governance": {"virtual_keys": [{
            "id": "vk-" + name, "name": name, "value": value, "is_active": True,
            "provider_configs": [],
            "mcp_configs": [{"mcp_client_name": "debug", "tools_to_execute": ["*"]}]
            if name == "full" else [],
        } for name, value in keys.items()]},
    }
    config_path = app_dir / "config.json"
    config_path.write_text(json.dumps(config, indent=2))
    config_path.chmod(0o600)
    base = f"http://127.0.0.1:{args.port}"
    report = {"binary_sha256": hashlib.sha256(args.binary.read_bytes()).hexdigest(),
              "fresh_database": True, "gateway_starts": 1, "checks": []}

    def check(name, passed):
        report["checks"].append({"name": name, "passed": bool(passed)})
        print(("PASS " if passed else "FAIL ") + name, flush=True)

    def request(path, method=None, params=None, profile="full"):
        headers = {"Authorization": "Bearer " + keys[profile],
                   "Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
        data = None if method is None else json.dumps(
            {"jsonrpc": "2.0", "id": 1, "method": method, "params": params or {}}).encode()
        req = urllib.request.Request(base + path, data=data, headers=headers)
        try:
            response = urllib.request.urlopen(req, timeout=15)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read().decode()
            if response.headers.get_content_type() == "text/event-stream":
                raw = next(line[6:] for line in raw.splitlines() if line.startswith("data: "))
            return response.status, json.loads(raw)

    def call(path, profile, name, arguments):
        status, body = request(path, "tools/call", {"name": name, "arguments": arguments}, profile)
        return status, body.get("result", {}), "error" not in body and "result" in body

    env = {name: os.environ[name] for name in ("PATH", "HOME", "TMPDIR") if name in os.environ}
    log_path = args.work_dir / "gateway.log"
    log_path.touch(mode=0o600)
    with log_path.open("w") as log:
        process = subprocess.Popen([str(args.binary.resolve()), "-host", "127.0.0.1",
                                    "-port", str(args.port), "-app-dir", str(app_dir.resolve()),
                                    "-log-level", "warn"], stdout=log, stderr=log,
                                   env=env, start_new_session=True)
        try:
            deadline = time.monotonic() + 90
            while True:
                if process.poll() is not None:
                    raise RuntimeError("candidate exited during startup; inspect the private gateway log")
                try:
                    with urllib.request.urlopen(base + "/health", timeout=1) as response:
                        if response.status == 200:
                            break
                except (OSError, urllib.error.URLError):
                    pass
                if time.monotonic() >= deadline:
                    raise TimeoutError("candidate health did not become ready within 90 seconds")
                time.sleep(0.25)
            check("candidate healthy on first start", True)
            status, body = request("/api/mcp/virtual-mcps/900001")
            attached = body.get("virtual_mcp", {}).get("virtual_key_ids", [])
            check("both VK attachments exist without restart", status == 200 and set(attached) == {"vk-full", "vk-virtual"})
            init = {"protocolVersion": "2025-06-18", "clientInfo": {"name": "candidate-qa", "version": "1"},
                    "capabilities": {"extensions": {"io.modelcontextprotocol/ui": {
                        "mimeTypes": ["text/html;profile=mcp-app"]}}}}
            for route, profile in (("/mcp/debug", "full"), ("/mcp/candidate-apps", "virtual"), ("/mcp", "virtual")):
                status, body = request(route, "initialize", init, profile)
                check(route + " initialize", status == 200 and "result" in body)
                if "result" in body:
                    report["server"] = body["result"].get("serverInfo")
                status, body = request(route, "tools/list", profile=profile)
                tools = body.get("result", {}).get("tools", [])
                check(route + " exposes Debug and Code Mode", status == 200 and
                      {"debug-debug-tool", "executeToolCode"}.issubset({t["name"] for t in tools}))
                app = next((t for t in tools if t["name"] == "debug-debug-tool"), {})
                uri = app.get("_meta", {}).get("ui", {}).get("resourceUri")
                if uri:
                    status, body = request(route, "resources/read", {"uri": uri}, profile)
                    contents = body.get("result", {}).get("contents", [])
                    valid_html = status == 200 and any(c.get("mimeType") == "text/html;profile=mcp-app" and c.get("text") for c in contents)
                else:
                    valid_html = False
                check(route + " preserves App resource HTML", valid_html)
                for failed in (False, True):
                    status, result, valid = call(route, profile, "debug-debug-tool",
                        {"contentType": "text", "simulateError": failed, "includeStructuredContent": True})
                    check(route + f" native tool error={failed}", status == 200 and valid and bool(result.get("isError")) == failed)
                    if not failed:
                        check(route + " native structured result retained", bool(result.get("structuredContent")))
                for label, code, failed in (
                    ("runtime failure", "result = definitely_not_defined + 1", True),
                    ("nested failure", "result = debug.debug_tool(contentType=\"text\", simulateError=True)", True),
                    ("nested success", "result = debug.debug_tool(contentType=\"text\", simulateError=False)", False),
                    ("simple success", "result = 42", False),
                ):
                    status, result, valid = call(route, profile, "executeToolCode", {"code": code})
                    text = "\n".join(c.get("text", "") for c in result.get("content", []))
                    check(route + " " + label, status == 200 and valid and bool(result.get("isError")) == failed
                          and ("Execution completed successfully" in text) != failed)
            for route in ("/mcp/debug", "/mcp/candidate-apps"):
                status, _ = request(route, "tools/list", profile="denied")
                check(route + " rejects ungranted key", status in (401, 403))
            status, body = request("/mcp", "tools/list", profile="denied")
            check("root hides Debug from ungranted key", status == 200 and "result" in body
                  and not any(t["name"].startswith("debug-") for t in body["result"].get("tools", [])))
        except Exception as error:
            report["runner_error"] = type(error).__name__
            check("qualification completed", False)
        finally:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGTERM)
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait()
            report["gateway_stopped"] = process.poll() is not None
    report["passed"] = sum(c["passed"] for c in report["checks"])
    report["total"] = len(report["checks"])
    (args.work_dir / "report.json").write_text(json.dumps(report, indent=2) + "\n")
    print(f"{report['passed']}/{report['total']} checks passed; gateway stopped={report['gateway_stopped']}")
    return 0 if report["passed"] == report["total"] else 1


if __name__ == "__main__":
    raise SystemExit(main())
