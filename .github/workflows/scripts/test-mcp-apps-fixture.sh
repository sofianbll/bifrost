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
FIXTURE_NAMES = ("mcp_apps_fixture", "mcp_apps_fixture_second", "mcp_apps_fixture_modern", "errorTestServer")
KEYS = {
    "full": "sk-bf-mcp-apps-full",
    "limited": "sk-bf-mcp-apps-limited",
    "limited_no_app": "sk-bf-mcp-apps-limited-no-app",
    "denied": "sk-bf-mcp-apps-denied",
    "codemode_errors": "sk-bf-codemode-errors",
}
SETUP_TOKEN = "bifrost-e2e-setup-token"
FIXTURE_RESOURCE_URI = "ui://mcp-apps-test-server/view.html"
BASELINE_SENTINELS = {
    "mcp-apps": (
        ("MCP Apps fixture direct initialize negotiates UI", "UI extension negotiated"),
        ("MCP Apps fixture direct tools list preserves App metadata", "both URI fields and input schema survive discovery"),
        ("MCP Apps fixture direct resources read returns HTML and CSP", "resource HTML MIME and CSP are relayed"),
        ("MCP Apps fixture Virtual MCP full discovery", "Virtual MCP preserves both UI metadata fields"),
        ("MCP Apps fixture aggregate discovery and namespaced UI URIs", "aggregate namespaces sources and marks callbacks app-only"),
        ("MCP Apps fixture aggregate reads second source resource", "second resource routes to its own source"),
        ("MCP Apps fixture aggregate namespaced callback routes first", "namespaced callback reaches correct instance"),
        ("MCP Apps fixture direct legacy-only metadata remains routable", "legacy-only URI is rewritten and modern field stays absent"),
        ("MCP Apps fixture direct modern-only metadata remains routable", "modern-only URI is rewritten and legacy field stays absent"),
        ("MCP Apps fixture limited direct resource remains readable", "MCP Apps fixture limited direct resource remains readable"),
    ),
    "codemode-errors": (
        ("Code Mode failed script sets outer isError (codemode-errors)", "Starlark execution failure marks the outer result as an error"),
        ("Code Mode nested Error prefix remains a value (codemode-errors)", "Error: prefix payload stays a successful script value"),
    ),
}
BASELINE_INVARIANTS = {
    "mcp-apps": (
        ("MCP Apps fixture direct initialize omits UI without negotiation", "UI extension omitted without negotiation"),
        ("MCP Apps fixture denied aggregate initialize", "denied aggregate initialize succeeds with empty capabilities"),
        ("MCP Apps fixture denied aggregate tools list is empty", "denied aggregate returns empty tools"),
        ("MCP Apps fixture Code Mode nested App call returns text only", "script returns nested text without structured App metadata"),
        ("MCP Apps fixture Code Mode nested tool error remains script value", "nested tool error is returned to script as a value per upstream behavior"),
        ("MCP Apps fixture aggregate namespaced callback routes second", "namespaced callback reaches correct instance"),
    ),
    "codemode-errors": (
        ("Code Mode nested validation error remains a value (codemode-errors)", "executeToolCode succeeds when the nested tool returns an error value"),
    ),
}

# Exact stock-83258 failure pairs for observed behavior and the known unsupported
# resource paths that will execute after URI seeding is fixed. Each entry records
# why that pair may be red in characterization; unknown failures remain out of
# scope, including a second failure on a listed request.
BASELINE_ALLOWED_STOCK_FAILURES = {
    "mcp-apps": {
        ("MCP Apps fixture direct initialize negotiates UI", "UI extension negotiated"):
            "stock initialize does not negotiate the MCP Apps UI extension",
        ("MCP Apps fixture direct tools list preserves App metadata", "both URI fields and input schema survive discovery"):
            "Code Mode exposes meta-tools instead of that source's native App tool",
        ("MCP Apps fixture direct tools list preserves App metadata", "App-only callback is marked for host visibility"):
            "Code Mode hides the source callback from the direct native catalog",
        ("MCP Apps fixture direct resources list returns array", "Response has content (text or tool_use)"):
            "stock returns JSON-RPC resources-not-supported for resources/list",
        ("MCP Apps fixture direct resources list returns array", "resources/list returns array"):
            "stock returns JSON-RPC resources-not-supported for resources/list",
        ("MCP Apps fixture direct resources read returns HTML and CSP", "resource HTML MIME and CSP are relayed"):
            "stock direct MCP resources/read is unsupported",
        ("MCP Apps fixture direct resources read returns HTML and CSP", "Response has content (text or tool_use)"):
            "stock direct MCP resources/read is unsupported",
        ("MCP Apps fixture Virtual MCP full resource read", "Virtual MCP resource is readable with MIME and CSP"):
            "stock Virtual MCP resources/read cannot relay this App resource",
        ("MCP Apps fixture Virtual MCP full resource read", "Response has content (text or tool_use)"):
            "stock Virtual MCP resources/read cannot relay this App resource",
        ("MCP Apps fixture aggregate reads first source resource", "first resource routes to its own source"):
            "stock aggregate resources/read cannot relay the first App resource",
        ("MCP Apps fixture aggregate reads first source resource", "Response has content (text or tool_use)"):
            "stock aggregate resources/read cannot relay the first App resource",
        ("MCP Apps fixture aggregate reads second source resource", "second resource routes to its own source"):
            "stock aggregate resources/read cannot relay the second App resource",
        ("MCP Apps fixture aggregate reads second source resource", "Response has content (text or tool_use)"):
            "stock aggregate resources/read cannot relay the second App resource",
        ("MCP Apps fixture limited direct resource remains readable", "MCP Apps fixture limited direct resource remains readable"):
            "stock direct resources/read is unsupported for the limited key too",
        ("MCP Apps fixture limited direct resource remains readable", "Response has content (text or tool_use)"):
            "stock direct resources/read is unsupported for the limited key too",
        ("MCP Apps fixture limited key cannot read ungranted App resource", "resource linked to a disallowed App tool is refused"):
            "stock resources/read unsupported error masks the grant-specific refusal",
        ("MCP Apps fixture limited key cannot read ungranted App resource", "Response has content (text or tool_use)"):
            "stock resources/read unsupported error masks the grant-specific refusal",
        ("MCP Apps fixture direct structured tool result", "Response has content (text or tool_use)"):
            "Code Mode hides the direct structured-result source tool",
        ("MCP Apps fixture direct structured tool result", "content structuredContent metadata and success marker survive"):
            "Code Mode hides the direct structured-result source tool",
        ("MCP Apps fixture direct App callback remains callable", "Response has content (text or tool_use)"):
            "Code Mode hides the direct App callback source tool",
        ("MCP Apps fixture direct App callback remains callable", "App-only callback resolves on direct route"):
            "Code Mode hides the direct App callback source tool",
        ("MCP Apps fixture direct tool error marker", "Response has content (text or tool_use)"):
            "Code Mode hides the direct tool-error source tool",
        ("MCP Apps fixture direct tool error marker", "fixture tool error remains isError"):
            "Code Mode hides the direct tool-error source tool",
        ("MCP Apps fixture Virtual MCP full discovery", "Virtual MCP preserves both UI metadata fields"):
            "stock Virtual MCP discovery omits the Code Mode source App tool",
        ("MCP Apps fixture Virtual MCP limited discovery", "limited key sees only its allowed App tool"):
            "stock limited catalog omits the direct App tool from its Code Mode source",
        ("MCP Apps fixture aggregate discovery and namespaced UI URIs", "aggregate namespaces sources and marks callbacks app-only"):
            "stock aggregate catalog omits the Code Mode source App tool and callback metadata",
        ("MCP Apps fixture aggregate namespaced callback routes first", "Response has content (text or tool_use)"):
            "stock aggregate catalog omits the Code Mode source callback",
        ("MCP Apps fixture aggregate namespaced callback routes first", "namespaced callback reaches correct instance"):
            "stock aggregate catalog omits the Code Mode source callback",
        ("MCP Apps fixture aggregate structured result", "aggregate tool result keeps source identity and metadata"):
            "stock aggregate catalog omits the Code Mode source structured-result tool",
        ("MCP Apps fixture Code Mode client exposes native App metadata", "Code Mode preserves native App metadata and has no UI metadata itself"):
            "stock Code Mode client filters out its source's native App tool",
        ("MCP Apps fixture Code Mode direct App call preserves full result", "Response has content (text or tool_use)"):
            "stock Code Mode client filters out its source's native App tool",
        ("MCP Apps fixture Code Mode direct App call preserves full result", "native direct call retains App result contract"):
            "stock Code Mode client filters out its source's native App tool",
        ("MCP Apps fixture Code Mode script failure sets isError", "only sandbox script failure marks outer result isError"):
            "stock returns a successful outer result for this Starlark parse failure",
        ("MCP Apps fixture limited direct discovery", "MCP Apps fixture limited direct discovery"):
            "stock limited direct catalog omits the permitted App tool",
        ("MCP Apps fixture limited aggregate discovery", "MCP Apps fixture limited aggregate discovery"):
            "stock limited aggregate catalog omits the permitted App tool",
        ("MCP Apps fixture direct legacy-only metadata remains routable", "legacy-only URI is rewritten and modern field stays absent"):
            "stock discovery drops the legacy-only UI metadata field",
        ("MCP Apps fixture direct modern-only metadata remains routable", "modern-only URI is rewritten and legacy field stays absent"):
            "stock discovery drops the modern-only UI metadata field",
    },
    "codemode-errors": {
        ("Code Mode failed script sets outer isError (codemode-errors)", "Starlark execution failure marks the outer result as an error"):
            "stock outer handler does not mark this script failure as isError",
        ("Code Mode nested Error prefix remains a value (codemode-errors)", "Error: prefix payload stays a successful script value"):
            "stock nested tool result with Error: prefix is interpreted as an error",
    },
}


DEFAULT_SCOPES = ("mcp-apps", "codemode-errors")
VALID_APP_SCOPES = {"mcp-apps", "codemode-errors"}
FIRSTBOOT_SCOPE = "firstboot-grants"
FIRSTBOOT_STANDALONE_SCOPE = "firstboot-grants-standalone"
FIRSTBOOT_CONTRACTS = {
    FIRSTBOOT_SCOPE: {
        "scope": FIRSTBOOT_SCOPE,
        "request": "MCP Apps fixture firstboot grants invokes second callback (firstboot-grants)",
        "folder": "146. First-boot Virtual MCP grants (firstboot-grants)",
        "client": "mcp_apps_fixture",
        "clients": FIXTURE_NAMES[:3],
        "virtual_mcp": "mcp-apps-full-virtual",
        "virtual_key_id": "vk-mcp-apps-full",
        "virtual_key_value": KEYS["full"],
        "tool": "save_state",
        "discovery_clients": ("mcp_apps_fixture_second",),
        "custom_assertion": "file-declared grant invokes the second fixture source",
    },
    FIRSTBOOT_STANDALONE_SCOPE: {
        "scope": FIRSTBOOT_STANDALONE_SCOPE,
        "request": "stdio fixture firstboot grant invokes string_transform (firstboot-grants-standalone)",
        "folder": "251. First-boot Virtual MCP grants (firstboot-grants-standalone)",
        "client": "firstboot_grants_fixture",
        "clients": ("firstboot_grants_fixture",),
        "virtual_mcp": "firstboot-grants-fixture",
        "virtual_key_id": "vk-firstboot-grants-fixture",
        "virtual_key_value": "sk-bf-firstboot-grants-fixture",
        "tool": "string_transform",
        "request_tool": "firstboot_grants_fixture-string_transform",
        "discovery_clients": ("firstboot_grants_fixture",),
        "custom_assertion": "file-declared grant invokes the stdio fixture tool",
    },
}
FIRSTBOOT_STATUS_ASSERTION = "Status code is 200"
FIRSTBOOT_HTTP_ASSERTION = "HTTP status is exact"
FIRSTBOOT_CONTENT_ASSERTION = "Response has content (text or tool_use)"


