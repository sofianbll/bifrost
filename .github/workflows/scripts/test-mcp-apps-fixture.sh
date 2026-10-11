#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
export BIFROST_REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd -P)"
exec python3 - "$@" <<'PY'
import base64
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(os.environ["BIFROST_REPO_ROOT"])
FIXTURE_NAMES = ("mcp_apps_fixture", "mcp_apps_fixture_second", "errorTestServer")
KEYS = {
    "full": "sk-bf-mcp-apps-full",
    "limited": "sk-bf-mcp-apps-limited",
    "limited_no_app": "sk-bf-mcp-apps-limited-no-app",
    "denied": "sk-bf-mcp-apps-denied",
    "codemode_errors": "sk-bf-codemode-errors",
}
BASELINE_IMAGE = "maximhq/bifrost@sha256:74826952fd6f1fc32f7bf8c364c41e27d996d0d03e4157ca8b55ddadaacb0668"
FIXTURE_RESOURCE_URI = "ui://mcp-apps-test-server/view.html"
BASELINE_SENTINELS = {
    "mcp-apps": (
        ("MCP Apps fixture direct initialize negotiates UI", "UI extension negotiated"),
        ("MCP Apps fixture direct tools list preserves App metadata", "both URI fields and input schema survive discovery"),
        ("MCP Apps fixture direct resources read returns HTML and CSP", "resource HTML MIME and CSP are relayed"),
        ("MCP Apps fixture Virtual MCP full discovery", "Virtual MCP preserves both UI metadata fields"),
        ("MCP Apps fixture aggregate discovery and namespaced UI URIs", "aggregate namespaces sources and marks callbacks app-only"),
        ("MCP Apps fixture aggregate reads second source resource", "second resource routes to its own source"),
        ("MCP Apps fixture aggregate namespaced callback routes second", "namespaced callback reaches correct instance"),
        ("MCP Apps fixture limited direct resource remains readable", "MCP Apps fixture limited direct resource remains readable"),
    ),
    "codemode-errors": (
        ("Code Mode failed script sets outer isError (codemode-errors)", "Starlark execution failure marks the outer result as an error"),
    ),
}
BASELINE_INVARIANTS = {
    "mcp-apps": (
        ("MCP Apps fixture direct initialize omits UI without negotiation", "UI extension omitted without negotiation"),
        ("MCP Apps fixture denied aggregate initialize", "denied aggregate initialize succeeds with empty capabilities"),
        ("MCP Apps fixture denied aggregate tools list is empty", "denied aggregate returns empty tools"),
        ("MCP Apps fixture Code Mode nested App call returns text only", "script returns nested text without structured App metadata"),
        ("MCP Apps fixture Code Mode nested tool error remains script value", "nested tool error is returned to script as a value per upstream behavior"),
    ),
    "codemode-errors": (
        ("Code Mode nested validation error remains a value (codemode-errors)", "executeToolCode succeeds when the nested tool returns an error value"),
    ),
}


def build_config():
    profile = json.loads((ROOT / "tests/integrations/python/config.json").read_text())
    clients = [entry for entry in profile["mcp"]["client_configs"] if entry["name"] in FIXTURE_NAMES]
    if {entry["name"] for entry in clients} != set(FIXTURE_NAMES):
        raise RuntimeError("shared profile is missing a fixture client after stack integration")
    for entry in clients:
        binary = "error-test-server" if entry["name"] == "errorTestServer" else "mcp-apps-test-server"
        args = [] if binary == "error-test-server" else entry["stdio_config"]["args"]
        entry["stdio_config"] = {"command": f"/repo/examples/mcps/{binary}/bin/{binary}", "args": args}
        entry["allow_by_default"] = False
    virtual_mcps = [entry for entry in profile["mcp"]["virtual_mcps"]
                    if entry["endpoint_slug"] in ("mcp-apps-full-virtual", "mcp-apps-limited-virtual")]
    keys = [entry for entry in profile["governance"]["virtual_keys"] if entry["value"] in KEYS.values()]
    if {entry["value"] for entry in keys} != set(KEYS.values()) or len(virtual_mcps) != 2:
        raise RuntimeError("shared profile is missing fixture grants after stack integration")
    return {
        "client": {"enable_logging": False, "enforce_governance_header": True,
                   "enforce_auth_on_inference": True},
        "providers": {},
        "mcp": {"client_configs": clients, "virtual_mcps": virtual_mcps,
                "tool_manager_config": {"code_mode_binding_level": "server"}},
        "governance": {"virtual_keys": keys},
    }


