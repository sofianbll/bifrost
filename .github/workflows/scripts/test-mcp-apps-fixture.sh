#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
export BIFROST_REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd -P)"
exec python3 - "$@" <<'PY'
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
                    if entry["endpoint_slug"] == "mcp-apps-virtual"]
    keys = [entry for entry in profile["governance"]["virtual_keys"] if entry["value"] in KEYS.values()]
    if {entry["value"] for entry in keys} != set(KEYS.values()) or len(virtual_mcps) != 1:
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
    assert config["mcp"]["virtual_mcps"][0]["virtual_key_ids"] == ["vk-mcp-apps-full", "vk-mcp-apps-limited"]
    assert config["mcp"]["virtual_mcps"][0]["tools"] == [{"mcp_client_name": "mcp_apps_fixture", "tool_names": ["*"]}]
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
    safe = redact({"Authorization": "Bearer sk-bf-example", "body": "token sk-bf-example"})
    assert safe == {"Authorization": "<redacted>", "body": "token <redacted>"}
    with tempfile.TemporaryDirectory() as temp:
        write_failure_report(Path(temp), RuntimeError("Authorization: Bearer sk-bf-example"))
        failure = json.loads((Path(temp) / "fixture-error.json").read_text())
        assert failure["error_type"] == "RuntimeError"
        assert "sk-bf-example" not in failure["message"] and "<redacted>" in failure["message"]
    print("PASS fixture CI self-check (config scope, route guard, report and early-failure redaction)")


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


def ready(url):
    deadline = time.monotonic() + 180
    while time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(url + "/health", timeout=2) as response:
                if response.status == 200:
                    break
        except (OSError, urllib.error.URLError):
            time.sleep(1)
    else:
        raise RuntimeError("candidate /health did not become ready within 180 seconds")

    routes = (("/mcp/mcp-apps-fixture", "full", False),
              ("/mcp/mcp-apps-virtual", "full", False),
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
        if status != 200 or not tools or (needs_code and "executeToolCode" not in {t.get("name") for t in tools}):
            raise RuntimeError(f"MCP tools/list readiness failed on {path} (HTTP {status}, tools={len(tools)})")
    print("PASS candidate health and MCP readiness on four fixture routes", flush=True)


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
    if len(args) != 2:
        print("usage: test-mcp-apps-fixture.sh IMAGE REPORT_DIR | --self-check", file=sys.stderr)
        return 2
    image, report_dir_arg = args
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
        augmented = temp / "augmented.json"
        subprocess.run(["node", str(augment), "--source", str(source), "--out", str(augmented)],
                       cwd=ROOT, env=env, check=True)
        counts = {}
        for feature, expected in (("mcp-apps", 37), ("codemode-errors", 2)):
            filtered = temp / f"filtered-{feature}.json"
            subprocess.run(["node", str(filter_script), "--source", str(augmented), "--out", str(filtered),
                            "--feature", feature], cwd=ROOT, env=env, check=True)
            count = len(list(requests_in(json.loads(filtered.read_text()).get("item", []))))
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
            if subprocess.run(["docker", "image", "inspect", image], stdout=subprocess.DEVNULL,
                              stderr=subprocess.DEVNULL).returncode:
                raise RuntimeError(f"candidate image not present in Docker: {image}")
            subprocess.run(["docker", "run", "-d", "--user", f"{os.getuid()}:{os.getgid()}", "--name", container, "-p", "127.0.0.1::8080",
                            "-e", "APP_DIR=/fixture-data", "-e", "LOG_LEVEL=warn",
                            "-v", f"{ROOT}:/repo:ro", "-v", f"{app_dir}:/fixture-data",
                            "-w", "/repo/transports/bifrost-http", image],
                           cwd=ROOT, env=env, check=True, stdout=subprocess.DEVNULL)
            container_started = True
            port_line = subprocess.check_output(["docker", "port", container, "8080/tcp"], text=True).splitlines()[0]
            port = int(port_line.rsplit(":", 1)[1])
            base_url = f"http://127.0.0.1:{port}"
            ready(base_url)

            pass_fail = 0
            runs = [("mcp-apps", 60)]
            if "codemode-errors" in counts:
                runs.append(("codemode-errors", 4))
            for index, (feature, cap) in enumerate(runs, start=1):
                command = ["make", "run-provider-harness-test", "CI=1", "PARALLEL=0",
                           f"FEATURE={feature}", f"BASE_URL={base_url}", f"APP_DIR={app_dir}",
                           f"HARNESS_MAX_REQUESTS={cap}"]
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
                archive_report(log, report_dir / f"{feature}-runner.log")
                archive_report(ROOT / "tmp/harness-quiet.log", report_dir / f"{feature}-harness.log")
                report_ok = archive_report(raw_report, report_dir / f"{feature}-newman-report.json")
                failures_ok = archive_report(raw_failures, report_dir / f"{feature}-harness-failures.md")
                (report_dir / f"{feature}-exit-code.txt").write_text(str(rc) + "\n")
                if not report_ok:
                    raise RuntimeError(f"{feature}: Make target produced no Newman JSON report")
                if rc:
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
                archive_report(gateway_log, report_dir / "fixture-gateway.log")
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