def firstboot_assertions(contract, status):
    expected = {FIRSTBOOT_STATUS_ASSERTION, FIRSTBOOT_HTTP_ASSERTION, contract["custom_assertion"]}
    if status == 200:
        expected.add(FIRSTBOOT_CONTENT_ASSERTION)
    elif status != 403:
        raise RuntimeError(f"{contract['scope']}: unexpected HTTP status for assertion inventory: {status}")
    return expected


def firstboot_failure_assertions(contract):
    return {FIRSTBOOT_STATUS_ASSERTION, FIRSTBOOT_HTTP_ASSERTION, contract["custom_assertion"]}


_SHARED_FIRSTBOOT = FIRSTBOOT_CONTRACTS[FIRSTBOOT_SCOPE]
FIRSTBOOT_REQUEST = _SHARED_FIRSTBOOT["request"]
FIRSTBOOT_FOLDER = _SHARED_FIRSTBOOT["folder"]
FIRSTBOOT_VIRTUAL_MCP = _SHARED_FIRSTBOOT["virtual_mcp"]
FIRSTBOOT_VIRTUAL_KEY = _SHARED_FIRSTBOOT["virtual_key_id"]
FIRSTBOOT_CALLBACK_ASSERTION = _SHARED_FIRSTBOOT["custom_assertion"]
FIRSTBOOT_ASSERTIONS = {
    "firstboot": firstboot_assertions(_SHARED_FIRSTBOOT, 403),
    "firstboot-green": firstboot_assertions(_SHARED_FIRSTBOOT, 200),
    "restart": firstboot_assertions(_SHARED_FIRSTBOOT, 200),
}
FIRSTBOOT_EXPECTED_FAILURES = firstboot_failure_assertions(_SHARED_FIRSTBOOT)


def parse_scopes(value=None):
    raw = os.environ.get("BIFROST_FIXTURE_SCOPES") if value is None else value
    if raw is None:
        return DEFAULT_SCOPES
    if not raw or raw.strip() != raw:
        raise RuntimeError("BIFROST_FIXTURE_SCOPES must be a non-empty comma-separated scope list")
    parts = raw.split(",")
    if any(not part for part in parts) or len(parts) != len(set(parts)):
        raise RuntimeError("BIFROST_FIXTURE_SCOPES contains an empty or duplicate scope")
    firstboot_scopes = set(parts) & set(FIRSTBOOT_CONTRACTS)
    if firstboot_scopes:
        if len(parts) != 1:
            raise RuntimeError("firstboot-grants scopes must run exclusively")
        return (parts[0],)
    unknown = sorted(set(parts) - VALID_APP_SCOPES)
    if unknown:
        raise RuntimeError("unknown BIFROST_FIXTURE_SCOPES: " + ", ".join(unknown))
    return tuple(scope for scope in DEFAULT_SCOPES if scope in parts)


def feature_selections(scopes):
    if len(scopes) == 1 and scopes[0] in FIRSTBOOT_CONTRACTS:
        contract = FIRSTBOOT_CONTRACTS[scopes[0]]
        return ((scopes[0], 1, ["--feature", scopes[0], "--folder", contract["folder"]]),)
    selected = []
    for feature, count in (("mcp-apps", 39), ("codemode-errors", 3)):
        if feature not in scopes:
            continue
        selectors = ["--feature", feature]
        if feature == "mcp-apps":
            selectors.extend(("--exclude-feature-any", FIRSTBOOT_SCOPE))
        selected.append((feature, count, selectors))
    return tuple(selected)


def setup_policy(scopes):
    if len(scopes) == 1 and scopes[0] in FIRSTBOOT_CONTRACTS:
        return "file-declared-read-only"
    if "mcp-apps" in scopes:
        return "postseed-app-fixture-vmcps"
    return "none"


def expected_fixture_client_count(scopes):
    if len(scopes) == 1 and scopes[0] in FIRSTBOOT_CONTRACTS:
        return len(FIRSTBOOT_CONTRACTS[scopes[0]]["clients"])
    if "mcp-apps" in scopes:
        return 3 + int("codemode-errors" in scopes)
    return 1


def setup_virtual_key_attachments(url, scopes):
    if setup_policy(scopes) != "postseed-app-fixture-vmcps":
        return []
    return seed_virtual_key_attachments(url)


def is_firstboot_scope(scopes):
    return len(scopes) == 1 and scopes[0] in FIRSTBOOT_CONTRACTS


def firstboot_contract(scopes):
    if not is_firstboot_scope(scopes):
        raise RuntimeError(f"invalid first-boot fixture scope: {scopes}")
    return FIRSTBOOT_CONTRACTS[scopes[0]]


def require_same_container(before, after):
    if not before or before != after:
        raise RuntimeError("firstboot-grants restart changed the disposable container identity")
    return True


def firstboot_comparison_classification(phase, candidate_gate):
    if phase == "firstboot":
        if candidate_gate == "candidate-green":
            return "fixed"
        if candidate_gate == "expected-stock-red":
            return "unresolved"
        raise RuntimeError(f"firstboot phase has an invalid candidate gate: {candidate_gate}")
    if phase == "restart":
        if candidate_gate in ("candidate-green", "restart-green-control"):
            return "green-control"
        raise RuntimeError(f"restart phase has an invalid candidate gate: {candidate_gate}")
    raise RuntimeError(f"unknown firstboot-grants phase: {phase}")


def reference_host_status_filename(baseline):
    return "stock-reference-host-status.json" if baseline else "reference-host-status.json"


def run_reference_host(report_dir, base_url, source_identities, baseline, scopes):
    host_directory = os.environ.get("BIFROST_REFERENCE_HOST_DIRECTORY", "").strip()
    status_path = report_dir / reference_host_status_filename(baseline)
    if baseline:
        if host_directory:
            raise RuntimeError("pinned reference-host qualification is candidate-only")
        status_path.write_text(json.dumps({"status": "skipped-stock",
                                           "reason": "reference-host qualification runs only against the candidate"}, indent=2) + "\n")
        return
    if not host_directory:
        status_path.write_text(json.dumps({"status": "not-requested",
                                           "reason": "matrix row did not opt into pinned reference-host qualification"}, indent=2) + "\n")
        return
    if "mcp-apps" not in scopes or is_firstboot_scope(scopes):
        raise RuntimeError("pinned reference-host qualification requires a non-firstboot MCP Apps scope")
    host_path = Path(host_directory).resolve()
    manifest_path = host_path / ".bifrost-reference-host.json"
    renderer = ROOT / "tests/mcpapps/render-reference-host.mjs"
    if not manifest_path.is_file() or not renderer.is_file():
        raise RuntimeError("pinned reference-host bootstrap or renderer is missing")
    manifest = json.loads(manifest_path.read_text())
    if manifest.get("commit") != "82221c0c8ce7661efa6771c9d461511b1650495f" or manifest.get("version") != "2.0.3":
        raise RuntimeError("pinned reference-host source identity differs from the approved host")
    shutil.copy2(manifest_path, report_dir / "reference-host-manifest.json")
    host_env_base = {
        "PATH": os.environ.get("PATH", "/usr/bin:/bin"),
        "HOME": os.environ.get("HOME", str(report_dir)),
        "MCP_APP_HOST_DIRECTORY": str(host_path),
        "BIFROST_FIXTURE_SERVER_URL": f"{base_url}/mcp/mcp-apps-full-virtual",
        "BIFROST_FIXTURE_BEARER": KEYS["full"],
        "BIFROST_FIXTURE_EXPECTED_INSTANCE": "first",
        "BIFROST_FIXTURE_CLIENT_NAME": "mcp_apps_fixture",
        "BIFROST_CANDIDATE_SOURCE": source_identities["candidate"],
    }
    if os.environ.get("PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH"):
        host_env_base["PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH"] = os.environ["PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH"]
    outcomes = []
    for name, replay in (("normal", "false"), ("controlled-replay", "true")):
        host_env = {**host_env_base, "BIFROST_FIXTURE_PARTIAL_REPLAY": replay}
        run_result = subprocess.run(["node", str(renderer)], cwd=ROOT, env=host_env,
                                    capture_output=True, text=True)
        stdout = run_result.stdout.replace(KEYS["full"], "[REDACTED]")
        stderr = run_result.stderr.replace(KEYS["full"], "[REDACTED]")
        stem = f"reference-host-{name}"
        (report_dir / f"{stem}-exit-code.txt").write_text(f"{run_result.returncode}\n")
        (report_dir / f"{stem}-runner.log").write_text(stdout + stderr)
        if run_result.returncode:
            failure = {"status": "failed", "mode": name, "exit_code": run_result.returncode,
                       "log": f"{stem}-runner.log", "candidate_source": source_identities["candidate"],
                       "host_commit": manifest["commit"], "app_bundle_sha256": manifest["appBundle"]["sha256"]}
            (report_dir / f"{stem}.json").write_text(json.dumps(failure, indent=2) + "\n")
            status_path.write_text(json.dumps({"status": "failed", "failed_mode": name}, indent=2) + "\n")
            raise RuntimeError(f"pinned reference-host {name} qualification failed; see {stem}-runner.log")
        try:
            evidence = json.loads(stdout)
        except json.JSONDecodeError as error:
            raise RuntimeError(f"pinned reference-host {name} emitted invalid JSON evidence") from error
        if (evidence.get("hostCommit") != manifest["commit"] or
                evidence.get("candidateSource") != source_identities["candidate"] or
                evidence.get("expectedInstance") != "first" or
                evidence.get("fixtureClientName") != "mcp_apps_fixture" or
                evidence.get("checks", {}).get("controlledPartialInputDelivered") != (replay == "true")):
            raise RuntimeError(f"pinned reference-host {name} evidence does not match the requested source and mode")
        (report_dir / f"reference-host-{name}.json").write_text(json.dumps(evidence, indent=2) + "\n")
        outcomes.append({"mode": name, "passed": True, "host_commit": manifest["commit"],
                         "app_bundle_sha256": manifest["appBundle"]["sha256"]})
    status_path.write_text(json.dumps({"status": "passed", "candidate_source": source_identities["candidate"],
                                       "host_commit": manifest["commit"], "runs": outcomes}, indent=2) + "\n")