def requests_in(node):
    if isinstance(node, dict):
        if "request" in node and isinstance(node["request"], dict):
            yield node["request"]
        for value in node.values():
            yield from requests_in(value)
    elif isinstance(node, list):
        for value in node:
            yield from requests_in(value)


def items_in(node):
    if isinstance(node, dict):
        if isinstance(node.get("request"), dict):
            yield node
        else:
            for value in node.values():
                yield from items_in(value)
    elif isinstance(node, list):
        for value in node:
            yield from items_in(value)


def validate_collection(path, feature, expected):
    collection = json.loads(path.read_text())
    requests = list(requests_in(collection.get("item", [])))
    if len(requests) != expected:
        raise RuntimeError(f"{feature}: expected {expected} filtered requests, found {len(requests)}")
    for request in requests:
        url = request.get("url", "")
        raw = url.get("raw", "") if isinstance(url, dict) else url
        path_part = raw.split("{{baseUrl}}", 1)[-1]
        if not re.match(r"^/mcp(?:/|\?|$)", path_part):
            raise RuntimeError(f"{feature}: filtered request is outside MCP routes")
    return len(requests)


def executions_by_name(report, feature):
    executions = report.get("run", {}).get("executions", [])
    by_name = {}
    for execution in executions:
        name = execution.get("item", {}).get("name")
        status = (execution.get("response") or {}).get("code")
        if execution.get("skipped") or type(status) is not int or not 100 <= status <= 599:
            raise RuntimeError(f"{feature}: request {name!r} was skipped or produced no HTTP response")
        if not name or name in by_name:
            raise RuntimeError(f"{feature}: missing or duplicate Newman request name {name!r}")
        by_name[name] = execution
    return by_name


def validate_executions(report, feature, expected, expected_names):
    by_name = executions_by_name(report, feature)
    missing = sorted(set(expected_names) - by_name.keys())
    extra = sorted(by_name.keys() - set(expected_names))
    if len(by_name) != expected or missing or extra:
        raise RuntimeError(f"{feature}: execution mismatch (expected {expected}, got {len(by_name)}; missing={missing}; extra={extra})")


def failed_assertion(execution, assertion_name):
    matches = [item for item in execution.get("assertions", []) if item.get("assertion") == assertion_name]
    if len(matches) != 1:
        raise RuntimeError(f"expected exactly one assertion {assertion_name!r}, found {len(matches)}")
    return matches[0].get("error") is not None


def compare_reports(stock, candidate, feature):
    stock_items, candidate_items = executions_by_name(stock, feature + " stock"), executions_by_name(candidate, feature + " candidate")
    for case, assertion in BASELINE_SENTINELS.get(feature, ()):
        if case not in stock_items or case not in candidate_items or not failed_assertion(stock_items[case], assertion):
            raise RuntimeError(f"{feature}: stock did not demonstrate red sentinel {case!r} / {assertion!r}")
        if failed_assertion(candidate_items[case], assertion):
            raise RuntimeError(f"{feature}: candidate still fails sentinel {case!r} / {assertion!r}")
    for case, assertion in BASELINE_INVARIANTS.get(feature, ()):
        if case not in stock_items or case not in candidate_items:
            raise RuntimeError(f"{feature}: missing invariant execution {case!r}")
        if failed_assertion(stock_items[case], assertion) or failed_assertion(candidate_items[case], assertion):
            raise RuntimeError(f"{feature}: expected stock and candidate invariant to pass: {case!r}")
    return {"stock_red_sentinels": [case for case, _ in BASELINE_SENTINELS.get(feature, ())],
            "stock_green_invariants": [case for case, _ in BASELINE_INVARIANTS.get(feature, ())],
            "stock_failed_assertions": sum(1 for e in stock_items.values() for a in e.get("assertions", [])
                                            if a.get("error") is not None)}


