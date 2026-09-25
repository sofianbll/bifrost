#!/usr/bin/env python3
"""Compare the existing Excalidraw MCP Apps contract on an isolated gateway.

The key file maps full/limited/denied to temporary VKs. Limited allows only
read_me; denied has no MCP grants. No credentials or HTML are written to output.
"""
import argparse
import json
import urllib.error
import urllib.request
from pathlib import Path


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--url", required=True)
    parser.add_argument("--keys", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--expect-apps", action="store_true")
    parser.add_argument("--multiple-clients", action="store_true")
    args = parser.parse_args()
    keys = json.loads(args.keys.read_text())
    checks = []
    observations = {}

    def check(name, condition):
        checks.append({"name": name, "passed": bool(condition)})

    def rpc(method, params=None, key="full", path="/mcp/excalidraw"):
        request = urllib.request.Request(
            args.url.rstrip("/") + path,
            data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method,
                             "params": params or {}}).encode(),
            headers={"Content-Type": "application/json",
                     "Accept": "application/json, text/event-stream",
                     "Authorization": "Bearer " + keys.get(key, "sk-bf-invalid-qa")},
        )
        try:
            response = urllib.request.urlopen(request, timeout=40)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read().decode()
            if response.headers.get_content_type() == "text/event-stream":
                raw = next(line[6:] for line in raw.splitlines() if line.startswith("data: "))
            return response.status, json.loads(raw)

    def call(name, arguments):
        return rpc("tools/call", {"name": name, "arguments": arguments})

    def success(status, body):
        return status == 200 and "result" in body and not body["result"].get("isError")

    init = {"protocolVersion": "2025-06-18", "clientInfo": {"name": "mcp-apps-qa", "version": "1"},
            "capabilities": {"extensions": {"io.modelcontextprotocol/ui": {
                "mimeTypes": ["text/html;profile=mcp-app"]}}}}
    try:
        status, body = rpc("initialize", init)
        check("initialize succeeds", success(status, body))
        result = body.get("result", {})
        observations["server"] = result.get("serverInfo")
        observations["capabilities"] = result.get("capabilities")
        caps = result.get("capabilities", {})
        has_ui = "io.modelcontextprotocol/ui" in caps.get("extensions", {})
        check("UI negotiation matches variant", has_ui == args.expect_apps)

        status, body = rpc("tools/list")
        tools = body.get("result", {}).get("tools", [])
        observations["tools"] = [{"name": t["name"], "meta": t.get("_meta")} for t in tools]
        check("tools/list succeeds", success(status, body))
        view = next(t for t in tools if t["name"] == "excalidraw-create_view")
        uri = view.get("_meta", {}).get("ui", {}).get("resourceUri")
        check("UI resource metadata matches variant", bool(uri) == args.expect_apps)

        status, body = call("excalidraw-read_me", {})
        check("read_me succeeds", success(status, body))
        status, body = call("create_view" if args.expect_apps else "excalidraw-create_view", {"elements": "[]"})
        check("create_view succeeds", success(status, body))
        checkpoint = body.get("result", {}).get("structuredContent", {}).get("checkpointId")
        observations["structured_checkpoint"] = bool(checkpoint)
        check("structured result matches variant", bool(checkpoint) == args.expect_apps)

        status, body = rpc("resources/list")
        observations["resources_list_error"] = body.get("error")
        if args.expect_apps:
            check("resources/list is supported", success(status, body))
            status, body = rpc("resources/templates/list")
            check("UI resource template advertised", success(status, body) and
                  "ui://bifrost/{client}/{resource}" in
                  [r["uriTemplate"] for r in body["result"].get("resourceTemplates", [])])
            status, body = rpc("resources/read", {"uri": uri})
            contents = body.get("result", {}).get("contents", [])
            check("resources/read returns HTML and MIME", success(status, body) and bool(contents) and
                  contents[0].get("uri") == uri and contents[0].get("mimeType") == "text/html;profile=mcp-app" and
                  "<html" in contents[0].get("text", "").lower())
            for callback in ["save_checkpoint", "read_checkpoint"]:
                tool = next(t for t in tools if t["name"] == callback)
                check(callback + " is app-only", tool.get("_meta", {}).get("ui", {}).get("visibility") == ["app"])
                arguments = {"id": checkpoint}
                if callback == "save_checkpoint":
                    arguments["data"] = "[]"
                status, body = call(callback, arguments)
                check(callback + " original-name callback succeeds", success(status, body))
            status, body = rpc("resources/read", {"uri": "ui://bifrost/unknown/unknown"})
            check("unknown resource rejected", status != 200 or "error" in body)
        else:
            check("official resources/list remains unsupported", body.get("error", {}).get("code") == -32601)
            status, body = rpc("resources/read", {"uri": "ui://excalidraw/mcp-app.html"})
            observations["resources_read_error"] = body.get("error")
            check("official resources/read remains unsupported", body.get("error", {}).get("code") == -32601)

        status, body = rpc("tools/list", key="limited")
        limited = body.get("result", {}).get("tools", [])
        check("limited key sees only read_me", success(status, body) and
              {t["name"] for t in limited} == {"excalidraw-read_me"})
        for name in ["excalidraw-create_view", "create_view", "read_checkpoint", "save_checkpoint"]:
            status, body = rpc("tools/call", {"name": name, "arguments": {"elements": "[]"}}, key="limited")
            check("limited key cannot call " + name, not success(status, body))
        if uri:
            status, body = rpc("resources/read", {"uri": uri}, key="limited")
            check("limited key cannot read UI", not success(status, body))
        for key in ["denied", "invalid"]:
            status, body = rpc("tools/list", key=key)
            check(key + " key rejected", status in [401, 403])

        if args.multiple_clients:
            status, body = rpc("tools/list", path="/mcp")
            aggregate = body.get("result", {}).get("tools", [])
            check("aggregate lists both upstreams", success(status, body) and
                  all(any(t["name"].startswith(prefix) for t in aggregate) for prefix in ["excalidraw-", "second-"]))
            check("aggregate exposes no UI or app-only callbacks", all(
                not t.get("_meta", {}).get("ui", {}).get("resourceUri") and
                t.get("_meta", {}).get("ui", {}).get("visibility") != ["app"] for t in aggregate))
            status, body = rpc("initialize", init, path="/mcp")
            check("aggregate does not negotiate UI", "io.modelcontextprotocol/ui" not in
                  body.get("result", {}).get("capabilities", {}).get("extensions", {}))
            status, body = rpc("tools/call", {"name": "read_checkpoint", "arguments": {"id": checkpoint}}, path="/mcp")
            check("aggregate rejects unscoped callback", not success(status, body))
    except Exception as error:
        # Only the exception type is persisted: HTTP errors must not leak credentials.
        check("completed all probes (" + type(error).__name__ + ")", False)
    finally:
        report = {"url": args.url, "expect_apps": args.expect_apps, "observations": observations,
                  "checks": checks, "passed": sum(c["passed"] for c in checks), "total": len(checks)}
        args.output.write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps({k: report[k] for k in ["url", "passed", "total"]}))
        for item in checks:
            if not item["passed"]:
                print("FAIL:", item["name"])
    return 0 if checks and all(c["passed"] for c in checks) else 1


if __name__ == "__main__":
    raise SystemExit(main())