def build_config(scopes=None):
    scopes = tuple(scopes or DEFAULT_SCOPES)
    firstboot = is_firstboot_scope(scopes)
    contract = firstboot_contract(scopes) if firstboot else None
    apps_scope = firstboot or "mcp-apps" in scopes
    codemode_scope = not firstboot and "codemode-errors" in scopes
    profile = json.loads((ROOT / "tests/integrations/python/config.json").read_text())
    if firstboot:
        wanted_clients = contract["clients"]
    elif apps_scope:
        wanted_clients = FIXTURE_NAMES[:3] + (("errorTestServer",) if codemode_scope else ())
    else:
        wanted_clients = ("errorTestServer",)
    clients = [entry for entry in profile["mcp"]["client_configs"] if entry["name"] in wanted_clients]
    if {entry["name"] for entry in clients} != set(wanted_clients):
        raise RuntimeError("shared profile is missing a fixture client after stack integration")
    for entry in clients:
        if entry["name"] == "errorTestServer":
            binary = "error-test-server"
            args = []
        elif entry["name"] == "firstboot_grants_fixture":
            binary = "go-test-server"
            args = entry["stdio_config"].get("args", [])
        else:
            binary = "mcp-apps-test-server"
            args = entry["stdio_config"]["args"]
            if os.environ.get("BIFROST_REFERENCE_HOST_DIRECTORY") and entry["name"] in FIXTURE_NAMES[:3]:
                args = [*args, entry["name"]]
        entry["stdio_config"] = {"command": f"/repo/examples/mcps/{binary}/bin/{binary}", "args": args}
        entry["allow_by_default"] = False
    if firstboot:
        wanted_vmcps = (contract["virtual_mcp"],)
    elif apps_scope:
        wanted_vmcps = ("mcp-apps-full-virtual", "mcp-apps-limited-virtual")
    else:
        wanted_vmcps = ()
    virtual_mcps = [entry for entry in profile["mcp"]["virtual_mcps"]
                    if entry["endpoint_slug"] in wanted_vmcps]
    if firstboot:
        # Each first-boot case exercises its own same-file VMCP declaration; no
        # setup API attachment recreates either declaration after startup.
        if len(virtual_mcps) != 1 or virtual_mcps[0].get("virtual_key_ids") != [contract["virtual_key_id"]]:
            raise RuntimeError(f"shared profile lacks the exact {contract['scope']} Virtual MCP grant")
        wanted_key_values = {contract["virtual_key_value"]}
    else:
        # Defer Virtual MCP grants until after the disposable gateway has loaded
        # file-declared keys; this isolates native relay qualification from #24's
        # separate first-boot reconciliation behavior.
        for entry in virtual_mcps:
            entry.pop("virtual_key_ids", None)
        wanted_key_values = ({KEYS["full"], KEYS["limited"], KEYS["limited_no_app"], KEYS["denied"]}
                             if apps_scope else set())
        if codemode_scope:
            wanted_key_values.add(KEYS["codemode_errors"])
    keys = [entry for entry in profile["governance"]["virtual_keys"] if entry["value"] in wanted_key_values]
    if {entry["value"] for entry in keys} != wanted_key_values or len(virtual_mcps) != len(wanted_vmcps):
        raise RuntimeError("shared profile is missing fixture grants after stack integration")
    config = {
        "setup_token": SETUP_TOKEN,
        "client": {"enable_logging": False, "enforce_governance_header": True,
                   "enforce_auth_on_inference": True},
        "providers": {},
        "mcp": {"client_configs": clients, "virtual_mcps": virtual_mcps,
                "tool_manager_config": {"code_mode_binding_level": "server"}},
        "governance": {"virtual_keys": keys},
    }
    if os.environ.get("BIFROST_REFERENCE_HOST_DIRECTORY"):
        config["client"]["allowed_headers"] = ["MCP-Protocol-Version"]
    return config


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


def firstboot_phase_result(report, phase, return_code, attachment_snapshot, contract=None):
    contract = contract or _SHARED_FIRSTBOOT
    if phase not in ("firstboot", "restart"):
        raise RuntimeError(f"{contract['scope']}: unknown firstboot-grants phase: {phase}")
    by_name = executions_by_name(report, f"{contract['scope']} {phase}")
    if set(by_name) != {contract["request"]}:
        raise RuntimeError(f"{contract['scope']} {phase}: expected the single first-boot request")
    execution = by_name[contract["request"]]
    response = execution.get("response")
    if not isinstance(response, dict):
        raise RuntimeError(f"{contract['scope']} {phase}: Newman response is malformed")
    status = response.get("code")
    if type(status) is not int or not 100 <= status <= 599:
        raise RuntimeError(f"{contract['scope']} {phase}: Newman response status is malformed")
    assertion_contract = firstboot_assertions(contract, status)
    assertion_results = execution.get("assertions")
    if not isinstance(assertion_results, list) or not assertion_results:
        raise RuntimeError(f"{contract['scope']} {phase}: Newman report has no assertion results")
    assertion_names = []
    failures = []
    for item in assertion_results:
        if not isinstance(item, dict) or not isinstance(item.get("assertion"), str) \
                or not item["assertion"].strip():
            raise RuntimeError(f"{contract['scope']} {phase}: Newman assertion results are malformed")
        if item.get("skipped", False) is not False:
            raise RuntimeError(f"{contract['scope']} {phase}: Newman assertion was skipped or has malformed skip state")
        error = item.get("error")
        if error is not None and (not isinstance(error, dict) or not isinstance(error.get("name"), str)
                                  or not error["name"].strip()):
            raise RuntimeError(f"{contract['scope']} {phase}: Newman assertion error is malformed")
        assertion_names.append(item["assertion"])
        if error is not None:
            failures.append(item["assertion"])
    if len(assertion_names) != len(set(assertion_names)):
        raise RuntimeError(f"{contract['scope']} {phase}: Newman assertion results are duplicated")
    if set(assertion_names) != assertion_contract:
        raise RuntimeError(f"{contract['scope']} {phase}: Newman assertion set differs from the pinned fixture contract "
                           f"(expected={sorted(assertion_contract)}, got={sorted(assertion_names)})")
    if not isinstance(attachment_snapshot, dict):
        raise RuntimeError(f"{contract['scope']} {phase}: setup API attachment snapshot is malformed")
    ids = attachment_snapshot.get("virtual_key_ids")
    if not isinstance(ids, list) or any(not isinstance(item, str) or not item for item in ids) \
            or len(ids) != len(set(ids)):
        raise RuntimeError(f"{contract['scope']} {phase}: setup API attachment snapshot is malformed")
    if type(return_code) is not int:
        raise RuntimeError(f"{contract['scope']} {phase}: Newman exit code is malformed")
    return {"phase": phase, "http_status": status, "newman_exit_code": return_code,
            "stored_virtual_key_ids": ids, "assertions": assertion_names,
            "failed_assertions": failures,
            "request": contract["request"]}


def validate_firstboot_phase(result, phase, mode, baseline, contract=None):
    contract = contract or _SHARED_FIRSTBOOT
    if phase not in ("firstboot", "restart") or mode not in ("characterize", "strict"):
        raise RuntimeError(f"firstboot-grants received an invalid phase or comparison mode: {phase}/{mode}")
    if (not isinstance(result, dict) or type(result.get("http_status")) is not int
            or type(result.get("newman_exit_code")) is not int
            or not isinstance(result.get("stored_virtual_key_ids"), list)
            or not isinstance(result.get("failed_assertions"), list)):
        raise RuntimeError(f"{contract['scope']} {phase}: phase result is malformed")
    expected_attached = [contract["virtual_key_id"]]
    if mode == "strict" and not baseline:
        if result["http_status"] != 200 or result["stored_virtual_key_ids"] != expected_attached \
                or result["newman_exit_code"] != 0 or result["failed_assertions"]:
            raise RuntimeError(f"{contract['scope']} {phase}: strict candidate did not pass with exactly one persisted grant")
        return "candidate-green"
    if phase == "firstboot":
        if result["http_status"] != 403 or result["stored_virtual_key_ids"] != [] \
                or result["newman_exit_code"] == 0 \
                or set(result["failed_assertions"]) != firstboot_failure_assertions(contract):
            raise RuntimeError(f"{contract['scope']} firstboot: expected the stock missing-grant 403 with zero stored attachments")
        return "expected-stock-red"
    if result["http_status"] != 200 or result["stored_virtual_key_ids"] != expected_attached \
            or result["newman_exit_code"] != 0 or result["failed_assertions"]:
        raise RuntimeError(f"{contract['scope']} restart: expected a green restart control with exactly one stored attachment")
    return "restart-green-control"


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


def stock_failure_scope(feature, failures):
    allowed = set(BASELINE_ALLOWED_STOCK_FAILURES.get(feature, {}))
    unexpected = [failure for failure in failures
                  if (failure.get("request"), failure.get("assertion")) not in allowed]
    return not unexpected, unexpected


def compare_reports(stock, candidate, feature, mode="characterize"):
    stock_items, candidate_items = executions_by_name(stock, feature + " stock"), executions_by_name(candidate, feature + " candidate")
    classifications = []
    for case, assertion in BASELINE_SENTINELS.get(feature, ()):
        if case not in stock_items or case not in candidate_items:
            raise RuntimeError(f"{feature}: missing named sentinel execution {case!r}")
        stock_failed = failed_assertion(stock_items[case], assertion)
        candidate_failed = failed_assertion(candidate_items[case], assertion)
        state = ("unresolved" if candidate_failed else "fixed") if stock_failed else ("regression" if candidate_failed else "invariant")
        classifications.append({"request": case, "assertion": assertion,
                                "stock": "fail" if stock_failed else "pass",
                                "candidate": "fail" if candidate_failed else "pass", "classification": state})
    for case, assertion in BASELINE_INVARIANTS.get(feature, ()):
        if case not in stock_items or case not in candidate_items:
            raise RuntimeError(f"{feature}: missing invariant execution {case!r}")
        if failed_assertion(stock_items[case], assertion) or failed_assertion(candidate_items[case], assertion):
            raise RuntimeError(f"{feature}: expected stock and candidate invariant to pass: {case!r}")
        classifications.append({"request": case, "assertion": assertion,
                                "stock": "pass", "candidate": "pass", "classification": "green-control"})
    unexpected = {}
    for source_name, executions in (("stock", stock_items), ("candidate", candidate_items)):
        failures = []
        for request_name, execution in executions.items():
            failures.extend({"request": request_name, "assertion": item.get("assertion")}
                            for item in execution.get("assertions", []) if item.get("error") is not None)
        unexpected[source_name] = failures
    result = {"feature": feature, "mode": mode, "named_checks": classifications,
            "stock_failed_assertions": unexpected["stock"],
            "candidate_failed_assertions": unexpected["candidate"],
            "stock_red_sentinels": [item["request"] for item in classifications if item["classification"] == "unresolved" or (item["stock"] == "fail" and item["classification"] == "fixed")],
            "stock_green_invariants": [item["request"] for item in classifications if item["stock"] == "pass"],
            "fixed": [item["request"] for item in classifications if item["classification"] == "fixed"],
            "unresolved": [item["request"] for item in classifications if item["classification"] == "unresolved"],
            "regressions": [item["request"] for item in classifications if item["classification"] == "regression"]}
    result["strict_pass"] = not result["unresolved"] and not result["regressions"]
    return result


def contract_uri(client_name):
    enc = lambda value: base64.urlsafe_b64encode(value.encode()).decode().rstrip("=")
    return f"ui://bifrost/{enc(client_name)}/{enc(FIXTURE_RESOURCE_URI)}"


def seed_contract_uris(collection_path):
    collection = json.loads(collection_path.read_text())
    seeds = {"mcpAppsDirectUri": contract_uri("mcp_apps_fixture"),
             "mcpAppsVirtualUri": contract_uri("mcp_apps_fixture"),
             "mcpAppsAggUri1": contract_uri("mcp_apps_fixture"),
             "mcpAppsAggUri2": contract_uri("mcp_apps_fixture_second"),
             "mcpAppsAggUri3": contract_uri("mcp_apps_fixture_modern"),
             "mcpAppsLegacyUri": contract_uri("mcp_apps_fixture_second"),
             "mcpAppsModernUri": contract_uri("mcp_apps_fixture_modern")}
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
        value = value.replace(SETUP_TOKEN, "<redacted>")
        value = re.sub(r"(?i)(Bearer\s+)[A-Za-z0-9._~+/-]+=*", r"\1<redacted>", value)
        return re.sub(r"sk-bf-[A-Za-z0-9_-]+", "<redacted>", value)
    return value


def write_failure_report(report_dir, error, filename="fixture-error.json"):
    report_dir.mkdir(parents=True, exist_ok=True)
    details = {"error_type": type(error).__name__, "message": redact(str(error))}
    (report_dir / filename).write_text(json.dumps(details, indent=2) + "\n")


def failure_report_filename(args):
    return "stock-fixture-error.json" if len(args) == 3 and args[0] == "--baseline" else "fixture-error.json"