def contract_uri(client_name):
    enc = lambda value: base64.urlsafe_b64encode(value.encode()).decode().rstrip("=")
    return f"ui://bifrost/{enc(client_name)}/{enc(FIXTURE_RESOURCE_URI)}"


def seed_stock_uris(collection_path):
    collection = json.loads(collection_path.read_text())
    seeds = {"mcpAppsDirectUri": contract_uri("mcp_apps_fixture"),
             "mcpAppsVirtualUri": contract_uri("mcp_apps_fixture"),
             "mcpAppsAggUri1": contract_uri("mcp_apps_fixture"),
             "mcpAppsAggUri2": contract_uri("mcp_apps_fixture_second")}
    variables = {entry.get("key"): entry for entry in collection.get("variable", [])}
    for name, value in seeds.items():
        if name in variables:
            variables[name]["value"] = value
            variables[name]["enabled"] = True
        else:
            collection.setdefault("variable", []).append({"key": name, "value": value, "type": "string"})
    collection_path.write_text(json.dumps(collection, indent=2) + "\n")
    return seeds


def redact(value):
    if isinstance(value, dict):
        return {key: "<redacted>" if key.lower() in ("authorization", "proxy-authorization")
                else redact(item) for key, item in value.items()}
    if isinstance(value, list):
        return [redact(item) for item in value]
    if isinstance(value, str):
        value = re.sub(r"(?i)(Bearer\s+)[A-Za-z0-9._~+/-]+=*", r"\1<redacted>", value)
        return re.sub(r"sk-bf-[A-Za-z0-9_-]+", "<redacted>", value)
    return value


def write_failure_report(report_dir, error):
    report_dir.mkdir(parents=True, exist_ok=True)
    details = {"error_type": type(error).__name__, "message": redact(str(error))}
    (report_dir / "fixture-error.json").write_text(json.dumps(details, indent=2) + "\n")


