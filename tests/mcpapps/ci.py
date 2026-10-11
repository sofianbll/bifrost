#!/usr/bin/env python3
"""Run real MCP Apps qualification against the built Bifrost image in CI.

The gateway image stays unchanged. Three pinned npm servers run as HTTP sidecars
on an isolated Docker network; Excalidraw uses its public MCP endpoint.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import subprocess
import sys
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parent
MANIFEST = ROOT / "real-apps.json"
NODE_IMAGE = "node:25-alpine3.23@sha256:bdf2cca6fe3dabd014ea60163eca3f0f7015fbd5c7ee1b0e9ccb4ced6eb02ef4"
PDF_URL = "https://arxiv.org/pdf/1706.03762"
PROTOCOL = "2025-06-18"


def load_manifest():
    data = json.loads(MANIFEST.read_text())
    assert set(data["apps"]) == {"excalidraw", "pdf", "debug", "monitor"}
    assert len(data["routes"]["direct"]) == 4 and len(data["routes"]["code_mode"]) == 4
    assert data["pdf_tool_execution_timeout"] == "60s" and data["pdf_long_poll_seconds"] == 30
    for app in data["apps"].values():
        assert app["native_name"] and app["native_arguments"] is not None and app["code"]
    for name in ("pdf", "debug", "monitor"):
        assert data["apps"][name]["version"] == "2.0.3"
    return data


def rpc(base_url, path, method, params, token, timeout=90):
    request = urllib.request.Request(
        base_url.rstrip("/") + path,
        data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode(),
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json",
                 "Accept": "application/json, text/event-stream"},
    )
    try:
        response = urllib.request.urlopen(request, timeout=timeout)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        if response.headers.get_content_type() == "text/event-stream":
            raw = next(line[6:] for line in raw.splitlines() if line.startswith("data: "))
        return response.status, json.loads(raw)


def result_ok(status, body):
    result = body.get("result")
    return status == 200 and isinstance(result, dict) and not result.get("isError", False)


def gateway_command(image, name, network, app_dir):
    return ["docker", "run", "-d", "--user", f"{os.getuid()}:{os.getgid()}", "--name", name, "--network", network,
            "-p", "127.0.0.1::8080", "-v", f"{app_dir}:/app/data", image]


def mapped_port(text):
    return int(text.strip().splitlines()[0].rsplit(":", 1)[1])


def qualify(base_url, credentials, manifest, report):
    checks = report["checks"]

    def check(name, passed, details=None):
        item = {"name": name, "passed": bool(passed)}
        if details:
            item.update(details)
        checks.append(item)
        print(("PASS " if passed else "FAIL ") + name, flush=True)

    token = lambda route: credentials["routes"][route]["headers"]["Authorization"].removeprefix("Bearer ")
    init = {"protocolVersion": PROTOCOL, "clientInfo": {"name": "mcp-apps-ci", "version": "1"},
            "capabilities": {"extensions": {"io.modelcontextprotocol/ui": {
                "mimeTypes": ["text/html;profile=mcp-app"]}}}}
    for route_kind, paths in (("direct", manifest["routes"]["direct"]),
                              ("virtual", {app: manifest["routes"]["virtual"] for app in manifest["apps"]}),
                              ("aggregate", {app: manifest["routes"]["aggregate"] for app in manifest["apps"]})):
        for app, path in paths.items():
            name = manifest["apps"][app]["native_name"]
            key = "direct" if route_kind == "direct" else route_kind
            status, body = rpc(base_url, path, "initialize", init, token(key))
            check(f"{route_kind}/{app}: initialize", result_ok(status, body), {"http_status": status})
            status, body = rpc(base_url, path, "tools/list", {}, token(key))
            tools = body.get("result", {}).get("tools", [])
            item = next((tool for tool in tools if tool.get("name") == name), None)
            check(f"{route_kind}/{app}: native App tool listed", result_ok(status, body) and item is not None,
                  {"http_status": status})
            uri = item.get("_meta", {}).get("ui", {}).get("resourceUri") if item else None
            check(f"{route_kind}/{app}: App resource advertised", bool(uri), {"has_resource_uri": bool(uri)})
            if uri:
                status, body = rpc(base_url, path, "resources/read", {"uri": uri}, token(key))
                contents = body.get("result", {}).get("contents", [])
                html = any(c.get("mimeType") == "text/html;profile=mcp-app" and c.get("text") for c in contents)
                check(f"{route_kind}/{app}: resource HTML", result_ok(status, body) and html,
                      {"http_status": status, "content_count": len(contents)})
            status, body = rpc(base_url, path, "tools/call",
                               {"name": name, "arguments": manifest["apps"][app]["native_arguments"]}, token(key))
            result = body.get("result", {})
            valid_native = result_ok(status, body) and bool(result.get("content"))
            if app == "excalidraw":
                valid_native = valid_native and bool(result.get("structuredContent", {}).get("checkpointId"))
            if app == "pdf":
                valid_native = valid_native and bool(result.get("structuredContent", {}).get("viewUUID"))
            if app == "debug":
                valid_native = valid_native and bool(result.get("structuredContent"))
            if app == "monitor":
                valid_native = valid_native and bool(result.get("structuredContent", {}).get("hostname"))
            check(f"{route_kind}/{app}: native tool call", valid_native, {"http_status": status,
                  "content_blocks": len(result.get("content", [])), "has_structured_content": bool(result.get("structuredContent"))})
    for app, path in manifest["routes"]["code_mode"].items():
        key = "code_mode"
        name = manifest["apps"][app]["native_name"].removeprefix(app + "-")
        name = manifest["apps"][app]["code_namespace"] + "-" + name
        status, body = rpc(base_url, path, "initialize", init, token(key))
        check(f"code-mode/{app}: initialize", result_ok(status, body), {"http_status": status})
        status, body = rpc(base_url, path, "tools/list", {}, token(key))
        tools = {tool.get("name") for tool in body.get("result", {}).get("tools", [])}
        check(f"code-mode/{app}: native tool and executeToolCode listed", result_ok(status, body) and
              name in tools and "executeToolCode" in tools, {"http_status": status})
        status, body = rpc(base_url, path, "tools/call", {"name": name,
                           "arguments": manifest["apps"][app]["native_arguments"]}, token(key))
        result = body.get("result", {})
        check(f"code-mode/{app}: direct native call", result_ok(status, body) and bool(result.get("content")),
              {"http_status": status})
        status, body = rpc(base_url, path, "tools/call", {"name": "executeToolCode",
                           "arguments": {"code": manifest["apps"][app]["code"]}}, token(key))
        result = body.get("result", {})
        text = "\n".join(block.get("text", "") for block in result.get("content", []) if block.get("type") == "text")
        valid_text = (result_ok(status, body) and not result.get("_meta") and
                      not result.get("structuredContent") and "Print output:" in text and
                      "Execution completed successfully." in text)
        check(f"code-mode/{app}: nested App result is text", valid_text,
              {"http_status": status, "is_error": bool(result.get("isError")),
               "has_structured_content": bool(result.get("structuredContent")), "has_meta": bool(result.get("_meta")),
               "text_sha256": hashlib.sha256(text.encode()).hexdigest() if text else None})
    code_path = manifest["routes"]["code_mode"]["debug"]
    status, body = rpc(base_url, code_path, "tools/call", {"name": "executeToolCode",
                       "arguments": {"code": 'result = code_debug.debug_tool(contentType="text", simulateError=True)'}}, token("code_mode"))
    result = body.get("result", {})
    text = "\n".join(block.get("text", "") for block in result.get("content", []) if block.get("type") == "text")
    check("code-mode/debug: nested tool error remains a value", result_ok(status, body) and
          "Execution completed successfully." in text and "error" in text.lower() and
          not result.get("_meta") and not result.get("structuredContent"),
          {"http_status": status, "is_error": bool(result.get("isError")),
           "text_sha256": hashlib.sha256(text.encode()).hexdigest() if text else None})
    status, body = rpc(base_url, code_path, "tools/call", {"name": "executeToolCode",
                       "arguments": {"code": "result = definitely_not_defined + 1"}}, token("code_mode"))
    result = body.get("result", {})
    check("code-mode/debug: script failure is an MCP error", status == 200 and result.get("isError") is True,
          {"http_status": status, "is_error": bool(result.get("isError"))})


def build_config(manifest, credentials):
    key_values = {name: "sk-bf-" + secrets.token_hex(24)
                  for name in ("direct", "virtual", "aggregate", "code_mode", "limited")}
    clients = []
    for app, entry in manifest["apps"].items():
        normal_name = app
        if app == "excalidraw":
            upstream = {"connection_type": "http", "connection_string": entry["connection_string"]}
        else:
            upstream = {"connection_type": "http", "connection_string": f"http://{app}:{entry['port']}/mcp"}
        clients.append({"client_id": "ci-" + app, "name": normal_name, "endpoint_slug": app,
                        **upstream, "auth_type": "none", "allow_by_default": False,
                        "tools_to_execute": ["*"], "needs_session_stickiness": True,
                        **({"tool_execution_timeout": manifest["pdf_tool_execution_timeout"]} if app == "pdf" else {})})
        code_name = entry["code_namespace"]
        clients.append({"client_id": "ci-" + code_name, "name": code_name,
                        "endpoint_slug": manifest["routes"]["code_mode"][app].split("/mcp/")[1],
                        **upstream, "auth_type": "none", "allow_by_default": False,
                        "tools_to_execute": ["*"], "needs_session_stickiness": True,
                        "is_code_mode_client": True,
                        **({"tool_execution_timeout": manifest["pdf_tool_execution_timeout"]} if app == "pdf" else {})})
    direct_grants = [{"mcp_client_name": app, "tools_to_execute": ["*"]} for app in manifest["apps"]]
    code_grants = [{"mcp_client_name": entry["code_namespace"], "tools_to_execute": ["*"]}
                   for entry in manifest["apps"].values()]
    virtual_tools = [{"mcp_client_name": app, "tool_names": ["*"]} for app in manifest["apps"]]
    keys = []
    for name, value in key_values.items():
        grants = (direct_grants if name in ("direct", "aggregate") else
                  code_grants if name == "code_mode" else
                  [{"mcp_client_name": "excalidraw", "tools_to_execute": ["read_me"]}]
                  if name == "limited" else [])
        keys.append({"id": "vk-" + name, "name": name, "value": value, "is_active": True,
                     "provider_configs": [], "mcp_configs": grants})
    credentials["routes"] = {
        name: {"headers": {"Authorization": "Bearer " + key_values[name]}}
        for name in ("direct", "virtual", "aggregate", "code_mode")
    }
    credentials["_smoke_keys"] = {"full": key_values["direct"], "limited": key_values["limited"],
                                   "denied": "sk-bf-" + secrets.token_hex(24),
                                   "invalid": "sk-bf-" + secrets.token_hex(24)}
    credentials["secrets"] = list(key_values.values()) + [credentials["_smoke_keys"][name]
                                                          for name in ("denied", "invalid")]
    return {
        "client": {"enable_logging": False, "enforce_governance_header": True, "enforce_auth_on_inference": True},
        "providers": {},
        "mcp": {
            "client_configs": clients,
            "tool_manager_config": {"tool_execution_timeout": "60s", "code_mode_binding_level": "server"},
            "virtual_mcps": [{"id": 900012, "name": "CI MCP Apps", "endpoint_slug": "codeapps",
                              "tools": virtual_tools, "virtual_key_ids": ["vk-virtual", "vk-aggregate"]}],
        },
        "governance": {"virtual_keys": keys},
    }


def run(command, *, capture=True):
    try:
        return subprocess.run(command, check=True, text=True, stdout=subprocess.PIPE if capture else None,
                              stderr=subprocess.PIPE if capture else None)
    except (subprocess.CalledProcessError, OSError) as error:
        raise RuntimeError(type(error).__name__) from None


def write_private(path, data):
    path.write_text(json.dumps(data, indent=2) + "\n")
    path.chmod(0o600)


def cleanup_resources(started, network_created, image_built, network, image, command=run):
    errors = []
    for name in reversed(started):
        try:
            command(["docker", "rm", "-f", name])
        except RuntimeError:
            errors.append("container")
    if network_created:
        try:
            command(["docker", "network", "rm", network])
        except RuntimeError:
            errors.append("network")
    if image_built:
        try:
            command(["docker", "image", "rm", image])
        except RuntimeError:
            errors.append("image")
    return errors


def self_check():
    manifest = load_manifest()
    assert sum(len(manifest["routes"][kind]) for kind in ("direct", "code_mode")) == 8
    assert len(manifest["routes"]["direct"]) * 3 + len(manifest["routes"]["code_mode"]) == 16
    assert all(manifest["apps"][name]["package"] and manifest["apps"][name]["version"] == "2.0.3"
               for name in ("pdf", "debug", "monitor"))
    assert not any("token" in k.lower() or "credential" in k.lower() for k in manifest)
    fake_credentials = {}
    config = build_config(manifest, fake_credentials)
    schema = json.loads((ROOT.parents[1] / "transports/config.schema.json").read_text())
    client_properties = schema["$defs"]["mcp_client_config"]["properties"]
    virtual_properties = schema["$defs"]["virtual_mcp_config"]["properties"]
    key_properties = schema["properties"]["governance"]["properties"]["virtual_keys"]["items"]["properties"]
    assert all(set(client) <= set(client_properties) for client in config["mcp"]["client_configs"])
    assert set(config["mcp"]["virtual_mcps"][0]) <= set(virtual_properties)
    assert all(set(key) <= set(key_properties) for key in config["governance"]["virtual_keys"])
    assert config["mcp"]["tool_manager_config"]["tool_execution_timeout"] == "60s"
    assert next(c for c in config["mcp"]["client_configs"] if c["name"] == "pdf")["tool_execution_timeout"] == "60s"
    assert set(fake_credentials["routes"]) == {"direct", "virtual", "aggregate", "code_mode"}
    assert (ROOT / "smoke.py").is_file()
    assert set(fake_credentials["_smoke_keys"]) == {"full", "limited", "denied", "invalid"}
    assert all(value not in json.dumps(manifest) for value in fake_credentials["secrets"])
    assert len(config["mcp"]["client_configs"]) == 8 and len(config["mcp"]["virtual_mcps"][0]["tools"]) == 4
    assert len(config["governance"]["virtual_keys"]) == 5
    assert all(key["value"].startswith("sk-bf-") for key in config["governance"]["virtual_keys"])
    limited = next(key for key in config["governance"]["virtual_keys"] if key["name"] == "limited")
    assert limited["mcp_configs"] == [{"mcp_client_name": "excalidraw", "tools_to_execute": ["read_me"]}]
    command = gateway_command("candidate", "gateway", "net", Path("/tmp/private"))
    assert command[command.index("-p") + 1] == "127.0.0.1::8080"
    assert command[command.index("--user") + 1] == f"{os.getuid()}:{os.getgid()}"
    assert mapped_port("127.0.0.1:49152\n") == 49152
    assert result_ok(200, {"result": {"isError": False}})
    assert not result_ok(200, {"result": {"isError": True}})
    attempts = []
    def fake_remove(resource):
        attempts.append(resource)
        if resource == "debug":
            raise RuntimeError("simulated container cleanup failure")
    errors = cleanup_resources(["gateway", "monitor", "debug", "pdf"], True, True,
                               "network", "sidecars", lambda cmd: fake_remove(cmd[-1]))
    assert attempts == ["pdf", "debug", "monitor", "gateway", "network", "sidecars"]
    assert errors == ["container"]
    print("PASS manifest pins, matrix, timeout, loopback binding, MCP errors and cleanup order")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--image", default="bifrost:candidate")
    parser.add_argument("--work-dir", type=Path, help="Private temporary files; never upload this directory")
    parser.add_argument("--report-dir", type=Path, help="Redacted reports and screenshots only")
    parser.add_argument("--render-script", type=Path, help="Optional official reference-host Playwright runner")
    parser.add_argument("--reference-host-dir", type=Path, help="Pinned ext-apps checkout for a render script that accepts --host-dir")
    parser.add_argument("--node", default="node")
    parser.add_argument("--self-check", action="store_true")
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return 0
    if args.reference_host_dir and not args.reference_host_dir.is_dir():
        parser.error("--reference-host-dir must be an existing directory")
    if args.reference_host_dir and not args.render_script:
        parser.error("--reference-host-dir requires --render-script")
    if args.render_script and not args.reference_host_dir:
        parser.error("--render-script requires the pinned --reference-host-dir checkout root")
    if not args.work_dir or not args.report_dir:
        parser.error("--work-dir and --report-dir are required")
    manifest = load_manifest()
    args.work_dir.mkdir(parents=True, exist_ok=False, mode=0o700)
    args.report_dir.mkdir(parents=True, exist_ok=False, mode=0o755)
    app_dir = args.work_dir / "gateway-data"
    app_dir.mkdir(mode=0o700)
    image = "bifrost-mcpapps-servers:" + secrets.token_hex(6)
    network = "bifrost-mcpapps-" + secrets.token_hex(6)
    names = {"pdf": "mcpapps-pdf-" + secrets.token_hex(4),
             "debug": "mcpapps-debug-" + secrets.token_hex(4),
             "monitor": "mcpapps-monitor-" + secrets.token_hex(4),
             "gateway": "mcpapps-gateway-" + secrets.token_hex(4)}
    started, network_created, image_built = [], False, False
    report = {"mode": "real-app CI qualification", "image": args.image,
              "packages": {name: entry.get("package", "public endpoint") + ("@" + entry["version"] if entry.get("version") else "")
                           for name, entry in manifest["apps"].items()}, "checks": []}
    credentials = {}
    code = 1
    try:
        run(["docker", "image", "inspect", args.image])
        run(["docker", "network", "create", network])
        network_created = True
        context = args.work_dir / "servers"
        context.mkdir()
        package_names = [entry["package"] + "@" + entry["version"] for entry in manifest["apps"].values()
                         if entry.get("package")]
        dockerfile = "FROM " + NODE_IMAGE + "\nRUN npm install --global --omit=dev " + " ".join(package_names) + "\n"
        (context / "Dockerfile").write_text(dockerfile)
        run(["docker", "build", "--pull", "-t", image, str(context)])
        image_built = True
        for app, package_bin in (("pdf", "mcp-pdf-server"), ("debug", "mcp-server-debug"),
                                 ("monitor", "mcp-system-monitor-server")):
            entry = manifest["apps"][app]
            command = ["docker", "run", "-d", "--name", names[app], "--network", network,
                       "--network-alias", app, "-e", f"PORT={entry['port']}", image, package_bin]
            if app == "pdf":
                command.append("--enable-interact")
            run(command)
            started.append(names[app])
        config = build_config(manifest, credentials)
        write_private(app_dir / "config.json", config)
        # Match the runner UID so its private data remains removable after the gateway exits.
        run(gateway_command(args.image, names["gateway"], network, app_dir))
        started.append(names["gateway"])
        port_text = run(["docker", "port", names["gateway"], "8080/tcp"]).stdout
        port = mapped_port(port_text)
        base_url = f"http://127.0.0.1:{port}"
        deadline = time.monotonic() + 120
        while True:
            try:
                with urllib.request.urlopen(base_url + "/health", timeout=2) as response:
                    if response.status == 200:
                        break
            except (OSError, urllib.error.URLError):
                pass
            if time.monotonic() >= deadline:
                raise TimeoutError("candidate health check timed out")
            time.sleep(0.5)
        report["base_url"] = base_url
        report["gateway_image_running"] = True
        credentials_path = args.work_dir / "credentials.json"
        write_private(credentials_path, credentials)
        qualification_manifest = {
            "base_url": base_url,
            "apps": {name: {key: value for key, value in entry.items()
                            if key in ("native_name", "native_arguments", "html_resource_required")}
                     for name, entry in manifest["apps"].items()},
            "routes": manifest["routes"],
            "credentials_file": str(credentials_path),
        }
        qualification_path = args.work_dir / "qualification-manifest.json"
        write_private(qualification_path, qualification_manifest)
        qualify(base_url, credentials, manifest, report)
        smoke_keys_path = args.work_dir / "smoke-keys.json"
        smoke_report_path = args.work_dir / "smoke-report.json"
        write_private(smoke_keys_path, credentials["_smoke_keys"])
        smoke_result = subprocess.run([sys.executable, str(ROOT / "smoke.py"), "--url", base_url,
                                       "--keys", str(smoke_keys_path), "--output", str(smoke_report_path),
                                       "--expect-apps"], text=True, stdout=subprocess.PIPE,
                                      stderr=subprocess.PIPE)
        smoke_report = json.loads(smoke_report_path.read_text())
        smoke_passed = smoke_result.returncode == 0 and smoke_report["passed"] == smoke_report["total"]
        report["checks"].append({"name": "existing Excalidraw App probe", "passed": smoke_passed,
                                  "passed_checks": smoke_report["passed"], "total_checks": smoke_report["total"]})
        print(("PASS " if smoke_passed else "FAIL ") + "existing Excalidraw App probe", flush=True)
        if args.render_script:
            log_path = args.work_dir / "renderer.log"
            with log_path.open("w") as log:
                render_args = [args.node, str(args.render_script), "--manifest", str(qualification_path)]
                render_args.extend(["--host-dir", str(args.reference_host_dir), "--out", str(args.report_dir)])
                result = subprocess.run(render_args,
                                        stdout=log, stderr=log, text=True)
            report["render_exit_code"] = result.returncode
            report["checks"].append({"name": "reference-host render gate", "passed": result.returncode == 0})
            print(("PASS" if result.returncode == 0 else "FAIL") + " reference-host render gate", flush=True)
        code = 0 if all(item["passed"] for item in report["checks"]) else 1
    except Exception as error:
        report["runner_error"] = type(error).__name__
        report["checks"].append({"name": "qualification runner completed", "passed": False})
        print("FAIL qualification runner (" + type(error).__name__ + ")", flush=True)
    finally:
        cleanup_errors = cleanup_resources(started, network_created, image_built, network, image)
        try:
            shutil.rmtree(args.work_dir)
            private_files_removed = True
        except OSError:
            private_files_removed = False
        report["cleanup"] = {"containers_removed": len(started) - min(len(started), cleanup_errors.count("container")),
                              "network_removed": not network_created or "network" not in cleanup_errors,
                              "app_image_removed": not image_built or "image" not in cleanup_errors,
                              "private_files_removed": private_files_removed,
                              "passed": not cleanup_errors and private_files_removed}
        report["total"] = len(report["checks"])
        report["passed"] = sum(1 for item in report["checks"] if item["passed"])
        report["passed_all"] = code == 0 and report["cleanup"]["passed"] and report["passed"] == report["total"]
        (args.report_dir / "mcp-apps-qualification.json").write_text(json.dumps(report, indent=2) + "\n")
        if not report["passed_all"]:
            code = 1
    return code


if __name__ == "__main__":
    raise SystemExit(main())