def self_check():
    config = build_config()
    assert config["providers"] == {}
    assert config["setup_token"] == SETUP_TOKEN
    assert len(config["mcp"]["client_configs"]) == 4
    assert all(c["stdio_config"]["command"].startswith("/repo/") for c in config["mcp"]["client_configs"])
    assert all(not key["provider_configs"] for key in config["governance"]["virtual_keys"])
    second = next(c for c in config["mcp"]["client_configs"] if c["name"] == "mcp_apps_fixture_second")
    assert not second.get("is_code_mode_client", False)
    assert second["stdio_config"]["args"] == ["second", "legacy"]
    modern = next(c for c in config["mcp"]["client_configs"] if c["name"] == "mcp_apps_fixture_modern")
    assert modern["stdio_config"]["args"] == ["modern", "modern"]
    assert not modern.get("is_code_mode_client", False)
    assert "allowed_headers" not in config["client"]
    assert all("allowed_headers" not in client for client in config["mcp"]["client_configs"])
    original_host_directory = os.environ.get("BIFROST_REFERENCE_HOST_DIRECTORY")
    try:
        os.environ["BIFROST_REFERENCE_HOST_DIRECTORY"] = "/tmp/pinned-reference-host-self-check"
        host_config = build_config(("mcp-apps",))
    finally:
        if original_host_directory is None:
            os.environ.pop("BIFROST_REFERENCE_HOST_DIRECTORY", None)
        else:
            os.environ["BIFROST_REFERENCE_HOST_DIRECTORY"] = original_host_directory
    host_clients = {client["name"]: client for client in host_config["mcp"]["client_configs"]}
    assert host_clients["mcp_apps_fixture"]["stdio_config"]["args"] == ["first", "dual", "mcp_apps_fixture"]
    assert host_clients["mcp_apps_fixture_second"]["stdio_config"]["args"] == ["second", "legacy", "mcp_apps_fixture_second"]
    assert "allowed_headers" not in host_clients["mcp_apps_fixture_second"]
    assert host_clients["mcp_apps_fixture_modern"]["stdio_config"]["args"] == ["modern", "modern", "mcp_apps_fixture_modern"]
    assert "allowed_headers" not in host_clients["mcp_apps_fixture_modern"]
    assert host_config["client"]["allowed_headers"] == ["MCP-Protocol-Version"]
    assert all("allowed_headers" not in client for client in host_config["mcp"]["client_configs"])
    assert host_config["providers"] == {}
    assert reference_host_status_filename(False) == "reference-host-status.json"
    assert reference_host_status_filename(True) == "stock-reference-host-status.json"
    assert reference_host_status_filename(False) != reference_host_status_filename(True)
    vmcps = {entry["endpoint_slug"]: entry for entry in config["mcp"]["virtual_mcps"]}
    full_vmcp = vmcps["mcp-apps-full-virtual"]
    limited_vmcp = vmcps["mcp-apps-limited-virtual"]
    assert "virtual_key_ids" not in full_vmcp
    assert full_vmcp["tools"] == [{"mcp_client_name": name, "tool_names": ["*"]}
                                  for name in FIXTURE_NAMES[:2]]
    assert "virtual_key_ids" not in limited_vmcp
    assert limited_vmcp["tools"] == [{"mcp_client_name": "mcp_apps_fixture", "tool_names": ["open_view"]}]
    limited = next(key for key in config["governance"]["virtual_keys"] if key["value"] == KEYS["limited_no_app"])
    assert limited["mcp_configs"] == [{"mcp_client_name": "mcp_apps_fixture", "tools_to_execute": ["structured_result"]}]
    configured_scopes = os.environ.pop("BIFROST_FIXTURE_SCOPES", None)
    try:
        assert parse_scopes(None) == DEFAULT_SCOPES
    finally:
        if configured_scopes is not None:
            os.environ["BIFROST_FIXTURE_SCOPES"] = configured_scopes
    assert parse_scopes("codemode-errors") == ("codemode-errors",)
    assert parse_scopes("mcp-apps") == ("mcp-apps",)
    assert parse_scopes("firstboot-grants") == (FIRSTBOOT_SCOPE,)
    assert parse_scopes(FIRSTBOOT_STANDALONE_SCOPE) == (FIRSTBOOT_STANDALONE_SCOPE,)
    for invalid_scopes in ("", "mcp-apps,", "mcp-apps,mcp-apps", "unknown", "firstboot-grants,mcp-apps",
                           f"{FIRSTBOOT_SCOPE},{FIRSTBOOT_STANDALONE_SCOPE}"):
        try:
            parse_scopes(invalid_scopes)
        except RuntimeError:
            pass
        else:
            raise AssertionError(f"invalid feature scopes were accepted: {invalid_scopes!r}")
    assert setup_policy(DEFAULT_SCOPES) == "postseed-app-fixture-vmcps"
    assert setup_policy(("codemode-errors",)) == "none"
    assert setup_policy((FIRSTBOOT_SCOPE,)) == "file-declared-read-only"
    assert setup_policy((FIRSTBOOT_STANDALONE_SCOPE,)) == "file-declared-read-only"
    assert expected_fixture_client_count(DEFAULT_SCOPES) == 4
    assert expected_fixture_client_count((FIRSTBOOT_SCOPE,)) == 3
    assert expected_fixture_client_count((FIRSTBOOT_STANDALONE_SCOPE,)) == 1
    assert expected_fixture_client_count(("mcp-apps",)) == 3
    assert expected_fixture_client_count(("codemode-errors",)) == 1
    assert [entry[0:2] for entry in feature_selections(DEFAULT_SCOPES)] == [("mcp-apps", 39), ("codemode-errors", 3)]
    assert [entry[0:2] for entry in feature_selections(("codemode-errors",))] == [("codemode-errors", 3)]
    assert feature_selections((FIRSTBOOT_SCOPE,))[0][0:2] == (FIRSTBOOT_SCOPE, 1)
    assert feature_selections((FIRSTBOOT_STANDALONE_SCOPE,))[0][0:2] == (FIRSTBOOT_STANDALONE_SCOPE, 1)
    assert require_same_container("same-disposable-container", "same-disposable-container")
    saved_seed = globals().get("seed_virtual_key_attachments")
    try:
        globals()["seed_virtual_key_attachments"] = lambda url: (_ for _ in ()).throw(
            AssertionError("firstboot/code-only mode must never postseed a virtual key"))
        assert setup_virtual_key_attachments("http://fixture.invalid", (FIRSTBOOT_SCOPE,)) == []
        assert setup_virtual_key_attachments("http://fixture.invalid", (FIRSTBOOT_STANDALONE_SCOPE,)) == []
        assert setup_virtual_key_attachments("http://fixture.invalid", ("codemode-errors",)) == []
    finally:
        if saved_seed is not None:
            globals()["seed_virtual_key_attachments"] = saved_seed
    try:
        require_same_container("before", "after")
    except RuntimeError:
        pass
    else:
        raise AssertionError("firstboot restart self-check accepted a replacement container")
    assert firstboot_comparison_classification("firstboot", "candidate-green") == "fixed"
    assert firstboot_comparison_classification("firstboot", "expected-stock-red") == "unresolved"
    assert firstboot_comparison_classification("restart", "candidate-green") == "green-control"
    assert firstboot_comparison_classification("restart", "restart-green-control") == "green-control"
    for phase, gate in (("firstboot", "restart-green-control"), ("restart", "expected-stock-red"),
                        ("other", "candidate-green")):
        try:
            firstboot_comparison_classification(phase, gate)
        except RuntimeError:
            pass
        else:
            raise AssertionError("firstboot comparison mislabeled an inconsistent phase gate")
    firstboot_config = build_config((FIRSTBOOT_SCOPE,))
    assert firstboot_config["providers"] == {}
    assert {client["name"] for client in firstboot_config["mcp"]["client_configs"]} == set(FIXTURE_NAMES[:3])
    firstboot_vmcp = firstboot_config["mcp"]["virtual_mcps"]
    assert len(firstboot_vmcp) == 1 and firstboot_vmcp[0]["endpoint_slug"] == FIRSTBOOT_VIRTUAL_MCP
    assert firstboot_vmcp[0]["virtual_key_ids"] == [FIRSTBOOT_VIRTUAL_KEY]
    assert [key["id"] for key in firstboot_config["governance"]["virtual_keys"]] == [FIRSTBOOT_VIRTUAL_KEY]
    assert "setup-token API" not in json.dumps(firstboot_config)
    standalone = FIRSTBOOT_CONTRACTS[FIRSTBOOT_STANDALONE_SCOPE]
    standalone_config = build_config((FIRSTBOOT_STANDALONE_SCOPE,))
    assert standalone_config["providers"] == {}
    assert [client["name"] for client in standalone_config["mcp"]["client_configs"]] == [standalone["client"]]
    standalone_client = standalone_config["mcp"]["client_configs"][0]
    assert standalone_client["stdio_config"] == {"command": "/repo/examples/mcps/go-test-server/bin/go-test-server",
                                                    "args": []}
    assert standalone_config["mcp"]["virtual_mcps"] == [{
        **next(item for item in json.loads((ROOT / "tests/integrations/python/config.json").read_text())["mcp"]["virtual_mcps"]
               if item["endpoint_slug"] == standalone["virtual_mcp"]),
    }]
    standalone_vmcp = standalone_config["mcp"]["virtual_mcps"][0]
    assert standalone_vmcp["virtual_key_ids"] == [standalone["virtual_key_id"]]
    assert standalone_vmcp["tools"] == [{"mcp_client_name": standalone["client"], "tool_names": [standalone["tool"]]}]
    standalone_keys = standalone_config["governance"]["virtual_keys"]
    assert len(standalone_keys) == 1 and standalone_keys[0]["id"] == standalone["virtual_key_id"]
    assert standalone_keys[0]["value"] == standalone["virtual_key_value"]
    codemode_config = build_config(("codemode-errors",))
    assert [client["name"] for client in codemode_config["mcp"]["client_configs"]] == ["errorTestServer"]
    assert codemode_config["mcp"]["virtual_mcps"] == []
    assert [key["value"] for key in codemode_config["governance"]["virtual_keys"]] == [KEYS["codemode_errors"]]
    apps_config = build_config(("mcp-apps",))
    assert "errorTestServer" not in {client["name"] for client in apps_config["mcp"]["client_configs"]}
    assert KEYS["codemode_errors"] not in {key["value"] for key in apps_config["governance"]["virtual_keys"]}
    stock_firstboot = {"phase": "firstboot", "http_status": 403, "newman_exit_code": 1,
                       "stored_virtual_key_ids": [], "assertions": sorted(FIRSTBOOT_ASSERTIONS["firstboot"]),
                       "failed_assertions": sorted(FIRSTBOOT_EXPECTED_FAILURES),
                       "request": FIRSTBOOT_REQUEST}
    stock_restart = {"phase": "restart", "http_status": 200, "newman_exit_code": 0,
                     "stored_virtual_key_ids": [FIRSTBOOT_VIRTUAL_KEY],
                     "assertions": sorted(FIRSTBOOT_ASSERTIONS["restart"]), "failed_assertions": [],
                     "request": FIRSTBOOT_REQUEST}
    firstboot_report = {"run": {"executions": [{"item": {"name": FIRSTBOOT_REQUEST},
                    "response": {"code": 403},
                    "assertions": [{"assertion": assertion, "skipped": False,
                                    **({"error": {"name": "AssertionError"}}
                                       if assertion in FIRSTBOOT_EXPECTED_FAILURES else {})}
                                   for assertion in sorted(FIRSTBOOT_ASSERTIONS["firstboot"])]}]}}
    firstboot_snapshot = {"endpoint_slug": FIRSTBOOT_VIRTUAL_MCP, "virtual_mcp_id": "fixture-vmcp-id",
                          "virtual_key_ids": []}
    assert firstboot_phase_result(firstboot_report, "firstboot", 1, firstboot_snapshot) == stock_firstboot
    restart_report = {"run": {"executions": [{"item": {"name": FIRSTBOOT_REQUEST},
                   "response": {"code": 200},
                   "assertions": [{"assertion": assertion, "skipped": False}
                                  for assertion in sorted(FIRSTBOOT_ASSERTIONS["restart"])]}]}}
    restart_snapshot = {**firstboot_snapshot, "virtual_key_ids": [FIRSTBOOT_VIRTUAL_KEY]}
    assert firstboot_phase_result(restart_report, "restart", 0, restart_snapshot) == stock_restart
    try:
        firstboot_phase_result({"run": {"executions": []}}, "firstboot", 1, firstboot_snapshot)
    except RuntimeError:
        pass
    else:
        raise AssertionError("firstboot report gate accepted a missing request")
    for bad_assertions in ([],
                           [{"assertion": assertion, "error": {"name": "AssertionError"}}
                            for assertion in sorted(FIRSTBOOT_EXPECTED_FAILURES - {FIRSTBOOT_CALLBACK_ASSERTION})],
                           [{"assertion": assertion, "error": {"name": "AssertionError"}}
                            for assertion in sorted(FIRSTBOOT_EXPECTED_FAILURES)] +
                           [{"assertion": "unrelated assertion", "error": {"name": "AssertionError"}}],
                           [{"assertion": None, "error": {"name": "AssertionError"}}],
                           [{"assertion": assertion, "skipped": True,
                             "error": {"name": "AssertionError"}}
                            for assertion in sorted(FIRSTBOOT_EXPECTED_FAILURES)],
                           [{"assertion": assertion, "skipped": False, "error": "malformed"}
                            for assertion in sorted(FIRSTBOOT_EXPECTED_FAILURES)],
                           [{"assertion": assertion, "error": {"name": "AssertionError"}}
                            for assertion in sorted(FIRSTBOOT_EXPECTED_FAILURES)] +
                           [{"assertion": FIRSTBOOT_STATUS_ASSERTION, "error": {"name": "AssertionError"}}]):
        malformed_report = {"run": {"executions": [{"item": {"name": FIRSTBOOT_REQUEST},
                              "response": {"code": 403}, "assertions": bad_assertions}]}}
        try:
            firstboot_phase_result(malformed_report, "firstboot", 1, firstboot_snapshot)
        except RuntimeError:
            pass
        else:
            raise AssertionError("firstboot report gate accepted missing, extra, or absent assertion results")
    assert validate_firstboot_phase(stock_firstboot, "firstboot", "characterize", True) == "expected-stock-red"
    assert validate_firstboot_phase(stock_restart, "restart", "characterize", True) == "restart-green-control"
    candidate_firstboot_report = {"run": {"executions": [{"item": {"name": FIRSTBOOT_REQUEST},
                                  "response": {"code": 200},
                                  "assertions": [{"assertion": assertion, "skipped": False}
                                                 for assertion in sorted(FIRSTBOOT_ASSERTIONS["firstboot-green"])]}]}}
    candidate_snapshot = {**firstboot_snapshot, "virtual_key_ids": [FIRSTBOOT_VIRTUAL_KEY]}
    candidate_green = firstboot_phase_result(candidate_firstboot_report, "firstboot", 0, candidate_snapshot)
    assert candidate_green["assertions"] == sorted(FIRSTBOOT_ASSERTIONS["firstboot-green"])
    assert validate_firstboot_phase(candidate_green, "firstboot", "strict", False) == "candidate-green"
    assert validate_firstboot_phase({**candidate_green, "phase": "restart"}, "restart", "strict", False) == "candidate-green"
    for malformed in ({**candidate_green, "stored_virtual_key_ids": []},
                      {**candidate_green, "failed_assertions": ["unrelated assertion"]},
                      {**candidate_green, "newman_exit_code": 1}):
        try:
            validate_firstboot_phase(malformed, "firstboot", "strict", False)
        except RuntimeError:
            pass
        else:
            raise AssertionError("strict firstboot candidate gate accepted a failed state or report")
    standalone_stock_failures = firstboot_failure_assertions(standalone)
    standalone_stock_report = {"run": {"executions": [{"item": {"name": standalone["request"]},
        "response": {"code": 403},
        "assertions": [{"assertion": assertion, "skipped": False, "error": {"name": "AssertionError"}}
                       for assertion in sorted(standalone_stock_failures)]}]}}
    standalone_snapshot = {"endpoint_slug": standalone["virtual_mcp"], "virtual_mcp_id": "standalone-vmcp-id",
                           "virtual_key_ids": []}
    standalone_stock = firstboot_phase_result(standalone_stock_report, "firstboot", 1,
                                              standalone_snapshot, standalone)
    assert set(standalone_stock["failed_assertions"]) == standalone_stock_failures
    assert validate_firstboot_phase(standalone_stock, "firstboot", "characterize", True,
                                    standalone) == "expected-stock-red"
    standalone_green_report = {"run": {"executions": [{"item": {"name": standalone["request"]},
        "response": {"code": 200},
        "assertions": [{"assertion": assertion, "skipped": False}
                       for assertion in sorted(firstboot_assertions(standalone, 200))]}]}}
    standalone_green_snapshot = {**standalone_snapshot, "virtual_key_ids": [standalone["virtual_key_id"]]}
    standalone_green = firstboot_phase_result(standalone_green_report, "firstboot", 0,
                                              standalone_green_snapshot, standalone)
    assert validate_firstboot_phase(standalone_green, "firstboot", "strict", False,
                                    standalone) == "candidate-green"
    assert validate_firstboot_phase(standalone_green, "restart", "strict", False,
                                    standalone) == "candidate-green"
    contaminated = json.loads(json.dumps(standalone_stock_report))
    contaminated["run"]["executions"][0]["assertions"].append(
        {"assertion": "unrelated assertion", "skipped": False, "error": {"name": "AssertionError"}})
    try:
        firstboot_phase_result(contaminated, "firstboot", 1, standalone_snapshot, standalone)
    except RuntimeError:
        pass
    else:
        raise AssertionError("standalone stock gate accepted an unrelated failed assertion")
    source_path = Path(os.environ.get("BIFROST_TEST_COLLECTION_SOURCE",
                                    ROOT / "tests/e2e/api/collections/provider-harness.json"))
    if not source_path.is_file():
        raise RuntimeError(f"offline collection source is missing: {source_path}")
    source_data = json.loads(source_path.read_text())
    source_request_names = {item.get("name") for item in items_in(source_data.get("item", []))}
    if any(contract["request"] not in source_request_names for contract in FIRSTBOOT_CONTRACTS.values()):
        raise RuntimeError("offline collection source lacks a firstboot-grants contract request")
    standalone_items = [item for item in items_in(source_data.get("item", []))
                        if item.get("name") == standalone["request"]]
    if len(standalone_items) != 1:
        raise RuntimeError("offline collection source lacks the unique standalone firstboot request")
    try:
        standalone_call = json.loads(standalone_items[0]["request"]["body"]["raw"])
    except (KeyError, TypeError, json.JSONDecodeError) as error:
        raise RuntimeError("standalone firstboot request is not a literal JSON-RPC tools/call") from error
    if (standalone_call.get("method") != "tools/call"
            or standalone_call.get("params", {}).get("name") != standalone["request_tool"]):
        raise RuntimeError("standalone firstboot request does not call its canonical namespaced fixture tool")
    def immediate_parent_name(items, request_name):
        for item in items:
            if item.get("name") == request_name and isinstance(item.get("request"), dict):
                return None
            children = item.get("item") if isinstance(item, dict) else None
            if isinstance(children, list):
                found = immediate_parent_name(children, request_name)
                if found is not ...:
                    return item.get("name") if found is None else found
        return ...
    for contract in FIRSTBOOT_CONTRACTS.values():
        if immediate_parent_name(source_data.get("item", []), contract["request"]) != contract["folder"]:
            raise RuntimeError(f"{contract['scope']} harness folder must exactly match its Make/Newman folder argument")
    with tempfile.TemporaryDirectory(prefix="bifrost-fixture-filter-check-") as temp:
        temp = Path(temp)
        augmented = temp / "augmented.json"
        subprocess.run(["node", str(ROOT / "tests/e2e/api/runners/augment-provider-harness.mjs"),
                        "--source", str(source_path), "--out", str(augmented)],
                       cwd=ROOT, check=True, stdout=subprocess.DEVNULL)
        filter_script = ROOT / "tests/e2e/api/runners/filter-collection.mjs"
        selections = (
            ("apps-all", ["--feature", "mcp-apps"], 40),
            ("apps-default", ["--feature", "mcp-apps", "--exclude-feature-any", FIRSTBOOT_SCOPE], 39),
            ("codemode-errors", ["--feature", "codemode-errors"], 3),
            ("firstboot-feature-family", ["--feature", FIRSTBOOT_SCOPE], 2),
            ("firstboot-feature", ["--feature", FIRSTBOOT_SCOPE, "--folder", FIRSTBOOT_FOLDER], 1),
            ("firstboot-standalone-feature", ["--feature", FIRSTBOOT_STANDALONE_SCOPE], 1),
            ("firstboot-standalone-folder", ["--feature", FIRSTBOOT_STANDALONE_SCOPE,
                                             "--folder", FIRSTBOOT_CONTRACTS[FIRSTBOOT_STANDALONE_SCOPE]["folder"]], 1),
        )
        for name, selectors, expected_count in selections:
            filtered = temp / f"{name}.json"
            subprocess.run(["node", str(filter_script), "--source", str(augmented), "--out", str(filtered),
                            *selectors], cwd=ROOT, check=True, stdout=subprocess.DEVNULL)
            actual = len(list(requests_in(json.loads(filtered.read_text()).get("item", []))))
            if actual != expected_count:
                raise RuntimeError(f"offline filter {name}: expected {expected_count} requests, found {actual}")
    makefile = (ROOT / "Makefile").read_text()
    if ('else EXCLUDE_FLAG="--exclude-feature-any $(EXCLUDE_FEATURE_ANY)"' not in makefile
            or 'EXCLUDE_FLAG="--exclude-feature-any cache-parity,$(EXCLUDE_FEATURE_ANY)"' not in makefile):
        raise RuntimeError("Make target does not forward the comparator's explicit request exclusion")
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
    assert contract_uri("mcp_apps_fixture_modern") != contract_uri("mcp_apps_fixture")
    assert contract_uri("mcp_apps_fixture_modern") != contract_uri("mcp_apps_fixture_second")
    assert len({contract_uri(name) for name in FIXTURE_NAMES[:3]}) == 3
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
    stock_report = report({case for checks in BASELINE_SENTINELS.values() for case, _ in checks})
    candidate_report = report()
    for feature in BASELINE_SENTINELS:
        assert set(BASELINE_SENTINELS[feature]) <= set(BASELINE_ALLOWED_STOCK_FAILURES[feature]), \
            f"{feature}: every named stock sentinel must have an explicit exact-pair rationale"
        subset = lambda source: {"run": {"executions": [e for e in source["run"]["executions"]
                                                          if any(e["item"]["name"] == case for case, _ in BASELINE_SENTINELS[feature] + BASELINE_INVARIANTS[feature])]}}
        expected = [case for case, _ in BASELINE_SENTINELS[feature] + BASELINE_INVARIANTS[feature]]
        validate_executions(subset(stock_report), feature, len(expected), expected)
        validate_executions(subset(candidate_report), feature, len(expected), expected)
        classified = compare_reports(subset(stock_report), subset(candidate_report), feature)
        assert classified["fixed"] == [case for case, _ in BASELINE_SENTINELS[feature]]
        in_scope, unexpected = stock_failure_scope(feature, classified["stock_failed_assertions"])
        assert in_scope and not unexpected, "named stock sentinel assertions should be allowed red evidence"
        expected_pair = next(iter(BASELINE_ALLOWED_STOCK_FAILURES[feature]))
        expected_failure = {"request": expected_pair[0], "assertion": expected_pair[1]}
        assert stock_failure_scope(feature, [expected_failure]) == (True, []), \
            "explicitly classified exact stock behavior was rejected"
        unresolved = compare_reports(subset(stock_report), subset(stock_report), feature)
        assert unresolved["unresolved"] == [case for case, _ in BASELINE_SENTINELS[feature]]
        assert not compare_reports(subset(stock_report), subset(stock_report), feature, mode="strict")["strict_pass"]
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
        unrelated_stock = json.loads(json.dumps(subset(candidate_report)))
        sentinel_case = BASELINE_SENTINELS[feature][0][0]
        executions_by_name(unrelated_stock, feature)[sentinel_case]["assertions"].append(
            {"assertion": "unrelated stock assertion", "error": {"name": "AssertionError"}})
        contaminated = compare_reports(unrelated_stock, subset(candidate_report), feature)
        in_scope, unexpected = stock_failure_scope(feature, contaminated["stock_failed_assertions"])
        assert not in_scope and unexpected == [{"request": sentinel_case, "assertion": "unrelated stock assertion"}], \
            "baseline comparator accepted an unrelated failure on a sentinel request"
        unrelated_candidate = json.loads(json.dumps(subset(candidate_report)))
        executions_by_name(unrelated_candidate, feature)[sentinel_case]["assertions"].append(
            {"assertion": "unrelated candidate assertion", "error": {"name": "AssertionError"}})
        in_scope, unexpected = stock_failure_scope(
            feature, compare_reports(subset(stock_report), unrelated_candidate, feature)["candidate_failed_assertions"])
        assert not in_scope and unexpected == [{"request": sentinel_case, "assertion": "unrelated candidate assertion"}], \
            "characterization comparator accepted an unrelated candidate failure"
    candidate_names = {case for case, _ in BASELINE_SENTINELS["codemode-errors"] + BASELINE_INVARIANTS["codemode-errors"]}
    candidate_subset = {"run": {"executions": [e for e in candidate_report["run"]["executions"]
                                              if e["item"]["name"] in candidate_names]}}
    sentinel_case = BASELINE_SENTINELS["codemode-errors"][0][0]
    candidate_sentinel = next(e for e in candidate_subset["run"]["executions"] if e["item"]["name"] == sentinel_case)
    candidate_sentinel["assertions"][0]["error"] = {"name": "AssertionError"}
    strict_result = compare_reports({"run": {"executions": [e for e in stock_report["run"]["executions"]
                                                              if e["item"]["name"] in candidate_names]}},
                                    candidate_subset, "codemode-errors", mode="strict")
    assert not strict_result["strict_pass"], "baseline comparator accepted a failing candidate sentinel"
    with tempfile.TemporaryDirectory() as temp:
        collection_path = Path(temp) / "collection.json"
        collection_path.write_text(json.dumps({"item": [], "variable": []}))
        seeds = seed_contract_uris(collection_path)
        written = json.loads(collection_path.read_text())
        assert {entry["key"]: entry["value"] for entry in written["variable"]} == seeds
    safe = redact({"Authorization": "Bearer sk-bf-example",
                   "body": f"token {SETUP_TOKEN} sk-bf-example"})
    assert safe == {"Authorization": "<redacted>", "body": "token <redacted> <redacted>"}
    with tempfile.TemporaryDirectory() as temp:
        assert failure_report_filename(["--baseline", "stock-image", "reports"]) == "stock-fixture-error.json"
        assert failure_report_filename(["candidate-image", "reports"]) == "fixture-error.json"
        write_failure_report(Path(temp), RuntimeError(f"Authorization: Bearer sk-bf-example; {SETUP_TOKEN}"))
        failure = json.loads((Path(temp) / "fixture-error.json").read_text())
        assert failure["error_type"] == "RuntimeError"
        assert "sk-bf-example" not in failure["message"]
        assert SETUP_TOKEN not in failure["message"] and "<redacted>" in failure["message"]
        write_failure_report(Path(temp), RuntimeError("stock Newman report missing"), "stock-fixture-error.json")
        assert "stock Newman report missing" in json.loads(
            (Path(temp) / "stock-fixture-error.json").read_text())["message"]
        assert "stock Newman report missing" not in (Path(temp) / "fixture-error.json").read_text()
    saved_health, saved_rpc, saved_setup_request = wait_health, rpc, setup_request
    saved_sleep, saved_clock = time.sleep, time.monotonic
    try:
        globals()["wait_health"] = lambda url: None
        time.sleep = lambda seconds: None
        incomplete = {"result": {"tools": [{"name": "executeToolCode"}]}}
        complete = {"result": {"tools": [{"name": "executeToolCode"},
                                            {"name": "mcp_apps_fixture_second-open_view"},
                                            {"name": "mcp_apps_fixture_second-save_state"},
                                            {"name": "mcp_apps_fixture_modern-open_view"},
                                            {"name": "mcp_apps_fixture_modern-save_state"}]}}
        for initial in ((200, incomplete), (503, complete)):
            replies = iter((initial, (200, complete)))
            calls = []
            def fake_rpc(url, path, method, params, token):
                assert (path, method, params, token) == ("/mcp", "tools/list", {}, KEYS["full"])
                calls.append(path)
                return next(replies)
            globals()["rpc"] = fake_rpc
            ready_characterization("http://fixture.invalid")
            assert len(calls) == 2, "stock readiness accepted an incomplete or failed catalog"
        clock = iter((0, 0, 181))
        time.monotonic = lambda: next(clock)
        globals()["rpc"] = lambda *args: (200, incomplete)
        try:
            ready_characterization("http://fixture.invalid")
        except RuntimeError as error:
            assert "stock aggregate tool catalog incomplete" in str(error)
        else:
            raise AssertionError("stock readiness accepted a permanently incomplete catalog")
        time.monotonic = saved_clock
        for contract in FIRSTBOOT_CONTRACTS.values():
            client_name = contract["discovery_clients"][0]
            unrelated = {"config": {"name": "unrelated_client"}, "state": "error", "tools": []}
            needed = {"config": {"name": client_name}, "state": "healthy",
                      "tools": [{"name": contract["tool"]}]}
            setup_calls = []
            def fake_setup_request(url, path, method):
                assert (path, method) == ("/api/mcp/clients", "GET")
                setup_calls.append(path)
                return 200, {"clients": [unrelated, needed]}
            globals()["setup_request"] = fake_setup_request
            ready_firstboot_fixture_catalog("http://fixture.invalid", contract)
            assert setup_calls and set(setup_calls) == {"/api/mcp/clients"}
            missing_tool = {**needed, "tools": [{"name": "unrelated_tool"}]}
            readiness_clock = iter((0, 0, 181))
            time.monotonic = lambda: next(readiness_clock)
            globals()["setup_request"] = lambda *args: (200, {"clients": [unrelated, missing_tool]})
            try:
                ready_firstboot_fixture_catalog("http://fixture.invalid", contract)
            except RuntimeError as error:
                assert client_name in str(error)
                assert "unrelated_tool" in str(error) and contract["tool"] in str(error)
            else:
                raise AssertionError(f"{contract['scope']} readiness accepted a source missing its requested tool")
            time.monotonic = saved_clock
        globals()["setup_request"] = saved_setup_request
    finally:
        globals()["wait_health"], globals()["rpc"] = saved_health, saved_rpc
        globals()["setup_request"] = saved_setup_request
        time.sleep, time.monotonic = saved_sleep, saved_clock
    print("PASS fixture CI self-check (source identities, config scope, route/name guards, observed classification, URI seeds, report redaction, catalog readiness)")


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