def self_check():
    config = build_config()
    assert config["providers"] == {}
    assert len(config["mcp"]["client_configs"]) == 3
    assert all(c["stdio_config"]["command"].startswith("/repo/") for c in config["mcp"]["client_configs"])
    assert all(not key["provider_configs"] for key in config["governance"]["virtual_keys"])
    second = next(c for c in config["mcp"]["client_configs"] if c["name"] == "mcp_apps_fixture_second")
    assert not second.get("is_code_mode_client", False)
    vmcps = {entry["endpoint_slug"]: entry for entry in config["mcp"]["virtual_mcps"]}
    full_vmcp = vmcps["mcp-apps-full-virtual"]
    limited_vmcp = vmcps["mcp-apps-limited-virtual"]
    assert full_vmcp["virtual_key_ids"] == ["vk-mcp-apps-full"]
    assert full_vmcp["tools"] == [{"mcp_client_name": name, "tool_names": ["*"]}
                                  for name in FIXTURE_NAMES[:2]]
    assert limited_vmcp["virtual_key_ids"] == ["vk-mcp-apps-limited"]
    assert limited_vmcp["tools"] == [{"mcp_client_name": "mcp_apps_fixture", "tool_names": ["open_view"]}]
    limited = next(key for key in config["governance"]["virtual_keys"] if key["value"] == KEYS["limited_no_app"])
    assert limited["mcp_configs"] == [{"mcp_client_name": "mcp_apps_fixture", "tools_to_execute": ["structured_result"]}]
    sample = {"item": [{"name": "ok", "request": {"url": "{{baseUrl}}/mcp/x"}}]}
    with tempfile.TemporaryDirectory() as temp:
        path = Path(temp) / "collection.json"
        path.write_text(json.dumps(sample))
        assert validate_collection(path, "self-check", 1) == 1
        sample["item"].append({"request": {"url": "{{baseUrl}}/v1/chat/completions"}})
        path.write_text(json.dumps(sample))
        try:
            validate_collection(path, "self-check", 2)
        except RuntimeError:
            pass
        else:
            raise AssertionError("inference route guard accepted a non-MCP request")
    sample_exec = {"item": {"name": "one"}, "response": {"code": 200}, "assertions": []}
    validate_executions({"run": {"executions": [sample_exec]}}, "self-check", 1, ["one"])
    try:
        validate_executions({"run": {"executions": []}}, "self-check", 1, ["one"])
    except RuntimeError:
        pass
    else:
        raise AssertionError("execution identity guard accepted a missing request")
    try:
        validate_executions({"run": {"executions": [sample_exec, sample_exec]}}, "self-check", 1, ["one"])
    except RuntimeError:
        pass
    else:
        raise AssertionError("execution identity guard accepted a duplicate request")
    for incomplete in ({**sample_exec, "skipped": True}, {**sample_exec, "response": None}):
        try:
            validate_executions({"run": {"executions": [incomplete]}}, "self-check", 1, ["one"])
        except RuntimeError:
            pass
        else:
            raise AssertionError("execution identity guard accepted an unexecuted request")
    assert contract_uri("mcp_apps_fixture") == "ui://bifrost/bWNwX2FwcHNfZml4dHVyZQ/dWk6Ly9tY3AtYXBwcy10ZXN0LXNlcnZlci92aWV3Lmh0bWw"
    assert contract_uri("mcp_apps_fixture_second") != contract_uri("mcp_apps_fixture")
    def execution(name, checks=(), failed=(), extra_failed=()):
        return {"item": {"name": name}, "response": {"code": 200}, "assertions": [
            *({"assertion": assertion, "error": {"name": "AssertionError"}} for assertion in failed),
            *({"assertion": assertion, "error": None} for assertion in checks if assertion not in failed),
            *({"assertion": assertion, "error": {"name": "AssertionError"}} for assertion in extra_failed)]}
    def report(failing_sentinels=(), fail_invariants=(), include_generic=False):
        executions = []
        for feature_name, checks in BASELINE_SENTINELS.items():
            for case, assertion in checks:
                failed = [assertion] if case in failing_sentinels else []
                generic = ["Response has content (text or tool_use)"] if include_generic and case == checks[0][0] else []
                executions.append(execution(case, checks=[assertion, *generic], failed=failed + generic))
        for feature_name, checks in BASELINE_INVARIANTS.items():
            for case, assertion in checks:
                failed = [assertion] if case in fail_invariants else []
                executions.append(execution(case, checks=[assertion], failed=failed))
        return {"run": {"executions": executions}}
    stock_report = report({case for checks in BASELINE_SENTINELS.values() for case, _ in checks}, include_generic=True)
    candidate_report = report()
    for feature in BASELINE_SENTINELS:
        subset = lambda source: {"run": {"executions": [e for e in source["run"]["executions"]
                                                          if any(e["item"]["name"] == case for case, _ in BASELINE_SENTINELS[feature] + BASELINE_INVARIANTS[feature])]}}
        expected = [case for case, _ in BASELINE_SENTINELS[feature] + BASELINE_INVARIANTS[feature]]
        validate_executions(subset(stock_report), feature, len(expected), expected)
        validate_executions(subset(candidate_report), feature, len(expected), expected)
        compare_reports(subset(stock_report), subset(candidate_report), feature)
        try:
            compare_reports(subset(stock_report), subset(stock_report), feature)
        except RuntimeError:
            pass
        else:
            raise AssertionError("baseline comparator accepted a failing candidate sentinel")
        broken_invariant = json.loads(json.dumps(subset(stock_report)))
        invariant_case, invariant_assertion = BASELINE_INVARIANTS[feature][0]
        item = executions_by_name(broken_invariant, feature)[invariant_case]
        next(a for a in item["assertions"] if a["assertion"] == invariant_assertion)["error"] = {"name": "AssertionError"}
        try:
            compare_reports(broken_invariant, subset(candidate_report), feature)
        except RuntimeError:
            pass
        else:
            raise AssertionError("baseline comparator accepted a failing stock invariant")
    broken_stock = report()
    try:
        compare_reports({"run": {"executions": [e for e in broken_stock["run"]["executions"]
                                                if e["item"]["name"] in {case for case, _ in BASELINE_SENTINELS["codemode-errors"] + BASELINE_INVARIANTS["codemode-errors"]}]}},
                        {"run": {"executions": [e for e in candidate_report["run"]["executions"]
                                                if e["item"]["name"] in {case for case, _ in BASELINE_SENTINELS["codemode-errors"] + BASELINE_INVARIANTS["codemode-errors"]}]}},
                        "codemode-errors")
    except RuntimeError:
        pass
    else:
        raise AssertionError("baseline comparator accepted a stock build with no red sentinel")
    candidate_names = {case for case, _ in BASELINE_SENTINELS["codemode-errors"] + BASELINE_INVARIANTS["codemode-errors"]}
    candidate_subset = {"run": {"executions": [e for e in candidate_report["run"]["executions"]
                                              if e["item"]["name"] in candidate_names]}}
    sentinel_case = BASELINE_SENTINELS["codemode-errors"][0][0]
    candidate_sentinel = next(e for e in candidate_subset["run"]["executions"] if e["item"]["name"] == sentinel_case)
    candidate_sentinel["assertions"][0]["error"] = {"name": "AssertionError"}
    try:
        compare_reports({"run": {"executions": [e for e in stock_report["run"]["executions"]
                                                if e["item"]["name"] in candidate_names]}},
                        candidate_subset, "codemode-errors")
    except RuntimeError:
        pass
    else:
        raise AssertionError("baseline comparator accepted a failing candidate sentinel")
    with tempfile.TemporaryDirectory() as temp:
        collection_path = Path(temp) / "collection.json"
        collection_path.write_text(json.dumps({"item": [], "variable": []}))
        seeds = seed_stock_uris(collection_path)
        written = json.loads(collection_path.read_text())
        assert {entry["key"]: entry["value"] for entry in written["variable"]} == seeds
    safe = redact({"Authorization": "Bearer sk-bf-example", "body": "token sk-bf-example"})
    assert safe == {"Authorization": "<redacted>", "body": "token <redacted>"}
    with tempfile.TemporaryDirectory() as temp:
        write_failure_report(Path(temp), RuntimeError("Authorization: Bearer sk-bf-example"))
        failure = json.loads((Path(temp) / "fixture-error.json").read_text())
        assert failure["error_type"] == "RuntimeError"
        assert "sk-bf-example" not in failure["message"] and "<redacted>" in failure["message"]
    print("PASS fixture CI self-check (config scope, route/name guards, baseline red/green comparator, URI seeds, report redaction)")


