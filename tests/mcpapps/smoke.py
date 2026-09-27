#!/usr/bin/env python3
"""Compare the existing Excalidraw MCP Apps contract on an isolated gateway.

The key file maps full/limited/denied to temporary VKs. Limited allows only
read_me; denied has no MCP grants. No credentials or HTML are written to output.

This is a manual qualification probe. --code-mode deliberately includes the
known unsupported nested structured-result contract, so that mode can exit 1
on the published fork. It is not the release gate; keep its failed observation.
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
    parser.add_argument("--code-mode", action="store_true",
                        help="Probe Code Mode, including the known failing nested structured-result contract")
    parser.add_argument("--normal-matrix", action="store_true",
                        help="Verify direct, Virtual MCP and combined VK grants (normal mode only)")
    args = parser.parse_args()
    if args.normal_matrix and (args.code_mode or not args.expect_apps):
        parser.error("--normal-matrix requires --expect-apps and excludes --code-mode")
    keys = json.loads(args.keys.read_text())
    checks = []
    observations = {}

    def check(name, condition):
        checks.append({"name": name, "passed": bool(condition)})

    def rpc(method, params=None, key="full", path="/mcp/excalidraw"):
        headers = {"Content-Type": "application/json", "Accept": "application/json, text/event-stream"}
        if key is not None:
            headers["Authorization"] = "Bearer " + keys.get(key, "sk-bf-invalid-qa")
        request = urllib.request.Request(
            args.url.rstrip("/") + path,
            data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method,
                             "params": params or {}}).encode(),
            headers=headers,
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

    def call(name, arguments, key="full", path="/mcp/excalidraw"):
        return rpc("tools/call", {"name": name, "arguments": arguments}, key=key, path=path)

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
        if args.expect_apps:
            check("raw upstream UI metadata preserved", view.get("_meta", {}).get("ui/resourceUri") ==
                  "ui://excalidraw/mcp-app.html")
            for name in ("excalidraw-read_checkpoint", "excalidraw-save_checkpoint",
                         "excalidraw-export_to_excalidraw"):
                tool = next(t for t in tools if t["name"] == name)
                check(name + " hidden from model", tool.get("_meta", {}).get("ui", {}).get("visibility") == ["app"])
            check("create_view remains model-visible", "visibility" not in view.get("_meta", {}).get("ui", {}))

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
            check("resources/list returns an array", success(status, body) and
                  isinstance(body.get("result", {}).get("resources"), list))
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
        if args.expect_apps:
            status, body = call("no_such_tool", {})
            check("unknown tool returns protocol error", body.get("error", {}).get("code") == -32602)
            status, body = call("excalidraw-create_view", {"elements": "not-json"})
            check("upstream tool error remains isError", status == 200 and
                  body.get("result", {}).get("isError") is True)

        if args.multiple_clients:
            aggregate_key = "aggregate" if "aggregate" in keys else "full"
            status, body = rpc("tools/list", key=aggregate_key, path="/mcp")
            aggregate = body.get("result", {}).get("tools", [])
            check("aggregate lists both upstreams", success(status, body) and
                  all(any(t["name"].startswith(prefix) for t in aggregate) for prefix in ["excalidraw-", "second-"]))
            check("aggregate preserves admitted UI", any(
                t["name"] == "excalidraw-create_view" and
                t.get("_meta", {}).get("ui", {}).get("resourceUri") for t in aggregate))
            app_uris = {name: next((t.get("_meta", {}).get("ui", {}).get("resourceUri")
                                     for t in aggregate if t["name"] == name + "-create_view"), None)
                        for name in ("excalidraw", "second")}
            check("two sources have distinct rewritten UI URIs", all(app_uris.values()) and
                  len(set(app_uris.values())) == 2)
            for name, app_uri in app_uris.items():
                status, body = rpc("resources/read", {"uri": app_uri}, key=aggregate_key, path="/mcp")
                contents = body.get("result", {}).get("contents", [])
                check(name + " resource routes to HTML", success(status, body) and bool(contents) and
                      contents[0].get("uri") == app_uri and
                      contents[0].get("mimeType") == "text/html;profile=mcp-app" and
                      "<html" in contents[0].get("text", "").lower())
            status, body = rpc("resources/read", {"uri": app_uris["second"]}, key="full", path="/mcp")
            check("single-source key cannot read second source UI", not success(status, body))
            status, body = call("second-create_view", {"elements": "[]"}, key="full", path="/mcp")
            check("single-source key cannot call second source", not success(status, body))
            status, body = call("second-create_view", {"elements": "[]"}, key=aggregate_key, path="/mcp")
            second_checkpoint = body.get("result", {}).get("structuredContent", {}).get("checkpointId")
            check("second source native call preserves structured result", success(status, body) and bool(second_checkpoint))
            for name, source_checkpoint in (("excalidraw", checkpoint), ("second", second_checkpoint)):
                status, body = call(name + "-read_checkpoint", {"id": source_checkpoint},
                                    key=aggregate_key, path="/mcp")
                check(name + " namespaced callback routes", success(status, body))
            status, body = rpc("initialize", init, key=aggregate_key, path="/mcp")
            check("aggregate negotiates UI", "io.modelcontextprotocol/ui" in
                  body.get("result", {}).get("capabilities", {}).get("extensions", {}))
            status, body = rpc("tools/call", {"name": "read_checkpoint", "arguments": {"id": checkpoint}}, key=aggregate_key, path="/mcp")
            callback_sources = [t for t in aggregate if t["name"].endswith("-read_checkpoint")]
            check("aggregate routes only unambiguous callback", success(status, body) if len(callback_sources) == 1
                  else body.get("error", {}).get("code") == -32602)
            status, body = rpc("resources/read", {"uri": "ui://excalidraw/mcp-app.html"},
                               key=aggregate_key, path="/mcp")
            check("upstream UI URI cannot bypass gateway route", not success(status, body))

        if args.normal_matrix:
            # Fixture: full=excalidraw:*, limited=excalidraw:read_me;
            # codexmcp grants excalidraw:read_me + second:* to virtual and aggregate.
            # aggregate also owns excalidraw:* directly. Neither client uses Code Mode.
            originals = {"create_view", "read_me", "save_checkpoint", "read_checkpoint", "export_to_excalidraw"}
            first = {"excalidraw-" + name for name in originals}
            second = {"second-" + name for name in originals}
            narrow = {"excalidraw-read_me"}
            cases = [
                ("full", "/mcp/excalidraw", first),
                ("virtual", "/mcp/excalidraw", narrow),
                ("virtual", "/mcp/codexmcp", narrow | second),
                ("aggregate", "/mcp/codexmcp", narrow | second),
                ("full", "/mcp", first),
                ("virtual", "/mcp", narrow | second),
                ("aggregate", "/mcp", first | second),
                ("limited", "/mcp", narrow),
            ]
            matrix = observations["normal_matrix"] = []
            for key, path, expected in cases:
                status, body = rpc("tools/list", key=key, path=path)
                listed = body.get("result", {}).get("tools", [])
                names = [t["name"] for t in listed]
                # Original callback aliases are additional app-only entries for a single source.
                aliases = [t for t in listed if t["name"] in originals]
                actual = set(names) - {t["name"] for t in aliases}
                matrix.append({"key_profile": key, "path": path, "http_status": status,
                               "tools": names, "expected_namespaced_tools": sorted(expected),
                               "error": body.get("error")})
                check(f"normal {key} {path}: exact grants without duplicate names",
                      success(status, body) and actual == expected and len(names) == len(set(names)) and
                      all(t.get("_meta", {}).get("ui", {}).get("visibility") == ["app"] for t in aliases))
                status, body = rpc("initialize", init, key=key, path=path)
                check(f"normal {key} {path}: initializes", success(status, body))
                for tool in listed:
                    app_uri = tool.get("_meta", {}).get("ui", {}).get("resourceUri")
                    if not app_uri:
                        continue
                    status, body = rpc("resources/read", {"uri": app_uri}, key=key, path=path)
                    content = body.get("result", {}).get("contents", [])
                    check(f"normal {key} {path}: {tool['name']} HTML", success(status, body) and
                          bool(content) and content[0].get("uri") == app_uri and
                          content[0].get("mimeType") == "text/html;profile=mcp-app")

            for key in ["full", "limited", "denied", "invalid", None]:
                status, body = rpc("tools/list", key=key, path="/mcp/codexmcp")
                matrix.append({"key_profile": key or "absent", "path": "/mcp/codexmcp",
                               "http_status": status, "error": body.get("error")})
                check(f"normal {key} cannot enter unassigned Virtual MCP", status in [401, 403])

            for key, path in [("limited", "/mcp"), ("virtual", "/mcp"),
                              ("aggregate", "/mcp/codexmcp")]:
                for name in ["excalidraw-create_view", "excalidraw-save_checkpoint", "excalidraw-read_checkpoint"]:
                    status, body = call(name, {"elements": "[]", "id": checkpoint, "data": "[]"}, key, path)
                    check(f"normal {key} {path}: denied {name}", not success(status, body))
                status, body = rpc("resources/read", {"uri": uri}, key=key, path=path)
                check(f"normal {key} {path}: denied excalidraw UI", not success(status, body))

            for path in ["/mcp", "/mcp/excalidraw", "/mcp/codexmcp"]:
                for key in ["denied", "invalid", None]:
                    for method, params in [("tools/list", {}), ("resources/read", {"uri": uri}),
                                           ("tools/call", {"name": "read_checkpoint", "arguments": {"id": checkpoint}})]:
                        status, body = rpc(method, params, key=key, path=path)
                        matrix.append({"key_profile": key or "absent", "path": path, "method": method,
                                       "http_status": status, "response": body})
                        # A valid key with no grants may discover an empty root catalogue.
                        empty_catalogue = (key == "denied" and path == "/mcp" and method == "tools/list" and
                                           success(status, body) and body["result"].get("tools") == [])
                        check(f"normal {key} {path}: no access through {method}",
                              not success(status, body) or empty_catalogue)

            for key, path, prefix in [("full", "/mcp/excalidraw", "excalidraw"),
                                      ("virtual", "/mcp/codexmcp", "second"),
                                      ("aggregate", "/mcp", "second")]:
                status, body = call(prefix + "-create_view", {"elements": "[]"}, key, path)
                saved_id = body.get("result", {}).get("structuredContent", {}).get("checkpointId")
                check(f"normal {key} {path}: structured create result", success(status, body) and bool(saved_id))
                if not saved_id:
                    continue
                payload = json.dumps([{"type": "rectangle", "id": "normal-mode-qa", "x": 17, "y": 23,
                                       "width": 100, "height": 60, "label": {"text": path}}])
                status, body = call(prefix + "-save_checkpoint", {"id": saved_id, "data": payload}, key, path)
                check(f"normal {key} {path}: save callback", success(status, body))
                status, body = call(prefix + "-read_checkpoint", {"id": saved_id}, key, path)
                texts = [p.get("text") for p in body.get("result", {}).get("content", []) if p.get("type") == "text"]
                matrix.append({"key_profile": key, "path": path, "callback_readback": texts,
                               "expected_saved_data": json.loads(payload)})
                check(f"normal {key} {path}: exact saved data readback", success(status, body) and
                      any(json.loads(t) == json.loads(payload) for t in texts))

        if args.code_mode:
            status, body = rpc("tools/list", key="codeapp", path="/mcp/codeapp")
            code_tools = body.get("result", {}).get("tools", [])
            check("Code Mode lists native app and helper", success(status, body) and
                  {"codeapp-create_view", "executeToolCode"} <= {t["name"] for t in code_tools})
            code_uri = next((t.get("_meta", {}).get("ui", {}).get("resourceUri")
                             for t in code_tools if t["name"] == "codeapp-create_view"), None)
            status, body = rpc("resources/read", {"uri": code_uri}, key="codeapp", path="/mcp/codeapp")
            contents = body.get("result", {}).get("contents", [])
            check("Code Mode app resource remains available", success(status, body) and bool(contents) and
                  contents[0].get("mimeType") == "text/html;profile=mcp-app")
            status, body = call("codeapp-create_view", {"elements": "[]"}, key="codeapp", path="/mcp/codeapp")
            check("Code Mode direct app call preserves structured result", success(status, body) and
                  bool(body.get("result", {}).get("structuredContent", {}).get("checkpointId")))
            status, body = call("executeToolCode", {"code": 'result = codeapp.create_view(elements="[]")'},
                                key="codeapp", path="/mcp/codeapp")
            nested = body.get("result", {})
            observations["nested_code_mode"] = {"http_status": status, "is_error": nested.get("isError"),
                                                  "content_types": [part.get("type") for part in nested.get("content", [])],
                                                  "structured_keys": list(nested.get("structuredContent", {}))}
            check("Code Mode nested app call preserves structured result", success(status, body) and
                  bool(nested.get("structuredContent", {}).get("checkpointId")))
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