def setup_request(url, path, method):
    request = urllib.request.Request(
        url + path,
        data=b"{}" if method == "POST" else None,
        headers={"X-Bifrost-Setup-Token": SETUP_TOKEN, "Content-Type": "application/json"},
        method=method,
    )
    try:
        response = urllib.request.urlopen(request, timeout=15)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        raw = response.read().decode()
        return response.status, json.loads(raw) if raw else {}


def seed_virtual_key_attachments(url):
    expected = {"mcp-apps-full-virtual": "vk-mcp-apps-full",
                "mcp-apps-limited-virtual": "vk-mcp-apps-limited"}
    deadline = time.monotonic() + 180
    last_error = "setup API has not exposed both fixture Virtual MCPs"
    while time.monotonic() < deadline:
        try:
            status, body = setup_request(url, "/api/mcp/virtual-mcps", "GET")
            virtual_mcps = {item["endpoint_slug"]: item for item in body.get("virtual_mcps", [])}
            if status == 200 and expected.keys() <= virtual_mcps.keys():
                attachments = []
                for slug, virtual_key_id in expected.items():
                    definition = virtual_mcps[slug]
                    if virtual_key_id not in definition.get("virtual_key_ids", []):
                        attach_path = f"/api/mcp/virtual-mcps/{definition['id']}/virtual-keys/{virtual_key_id}"
                        attach_status, attach_body = setup_request(url, attach_path, "POST")
                        if attach_status != 200 or attach_body.get("success") is not True:
                            raise RuntimeError(f"setup-token fixture attachment failed for {slug} (HTTP {attach_status})")
                    attachments.append({"endpoint_slug": slug, "virtual_key_id": virtual_key_id,
                                        "setup": "configured setup-token API after gateway startup"})
                return attachments
            last_error = f"setup API HTTP {status}; fixture Virtual MCPs visible={sorted(virtual_mcps)}"
        except (OSError, urllib.error.URLError, KeyError, ValueError) as error:
            last_error = str(error)
        time.sleep(1)
    raise RuntimeError("could not seed disposable fixture Virtual MCP grants: " + last_error)