def rpc(url, path, method, params, token):
    request = urllib.request.Request(
        url + path,
        data=json.dumps({"jsonrpc": "2.0", "id": 1, "method": method, "params": params}).encode(),
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json",
                 "Accept": "application/json, text/event-stream"},
    )
    try:
        response = urllib.request.urlopen(request, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        if response.headers.get_content_type() == "text/event-stream":
            raw = next(line[6:] for line in raw.splitlines() if line.startswith("data: "))
        return response.status, json.loads(raw)


def wait_health(url):
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url + "/health", timeout=2) as response:
                if response.status == 200:
                    break
        except (OSError, urllib.error.URLError):
            time.sleep(1)
    else:
        raise RuntimeError("gateway /health did not become ready within 180 seconds")


def ready(url):
    wait_health(url)

    routes = (("/mcp/mcp-apps-fixture", "full", False),
              ("/mcp/mcp-apps-full-virtual", "full", False),
              ("/mcp/mcp-apps-limited-virtual", "limited", False),
              ("/mcp", "full", False),
              ("/mcp/error-test-server", "codemode_errors", True))
    init = {"protocolVersion": "2025-06-18", "clientInfo": {"name": "fixture-ci-readiness", "version": "1"},
            "capabilities": {}}
    for path, key, needs_code in routes:
        status, body = rpc(url, path, "initialize", init, KEYS[key])
        if status != 200 or "result" not in body:
            raise RuntimeError(f"MCP initialize readiness failed on {path} (HTTP {status})")
        status, body = rpc(url, path, "tools/list", {}, KEYS[key])
        tools = body.get("result", {}).get("tools", [])
        names = {tool.get("name", "") for tool in tools}
        if status != 200 or not tools or (needs_code and "executeToolCode" not in names):
            raise RuntimeError(f"MCP tools/list readiness failed on {path} (HTTP {status}, tools={len(tools)})")
        if path == "/mcp" and "mcp_apps_fixture_second-open_view" not in names:
            raise RuntimeError("aggregate readiness did not expose the second fixture App")
        if key == "limited" and (not any(name.endswith("open_view") for name in names)
                                  or any(name.endswith("fail_tool") for name in names)):
            raise RuntimeError("limited Virtual MCP readiness did not enforce its tool grant")
    print("PASS candidate health and MCP readiness on five fixture routes", flush=True)