def wait_file_declared_virtual_mcp(url, contract):
    deadline = time.monotonic() + 180
    last_error = f"setup API has not exposed the {contract['scope']} file-declared Virtual MCP"
    while time.monotonic() < deadline:
        try:
            status, body = setup_request(url, "/api/mcp/virtual-mcps", "GET")
            virtual_mcps = {item["endpoint_slug"]: item for item in body.get("virtual_mcps", [])}
            definition = virtual_mcps.get(contract["virtual_mcp"])
            if status == 200 and definition:
                ids = definition.get("virtual_key_ids", [])
                if not isinstance(ids, list):
                    raise RuntimeError("setup API returned malformed virtual_key_ids")
                return {"endpoint_slug": contract["virtual_mcp"], "virtual_mcp_id": definition["id"],
                        "virtual_key_ids": ids}
            last_error = f"setup API HTTP {status}; visible Virtual MCPs={sorted(virtual_mcps)}"
        except (OSError, urllib.error.URLError, KeyError, ValueError) as error:
            last_error = str(error)
        time.sleep(1)
    raise RuntimeError("could not read first-boot Virtual MCP state: " + last_error)


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


def ready_characterization(url):
    wait_health(url)
    # Characterization accepts the actual stock catalog, including stock's
    # omission of direct tools from the Code-Mode fixture source.
    deadline = time.monotonic() + 180
    names = set()
    required = {"executeToolCode", "mcp_apps_fixture_second-open_view", "mcp_apps_fixture_second-save_state",
                "mcp_apps_fixture_modern-open_view", "mcp_apps_fixture_modern-save_state"}
    while time.monotonic() < deadline:
        try:
            status, body = rpc(url, "/mcp", "tools/list", {}, KEYS["full"])
            names = {tool.get("name", "") for tool in body.get("result", {}).get("tools", [])}
            if status == 200 and required <= names:
                print("PASS characterization health and stock-compatible MCP catalog readiness", flush=True)
                return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(1)
    raise RuntimeError("stock aggregate tool catalog incomplete after 180 seconds: " + ", ".join(sorted(names)))