def run(command, cwd, env, log):
    with log.open("w") as output:
        result = subprocess.run(command, cwd=cwd, env=env, stdout=output, stderr=subprocess.STDOUT)
    return result.returncode


def archive_report(source, dest):
    if not source.is_file():
        return False
    raw = source.read_text(errors="replace")
    if source.suffix == ".json":
        data = redact(json.loads(raw))
        dest.write_text(json.dumps(data, indent=2) + "\n")
    else:
        dest.write_text(redact(raw))
    return True


def main(args):
    if args == ["--self-check"]:
        self_check()
        return 0
    baseline = args[0] == "--baseline" if args else False
    if len(args) != 2:
        print("usage: test-mcp-apps-fixture.sh IMAGE REPORT_DIR | --baseline REPORT_DIR | --self-check", file=sys.stderr)
        return 2
    image, report_dir_arg = (BASELINE_IMAGE if baseline else args[0]), args[1]
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise RuntimeError("Docker and harness execution is restricted to GitHub Actions; use --self-check locally")
    report_dir = Path(report_dir_arg).resolve()
    report_dir.mkdir(parents=True, exist_ok=True)
    if (ROOT / ".env").exists():
        raise RuntimeError("refusing to run: repository .env exists; fixture CI must not source credentials")
    for tool in ("docker", "go", "node", "make", "python3"):
        if not shutil.which(tool):
            raise RuntimeError(f"required CI tool missing: {tool}")

    source = ROOT / "tests/e2e/api/collections/provider-harness.json"
    augment = ROOT / "tests/e2e/api/runners/augment-provider-harness.mjs"
    filter_script = ROOT / "tests/e2e/api/runners/filter-collection.mjs"
    modules = ((ROOT / "examples/mcps/mcp-apps-test-server", "mcp-apps-test-server"),
               (ROOT / "examples/mcps/error-test-server", "error-test-server"))
    for module, _ in modules:
        if not (module / "go.mod").is_file():
            raise RuntimeError(f"fixture source missing after merge: {module.relative_to(ROOT)}")
    if not source.is_file() or not augment.is_file() or not filter_script.is_file():
        raise RuntimeError("MCP Apps harness cases or filter scripts are missing")

    with tempfile.TemporaryDirectory(prefix="bifrost-mcp-fixture-") as tmp:
        temp = Path(tmp)
        app_dir = temp / "app"
        app_dir.mkdir(mode=0o700)
        config_path = app_dir / "config.json"
        config_path.write_text(json.dumps(build_config(), indent=2) + "\n")
        config_path.chmod(0o600)

        env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"),
               "HOME": os.environ.get("HOME", str(temp)), "TMPDIR": tmp,
               "USE_INFISICAL": "0", "CI": "1"}
        harness_source = source
        make_source_args = []
        uri_seeds = {}
        if baseline:
            harness_source = temp / "stock-provider-harness.json"
            shutil.copy2(source, harness_source)
            uri_seeds = seed_stock_uris(harness_source)
            make_source_args = [f"HARNESS_SOURCE_COLLECTION={harness_source}"]
        augmented = temp / "augmented.json"
        subprocess.run(["node", str(augment), "--source", str(harness_source), "--out", str(augmented)],
                       cwd=ROOT, env=env, check=True)
        counts = {}
        expected_names = {}
        for feature, expected in (("mcp-apps", 37), ("codemode-errors", 2)):
            filtered = temp / f"filtered-{feature}.json"
            subprocess.run(["node", str(filter_script), "--source", str(augmented), "--out", str(filtered),
                            "--feature", feature], cwd=ROOT, env=env, check=True)
            filtered_data = json.loads(filtered.read_text())
            filtered_items = list(items_in(filtered_data.get("item", [])))
            names = [item.get("name") for item in filtered_items]
            if not names or any(not name for name in names) or len(names) != len(set(names)):
                raise RuntimeError(f"{feature}: filtered request names are missing or duplicated")
            expected_names[feature] = names
            count = len(list(requests_in(filtered_data.get("item", []))))
            counts[feature] = validate_collection(filtered, feature, expected)
        if "mcp-apps" not in counts:
            raise RuntimeError("mcp-apps filter produced no MCP requests")
        print("PASS filtered harness scopes: " + ", ".join(f"{k}={v}" for k, v in counts.items()), flush=True)

        outputs = []
        created_bins = []
        container = "bifrost-mcp-apps-fixture-" + temp.name.rsplit("-", 1)[-1]
        container_started = False
        try:
            for module, name in modules:
                output = module / "bin" / name
                output.parent.mkdir(parents=True, exist_ok=True)
                if output.exists():
                    raise RuntimeError(f"refusing to overwrite existing fixture binary: {output.relative_to(ROOT)}")
                env_go = {**env, "GOWORK": "off", "CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"}
                subprocess.run(["go", "build", "-trimpath", "-o", str(output), "."], cwd=module,
                               env=env_go, check=True)
                output.chmod(0o755)
                created_bins.append(output)

            base_config = json.loads(config_path.read_text())
            # The three stdio children must exist in the read-only repo mount at these exact paths.
            assert len(base_config["mcp"]["client_configs"]) == 3
            if baseline:
                subprocess.run(["docker", "pull", "--platform", "linux/amd64", image],
                               cwd=ROOT, env=env, check=True)
            elif subprocess.run(["docker", "image", "inspect", image], stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL).returncode:
                raise RuntimeError(f"candidate image not present in Docker: {image}")
            subprocess.run(["docker", "run", "-d", "--platform", "linux/amd64", "--user", f"{os.getuid()}:{os.getgid()}", "--name", container, "-p", "127.0.0.1::8080",
                            "-e", "APP_DIR=/fixture-data", "-e", "LOG_LEVEL=warn",
                            "-v", f"{ROOT}:/repo:ro", "-v", f"{app_dir}:/fixture-data",
                            "-w", "/repo/transports/bifrost-http", image],
                           cwd=ROOT, env=env, check=True, stdout=subprocess.DEVNULL)
            container_started = True
            port_line = subprocess.check_output(["docker", "port", container, "8080/tcp"], text=True).splitlines()[0]
            port = int(port_line.rsplit(":", 1)[1])
            base_url = f"http://127.0.0.1:{port}"
            ready(base_url) if not baseline else wait_health(base_url)

            pass_fail = 0
            runs = [("mcp-apps", 60)]
            if "codemode-errors" in counts:
                runs.append(("codemode-errors", 4))
            for index, (feature, cap) in enumerate(runs, start=1):
                command = ["make", "run-provider-harness-test", "CI=1", "PARALLEL=0",
                           f"FEATURE={feature}", f"BASE_URL={base_url}", f"APP_DIR={app_dir}",
                           f"HARNESS_MAX_REQUESTS={cap}", *make_source_args]
                log = temp / f"{feature}.log"
                (ROOT / "tmp/newman-report.json").unlink(missing_ok=True)
                (ROOT / "tmp/harness-failures.md").unlink(missing_ok=True)
                rc = run(command, ROOT, env, log)
                raw_report = temp / f"raw-{feature}-newman-report.json"
                raw_failures = temp / f"raw-{feature}-harness-failures.md"
                if (ROOT / "tmp/newman-report.json").is_file():
                    shutil.copy2(ROOT / "tmp/newman-report.json", raw_report)
                if (ROOT / "tmp/harness-failures.md").is_file():
                    shutil.copy2(ROOT / "tmp/harness-failures.md", raw_failures)
                prefix = "stock-" if baseline else ""
                archive_report(log, report_dir / f"{prefix}{feature}-runner.log")
                archive_report(ROOT / "tmp/harness-quiet.log", report_dir / f"{prefix}{feature}-harness.log")
                report_ok = archive_report(raw_report, report_dir / f"{prefix}{feature}-newman-report.json")
                failures_ok = archive_report(raw_failures, report_dir / f"{prefix}{feature}-harness-failures.md")
                (report_dir / f"{prefix}{feature}-exit-code.txt").write_text(str(rc) + "\n")
                if not report_ok:
                    raise RuntimeError(f"{feature}: Make target produced no Newman JSON report")
                report_data = json.loads(raw_report.read_text())
                validate_executions(report_data, feature, counts[feature], expected_names[feature])
                if baseline:
                    if rc == 0:
                        raise RuntimeError(f"{feature}: stock Newman run passed despite the expected red sentinels")
                    candidate_path = report_dir / f"{feature}-newman-report.json"
                    candidate_rc_path = report_dir / f"{feature}-exit-code.txt"
                    if not candidate_path.is_file() or not candidate_rc_path.is_file() or candidate_rc_path.read_text().strip() != "0":
                        raise RuntimeError("baseline comparison requires a complete passing candidate run first")
                    candidate_data = json.loads(candidate_path.read_text())
                    validate_executions(candidate_data, feature, counts[feature], expected_names[feature])
                    comparison = compare_reports(report_data, candidate_data, feature)
                    comparison.update({"stock_image": image, "stock_release_tag": "v2.2.4-amd64",
                                       "seeded_contract_uris": uri_seeds})
                    (report_dir / f"{feature}-comparison.json").write_text(json.dumps(comparison, indent=2) + "\n")
                    print(f"PASS {feature} stock comparison (expected red sentinels, green invariants)", flush=True)
                elif rc:
                    print(f"FAIL {feature} harness (exit {rc}); redacted reports: {report_dir}", flush=True)
                    pass_fail = rc
                else:
                    print(f"PASS {feature} harness; redacted reports: {report_dir}", flush=True)
                if failures_ok:
                    print(f"Saved redacted {feature} failure report", flush=True)
            return pass_fail
        finally:
            if container_started:
                gateway_log = temp / "gateway.log"
                run(["docker", "logs", container], ROOT, env, gateway_log)
                archive_report(gateway_log, report_dir / ("stock-fixture-gateway.log" if baseline else "fixture-gateway.log"))
                subprocess.run(["docker", "rm", "-f", container], cwd=ROOT, env=env, check=True,
                               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            for path in created_bins:
                path.unlink(missing_ok=True)


if __name__ == "__main__":
    try:
        raise SystemExit(main(sys.argv[1:]))
    except (OSError, subprocess.CalledProcessError, RuntimeError, ValueError) as error:
        if len(sys.argv) == 3:
            try:
                write_failure_report(Path(sys.argv[2]).resolve(), error)
            except OSError:
                print("fixture CI could not write the redacted failure report", file=sys.stderr)
        print(f"fixture CI failed: {error}", file=sys.stderr)
        raise SystemExit(1)
PY