def ready_codemode_errors(url):
    wait_health(url)
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            status, body = rpc(url, "/mcp/error-test-server", "tools/list", {}, KEYS["codemode_errors"])
            names = {tool.get("name", "") for tool in body.get("result", {}).get("tools", [])}
            if status == 200 and "executeToolCode" in names:
                print("PASS Code Mode error fixture catalog readiness", flush=True)
                return
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(1)
    raise RuntimeError("Code Mode error fixture catalog incomplete after 180 seconds")


def ready_firstboot_fixture_catalog(url, contract, diagnostic_path=None):
    wait_health(url)
    deadline = time.monotonic() + 180
    last = {"setup_api_status": None, "clients": {}}
    while time.monotonic() < deadline:
        try:
            status, body = setup_request(url, "/api/mcp/clients", "GET")
            clients = body.get("clients") if isinstance(body, dict) else None
            if not isinstance(clients, list):
                last = {"setup_api_status": status, "required_tool": contract["tool"],
                        "error": "response omitted the clients array", "clients": {}}
            else:
                by_name = {}
                for client in clients:
                    if not isinstance(client, dict):
                        continue
                    config = client.get("config")
                    name = config.get("name") if isinstance(config, dict) else None
                    if isinstance(name, str):
                        tools = client.get("tools")
                        tool_names = sorted(tool.get("name", "") for tool in tools
                                            if isinstance(tool, dict) and isinstance(tool.get("name"), str)) \
                            if isinstance(tools, list) else []
                        by_name[name] = {"state": client.get("state"),
                                         "tool_count": len(tool_names), "tool_names": tool_names,
                                         "last_failure": redact(client.get("last_failure"))}
                selected = {name: by_name.get(name) for name in contract["discovery_clients"]}
                last = {"setup_api_status": status, "required_tool": contract["tool"], "clients": selected}
                if status == 200 and all(info and info["state"] == "healthy"
                                         and contract["tool"] in info["tool_names"]
                                         for info in selected.values()):
                    if diagnostic_path:
                        diagnostic_path.write_text(json.dumps(redact({**last, "ready": True}), indent=2) + "\n")
                    print(f"PASS {contract['scope']} file-declared source discovery is ready", flush=True)
                    return
        except (OSError, urllib.error.URLError, ValueError, TypeError) as error:
            last = {"setup_api_status": None,
                    "required_tool": contract["tool"],
                    "error": f"{type(error).__name__}: {error}", "clients": {}}
        if diagnostic_path:
            diagnostic_path.write_text(json.dumps(redact({**last, "ready": False}), indent=2) + "\n")
        time.sleep(1)
    raise RuntimeError(f"{contract['scope']} file-declared source discovery incomplete after 180 seconds: "
                       + json.dumps(last))


def ready(url, scopes=DEFAULT_SCOPES):
    wait_health(url)

    routes = []
    if "mcp-apps" in scopes:
        routes.extend((("/mcp/mcp-apps-fixture", "full", False),
                       ("/mcp/mcp-apps-fixture-second", "full", False),
                       ("/mcp/mcp-apps-fixture-modern", "full", False),
                       ("/mcp/mcp-apps-full-virtual", "full", False),
                       ("/mcp/mcp-apps-limited-virtual", "limited", False),
                       ("/mcp", "full", False)))
    if "codemode-errors" in scopes:
        routes.append(("/mcp/error-test-server", "codemode_errors", True))
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
        if path == "/mcp" and not {"executeToolCode", "mcp_apps_fixture_second-save_state",
                                    "mcp_apps_fixture_modern-open_view", "mcp_apps_fixture_modern-save_state"} <= names:
            raise RuntimeError("aggregate catalog is incomplete for the configured fixture sources")
        if key == "limited" and (not any(name.endswith("open_view") for name in names)
                                  or any(name.endswith("fail_tool") for name in names)):
            raise RuntimeError("limited Virtual MCP readiness did not enforce its tool grant")
    print(f"PASS candidate health and MCP readiness on {len(routes)} selected fixture routes", flush=True)


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


def container_identity(container, app_dir, env):
    raw = subprocess.check_output(["docker", "inspect", container], text=True, env=env)
    inspected = json.loads(raw)[0]
    matching_mounts = [mount for mount in inspected.get("Mounts", [])
                       if mount.get("Destination") == "/fixture-data"]
    expected_source = str(app_dir.resolve())
    if len(matching_mounts) != 1 or matching_mounts[0].get("Source") != expected_source:
        raise RuntimeError("firstboot-grants container no longer uses the original disposable APP_DIR")
    if "APP_DIR=/fixture-data" not in inspected.get("Config", {}).get("Env", []):
        raise RuntimeError("firstboot-grants container lost its original APP_DIR")
    return inspected["Id"]


def run_firstboot_harness_phase(root, temp, report_dir, base_url, app_dir, source_args,
                                env, baseline, phase, contract):
    command = ["make", "run-provider-harness-test", "CI=1", "PARALLEL=0",
               f"FEATURE={contract['scope']}", f'FOLDER={contract["folder"]}',
               f"BASE_URL={base_url}", f"APP_DIR={app_dir}", "HARNESS_MAX_REQUESTS=1",
               *source_args]
    stem = f"{contract['scope']}-{phase}"
    log = temp / f"{stem}.log"
    (root / "tmp/newman-report.json").unlink(missing_ok=True)
    (root / "tmp/harness-failures.md").unlink(missing_ok=True)
    (root / "tmp/newman-cli.log").unlink(missing_ok=True)
    rc = run(command, root, env, log)
    raw_report = temp / f"raw-{stem}-newman-report.json"
    raw_failures = temp / f"raw-{stem}-harness-failures.md"
    raw_cli = temp / f"raw-{stem}-newman-cli.log"
    raw_filtered = temp / f"raw-{stem}-filtered-collection.json"
    if (root / "tmp/newman-report.json").is_file():
        shutil.copy2(root / "tmp/newman-report.json", raw_report)
    if (root / "tmp/harness-failures.md").is_file():
        shutil.copy2(root / "tmp/harness-failures.md", raw_failures)
    if (root / "tmp/newman-cli.log").is_file():
        shutil.copy2(root / "tmp/newman-cli.log", raw_cli)
    if (root / "tmp/harness-filtered.json").is_file():
        shutil.copy2(root / "tmp/harness-filtered.json", raw_filtered)
    prefix = "stock-" if baseline else ""
    artifact_stem = f"{prefix}{stem}"
    archive_report(log, report_dir / f"{artifact_stem}-runner.log")
    archive_report(root / "tmp/harness-quiet.log", report_dir / f"{artifact_stem}-harness.log")
    archive_report(raw_cli, report_dir / f"{artifact_stem}-newman-cli.log")
    archive_report(raw_filtered, report_dir / f"{artifact_stem}-filtered-collection.json")
    report_ok = archive_report(raw_report, report_dir / f"{artifact_stem}-newman-report.json")
    archive_report(raw_failures, report_dir / f"{artifact_stem}-harness-failures.md")
    (report_dir / f"{artifact_stem}-exit-code.txt").write_text(str(rc) + "\n")
    if not report_ok:
        raise RuntimeError(f"{contract['scope']} {phase}: Make target produced no Newman JSON report")
    data = json.loads(raw_report.read_text())
    validate_executions(data, f"{contract['scope']} {phase}", 1, [contract["request"]])
    return data, rc


def main(args):
    if args == ["--self-check"]:
        self_check()
        return 0
    scopes = parse_scopes()
    firstboot = is_firstboot_scope(scopes)
    contract = firstboot_contract(scopes) if firstboot else None
    baseline = args[0] == "--baseline" if args else False
    if baseline and len(args) == 3:
        stock_image, report_dir_arg = args[1], args[2]
        candidate_image = None
    elif not baseline and len(args) == 2:
        candidate_image, report_dir_arg = args
        stock_image = None
    else:
        print("usage: test-mcp-apps-fixture.sh IMAGE REPORT_DIR | --baseline STOCK_IMAGE REPORT_DIR | --self-check", file=sys.stderr)
        return 2
    mode = os.environ.get("BIFROST_COMPARISON_MODE", "characterize")
    if mode not in ("characterize", "strict"):
        raise RuntimeError("BIFROST_COMPARISON_MODE must be characterize or strict")
    source_identities = {
        "stock": os.environ.get("BIFROST_STOCK_SOURCE", ""),
        "candidate": os.environ.get("BIFROST_CANDIDATE_SOURCE", ""),
        "stable_reference": os.environ.get("BIFROST_STABLE_REFERENCE", ""),
    }
    for name, identity in source_identities.items():
        if not re.fullmatch(r"[0-9a-f]{40}", identity):
            raise RuntimeError(f"{name} source identity must be an immutable 40-character commit SHA")
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
    modules = (((ROOT / "examples/mcps/go-test-server", "go-test-server"),) if firstboot and
               scopes == (FIRSTBOOT_STANDALONE_SCOPE,) else
               ((ROOT / "examples/mcps/mcp-apps-test-server", "mcp-apps-test-server"),
                (ROOT / "examples/mcps/error-test-server", "error-test-server")))
    for module, _ in modules:
        if not (module / "go.mod").is_file():
            raise RuntimeError(f"fixture source missing after merge: {module.relative_to(ROOT)}")
        subprocess.run(["go", "test", "-count=1", "./..."], cwd=module,
                       env={**os.environ, "GOWORK": "off"}, check=True)
    if not source.is_file() or not augment.is_file() or not filter_script.is_file():
        raise RuntimeError("MCP Apps harness cases or filter scripts are missing")

    with tempfile.TemporaryDirectory(prefix="bifrost-mcp-fixture-") as tmp:
        temp = Path(tmp)
        app_dir = temp / "app"
        app_dir.mkdir(mode=0o700)
        config_path = app_dir / "config.json"
        config_path.write_text(json.dumps(build_config(scopes), indent=2) + "\n")
        config_path.chmod(0o600)

        env = {"PATH": os.environ.get("PATH", "/usr/bin:/bin"),
               "HOME": os.environ.get("HOME", str(temp)), "TMPDIR": tmp,
               "USE_INFISICAL": "0", "CI": "1",
               "BIFROST_SETUP_TOKEN": SETUP_TOKEN,
               "BIFROST_E2E_SETUP_TOKEN": SETUP_TOKEN}
        harness_source = temp / ("stock-provider-harness.json" if baseline else "candidate-provider-harness.json")
        shutil.copy2(source, harness_source)
        # Seed known contract URIs independently of discovery assertions. A
        # stock discovery failure must not make Newman skip later resource
        # requests; those responses are separate evidence in the same run.
        uri_seeds = {} if firstboot else seed_contract_uris(harness_source)
        make_source_args = [f"HARNESS_SOURCE_COLLECTION={harness_source}"]
        augmented = temp / "augmented.json"
        subprocess.run(["node", str(augment), "--source", str(harness_source), "--out", str(augmented)],
                       cwd=ROOT, env=env, check=True)
        counts = {}
        expected_names = {}
        selections = feature_selections(scopes)
        for feature, expected, selectors in selections:
            filtered = temp / f"filtered-{feature}.json"
            subprocess.run(["node", str(filter_script), "--source", str(augmented), "--out", str(filtered),
                            *selectors], cwd=ROOT, env=env, check=True)
            filtered_data = json.loads(filtered.read_text())
            filtered_items = list(items_in(filtered_data.get("item", [])))
            names = [item.get("name") for item in filtered_items]
            if not names or any(not name for name in names) or len(names) != len(set(names)):
                raise RuntimeError(f"{feature}: filtered request names are missing or duplicated")
            expected_names[feature] = names
            count = len(list(requests_in(filtered_data.get("item", []))))
            counts[feature] = validate_collection(filtered, feature, expected)
        expected_scope_counts = ({scopes[0]: 1} if firstboot else
                                 {feature: expected for feature, expected in (("mcp-apps", 39), ("codemode-errors", 3))
                                  if feature in scopes})
        if counts != expected_scope_counts:
            raise RuntimeError(f"filtered harness scopes differ from selected scope contract: {counts}")
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
            expected_clients = expected_fixture_client_count(scopes)
            # The selected stdio clients must exist in the read-only repo mount at exact paths.
            if len(base_config["mcp"]["client_configs"]) != expected_clients:
                raise RuntimeError(f"expected {expected_clients} checked-in synthetic fixture clients")
            image = stock_image if baseline else candidate_image
            if subprocess.run(["docker", "image", "inspect", image], stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL).returncode:
                raise RuntimeError(f"source-pinned image not present in Docker: {image}")
            subprocess.run(["docker", "run", "-d", "--platform", "linux/amd64", "--user", f"{os.getuid()}:{os.getgid()}", "--name", container, "-p", "127.0.0.1::8080",
                            "-e", "APP_DIR=/fixture-data", "-e", "LOG_LEVEL=warn",
                            "-v", f"{ROOT}:/repo:ro", "-v", f"{app_dir}:/fixture-data",
                            "-w", "/repo/transports/bifrost-http", image],
                           cwd=ROOT, env=env, check=True, stdout=subprocess.DEVNULL)
            container_started = True
            port_line = subprocess.check_output(["docker", "port", container, "8080/tcp"], text=True).splitlines()[0]
            port = int(port_line.rsplit(":", 1)[1])
            base_url = f"http://127.0.0.1:{port}"
            if firstboot:
                run_reference_host(report_dir, base_url, source_identities, baseline, scopes)
                observations = {}
                container_ids = {}
                classifications = {}
                for phase in ("firstboot", "restart"):
                    if phase == "restart":
                        container_ids["before_restart"] = container_identity(container, app_dir, env)
                        subprocess.check_output(["docker", "restart", container], text=True, env=env)
                        container_ids["after_restart"] = container_identity(container, app_dir, env)
                        require_same_container(container_ids["before_restart"], container_ids["after_restart"])
                    prefix = "stock-" if baseline else ""
                    readiness_path = report_dir / f"{prefix}{contract['scope']}-{phase}-source-discovery.json"
                    ready_firstboot_fixture_catalog(base_url, contract, readiness_path)
                    snapshot = wait_file_declared_virtual_mcp(base_url, contract)
                    phase_stem = f"{contract['scope']}-{phase}"
                    observation_path = report_dir / (("stock-" if baseline else "") +
                                                     f"{phase_stem}-setup.json")
                    observation_path.write_text(json.dumps({"phase": phase, "setup_api_read_only": True,
                                                            "virtual_mcp": snapshot,
                                                            "container_id": container_identity(container, app_dir, env),
                                                            "app_dir": str(app_dir.resolve()),
                                                            "source_identities": source_identities}, indent=2) + "\n")
                    report_data, rc = run_firstboot_harness_phase(
                        ROOT, temp, report_dir, base_url, app_dir, make_source_args, env, baseline, phase, contract)
                    result = firstboot_phase_result(report_data, phase, rc, snapshot, contract)
                    classifications[phase] = validate_firstboot_phase(result, phase, mode, baseline, contract)
                    observations[phase] = result
                    result_path = report_dir / f"{phase_stem}-observation.json"
                    if not baseline:
                        result_path.write_text(json.dumps({**result, "classification": classifications[phase],
                                                           "source_identities": source_identities}, indent=2) + "\n")
                        print(f"PASS {contract['scope']} {phase}: {classifications[phase]}", flush=True)
                    else:
                        candidate_report_path = report_dir / f"{phase_stem}-observation.json"
                        candidate_setup_path = report_dir / f"{phase_stem}-setup.json"
                        if not candidate_report_path.is_file() or not candidate_setup_path.is_file():
                            raise RuntimeError(f"{contract['scope']} {phase}: candidate phase artifact is missing")
                        candidate_result = json.loads(candidate_report_path.read_text())
                        candidate_setup = json.loads(candidate_setup_path.read_text())
                        candidate_snapshot = candidate_setup["virtual_mcp"]
                        if candidate_result.get("source_identities") != source_identities \
                                or candidate_setup.get("source_identities") != source_identities:
                            raise RuntimeError(f"{contract['scope']} {phase}: candidate artifacts have different pinned source identities")
                        candidate_class = validate_firstboot_phase(candidate_result, phase, mode, False, contract)
                        candidate_result_state = {"http_status": candidate_result["http_status"],
                                                  "stored_virtual_key_ids": candidate_result["stored_virtual_key_ids"],
                                                  "assertions": candidate_result["assertions"],
                                                  "failed_assertions": candidate_result["failed_assertions"]}
                        stock_result_state = {"http_status": result["http_status"],
                                              "stored_virtual_key_ids": result["stored_virtual_key_ids"],
                                              "assertions": result["assertions"],
                                              "failed_assertions": result["failed_assertions"]}
                        expected_class = firstboot_comparison_classification(phase, candidate_class)
                        phase_comparison = {"phase": phase, "classification": expected_class,
                                            "stock": stock_result_state, "candidate": candidate_result_state,
                                            "stock_virtual_mcp_id": snapshot["virtual_mcp_id"],
                                            "candidate_virtual_mcp_id": candidate_snapshot["virtual_mcp_id"],
                                            "candidate_gate": candidate_class}
                        (report_dir / f"{phase_stem}-comparison.json").write_text(
                            json.dumps(phase_comparison, indent=2) + "\n")
                        print(f"PASS {contract['scope']} {phase} source comparison: {expected_class}", flush=True)
                (report_dir / (("stock-" if baseline else "") + f"{contract['scope']}-phases.json")).write_text(
                    json.dumps({"phases": observations, "classifications": classifications,
                                "same_container_across_restart": bool(container_ids) and
                                container_ids["before_restart"] == container_ids["after_restart"],
                                "container_ids": container_ids,
                                "source_identities": source_identities}, indent=2) + "\n")
                if baseline:
                    phases = {phase: json.loads((report_dir / f"{contract['scope']}-{phase}-comparison.json").read_text())
                              for phase in ("firstboot", "restart")}
                    strict_pass = (mode == "strict" and phases["firstboot"]["classification"] == "fixed"
                                   and phases["restart"]["classification"] == "green-control")
                    overall = {"scope": contract["scope"], "mode": mode, "phases": phases,
                               "strict_pass": strict_pass,
                               "classification_note": "Read-only setup API state and the exact one-request Newman response are classified per boot phase."}
                    (report_dir / f"{contract['scope']}-comparison.json").write_text(json.dumps(overall, indent=2) + "\n")
                    if mode == "strict" and not strict_pass:
                        raise RuntimeError(f"{contract['scope']} strict comparison did not fix first boot while preserving restart control")
                return 0
            setup_mode = setup_policy(scopes)
            attachments = setup_virtual_key_attachments(base_url, scopes)
            setup_name = "stock-fixture-setup.json" if baseline else "fixture-setup.json"
            (report_dir / setup_name).write_text(json.dumps({"selected_scopes": scopes,
                                                            "virtual_key_attachments": attachments,
                                                            "setup_policy": setup_mode,
                                                            "setup_api_postseeding": bool(attachments)}, indent=2) + "\n")
            if baseline or mode == "characterize":
                if "mcp-apps" in scopes:
                    ready_characterization(base_url)
                else:
                    ready_codemode_errors(base_url)
            else:
                ready(base_url, scopes)

            pass_fail = 0
            runs = [(feature, 60 if feature == "mcp-apps" else 4) for feature in scopes]
            for index, (feature, cap) in enumerate(runs, start=1):
                command = ["make", "run-provider-harness-test", "CI=1", "PARALLEL=0",
                           f"FEATURE={feature}", f"BASE_URL={base_url}", f"APP_DIR={app_dir}",
                           f"HARNESS_MAX_REQUESTS={cap}",
                           *([f"EXCLUDE_FEATURE_ANY={FIRSTBOOT_SCOPE}"] if feature == "mcp-apps" else []),
                           *make_source_args]
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
                    candidate_path = report_dir / f"{feature}-newman-report.json"
                    candidate_rc_path = report_dir / f"{feature}-exit-code.txt"
                    if not candidate_path.is_file() or not candidate_rc_path.is_file():
                        raise RuntimeError("source comparison requires complete candidate reports first")
                    candidate_rc = candidate_rc_path.read_text().strip()
                    candidate_data = json.loads(candidate_path.read_text())
                    validate_executions(candidate_data, feature, counts[feature], expected_names[feature])
                    comparison = compare_reports(report_data, candidate_data, feature, mode=mode)
                    comparison.update({"stock_image": image, "candidate_image": "from-prior-candidate-run",
                                       "stock_source": source_identities["stock"],
                                       "candidate_source": source_identities["candidate"],
                                       "stable_release_reference": source_identities["stable_reference"],
                                       "seeded_contract_uris": uri_seeds,
                                       "stock_exit_code": rc, "candidate_exit_code": int(candidate_rc),
                                       "classification_note": "Classification reflects observed named assertions; no stock-red result is assumed."})
                    scope_valid, unrelated_stock_failures = stock_failure_scope(
                        feature, comparison["stock_failed_assertions"])
                    candidate_scope_valid, unrelated_candidate_failures = stock_failure_scope(
                        feature, comparison["candidate_failed_assertions"])
                    comparison["stock_failure_scope_valid"] = scope_valid
                    comparison["unrelated_stock_failures"] = unrelated_stock_failures
                    comparison["candidate_failure_scope_valid"] = candidate_scope_valid
                    comparison["unrelated_candidate_failures"] = unrelated_candidate_failures
                    comparison["allowed_stock_failure_rationales"] = [
                        {"request": case, "assertion": assertion, "rationale": rationale}
                        for (case, assertion), rationale in BASELINE_ALLOWED_STOCK_FAILURES[feature].items()]
                    comparison["strict_pass"] = (comparison["strict_pass"] and candidate_rc == "0"
                                                  and not comparison["candidate_failed_assertions"]
                                                  and comparison["stock_failure_scope_valid"]
                                                  and comparison["candidate_failure_scope_valid"]
                                                  and (rc == 0 or bool(comparison["stock_failed_assertions"])))
                    (report_dir / f"{feature}-comparison.json").write_text(json.dumps(comparison, indent=2) + "\n")
                    print(f"PASS {feature} source comparison ({mode}; unresolved={len(comparison['unresolved'])}, regressions={len(comparison['regressions'])})", flush=True)
                    if (not scope_valid or not candidate_scope_valid
                            or (rc != 0 and not comparison["stock_failed_assertions"])
                            or (int(candidate_rc) != 0 and not comparison["candidate_failed_assertions"])):
                        print(f"FAIL {feature} run had an unrelated assertion or an unclassified nonzero exit", flush=True)
                        pass_fail = 1
                    elif mode == "strict" and not comparison["strict_pass"]:
                        print(f"FAIL {feature} strict comparison; see {feature}-comparison.json", flush=True)
                        pass_fail = 1
                elif rc:
                    print(f"FAIL {feature} harness (exit {rc}); redacted reports: {report_dir}", flush=True)
                    pass_fail = rc
                else:
                    print(f"PASS {feature} harness; redacted reports: {report_dir}", flush=True)
                if failures_ok:
                    print(f"Saved redacted {feature} failure report", flush=True)
            run_reference_host(report_dir, base_url, source_identities, baseline, scopes)
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
        if len(sys.argv) == 4 and sys.argv[1] == "--baseline":
            report_arg = sys.argv[3]
        elif len(sys.argv) == 3:
            report_arg = sys.argv[2]
        else:
            report_arg = None
        if report_arg:
            try:
                write_failure_report(Path(report_arg).resolve(), error, failure_report_filename(sys.argv[1:]))
            except OSError:
                print("fixture CI could not write the redacted failure report", file=sys.stderr)
        print(f"fixture CI failed: {error}", file=sys.stderr)
        raise SystemExit(1)
PY
