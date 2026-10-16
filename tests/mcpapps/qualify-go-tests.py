#!/usr/bin/env python3
"""Run named stable MCP checks with JSON inventory and paired inherited-failure review."""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys

PINNED_SOURCES = {
    "stock": "9237ec06672406e437b3cb33fc793ffd0403b3ae",
    "candidate": "b38fadec8d78ec64ffbefb53f650e6ef149d948e",
}
EXPECTED_BROAD_PACKAGES = {
    "github.com/maximhq/bifrost/core/mcp",
    "github.com/maximhq/bifrost/core/mcp/codemode/starlark",
    "github.com/maximhq/bifrost/core/mcp/credstore",
    "github.com/maximhq/bifrost/core/mcp/utils",
    "github.com/maximhq/bifrost/core/schemas",
}

SUITES = {
    "broad-mcp-schemas": {
        "cwd": "core",
        "packages": ["./mcp/...", "./schemas/..."],
        "pattern": ".*",
        "expected": [],
        "broad": True,
    },
    "core-mcp-contract": {
        "cwd": "core",
        "packages": ["./mcp"],
        "expected": [
            "TestConvertMCPToolToBifrostSchema_PreservesMCPAppDefinition",
            "TestConvertMCPToolToBifrostSchemaBoundsRetainedMCPAppMetadata",
            "TestReadMCPAppResourceUsesMCPHooksAndHidesDenials",
            "TestExecuteMCPAppToolPreservesNativeFieldsAndDropsStaleHookOutput",
            "TestGetAvailableToolsKeepsAppOnlyToolsOutOfModelCatalog",
            "TestGetAvailableToolsForMCPIncludesAppsFromCodeModeClients",
        ],
    },
    "codemode-starlark": {
        "cwd": "core",
        "packages": ["./mcp/codemode/starlark"],
        "expected": [
            "TestExecuteToolCodeCanUseProtocolMarkedRemoteErrorsAsValues",
        ],
    },
    "codemode-existing-markers": {
        "cwd": "core",
        "packages": ["./mcp/codemode/starlark"],
        "expected": [
            "TestHandleExecuteToolCodeMarksSandboxErrors",
            "TestCreateToolResponseMessageMarksError",
        ],
    },
    "codemode-guardrail": {
        "cwd": "core",
        "packages": ["./internal/mcptests"],
        "expected": ["TestExecuteToolCode_GuardrailedResultNeverReachesModel"],
    },
    "configstore-app-cache": {
        "cwd": "framework",
        "packages": ["./configstore"],
        "expected": ["TestMCPAppToolMetadataSurvivesRefreshAndManagerReload"],
    },
    "configstore-mcp-tables": {
        "cwd": "framework",
        "packages": ["./configstore/tables"],
        "expected": [
            "TestTableMCPClient_EncryptDecrypt",
            "TestTableMCPClient_SecretVarConnectionString_NotEncrypted",
            "TestTableMCPClient_DirectConnStr_EmptyHeaders",
            "TestTableMCPClient_HeadersOnly_NoConnStr",
            "TestTableMCPClient_UpdatePreservesDecryption",
            "TestTableMCPClient_EncryptionDisabled_StoresPlaintext",
        ],
    },
    "http-handlers": {
        "cwd": "transports",
        "packages": ["./bifrost-http/handlers"],
        "expected": [
            "TestSyncMCPServerSerialCatalogChanges",
            "TestSyncMCPServerOverlappingRefreshKeepsLatestCatalog",
            "TestInitializeNegotiatesMCPAppsExtensionOnlyWhenRequested",
            "TestMCPAppsBareCallbackSelectsUniqueAvailableSource",
            "TestMCPAppsBareCallbackIsAmbiguousWithoutDisclosingSources",
            "TestMCPAppsAmbiguousCallbackWithNullIDReturnsJSONRPCError",
            "TestMCPAppsBareCallbackUsesOnlyAdmittedSourceAndCatalogStaysQualified",
            "TestMCPAppsBareCallbackCannotBypassDeniedGatewayRequest",
            "TestCallToolRefusesHiddenToolWithoutDisclosingItsName",
            "TestCallToolPreservesNativeMCPAppResultAndUsesEffectiveFallback",
            "TestMCPAppsMixedCatalogKeepsNativeResultScopedToAppTools",
            "TestToolsListPreservesAndQualifiesMCPAppMetadata",
            "TestMCPAppsPreservesSupportedOutputSchema",
            "TestResourcesListAndReadAreSourceQualified",
            "TestCallToolPreservesCodeModeErrorMarker",
        ],
    },
    "http-server": {
        "cwd": "transports",
        "packages": ["./bifrost-http/server"],
        "expected": ["TestBifrostHTTPServerImplementsMCPAppsRelaySeams"],
    },
    "config-authority-lifecycle": {
        "cwd": "transports",
        "packages": ["./bifrost-http/lib"],
        "expected": ["TestLoadConfig_FirstBootVirtualMCPAttachesFileDeclaredVirtualKey"],
    },
    "common-config-authority": {
        "cwd": "transports",
        "packages": ["./bifrost-http/lib"],
        "expected": [
            "TestLoadConfig_FullConfigFile_FreshDB",
            "TestReconcileVirtualMCPsConfig",
            "TestReconcileVirtualMCPsConfig_DerivesSlugFromName",
            "TestReconcileVirtualMCPsConfig_DedupeNameAndID",
            "TestLoadConfig_InferenceAuthSetupFailures",
            "TestLoadConfig_GovernanceAuthConfig_PersistsFromFirstImport",
            "TestSourceOfTruthConfigJSON_MCPMissingLeavesDBUntouched",
            "TestSourceOfTruthConfigJSON_MCPPresentEmptyPrunesDB",
        ],
    },
    "common-http-auth": {
        "cwd": "transports",
        "packages": ["./bifrost-http/handlers"],
        "expected": ["TestAuthenticate_JWTPath", "TestAuthenticate_HeaderAndAnonPath"],
    },
    "common-flat-cache": {
        "cwd": "core",
        "packages": ["./mcp"],
        "expected": [
            "TestComputeToolsHash_StableAcrossMapIterationOrder",
            "TestSetClientTools_UnchangedTools_DoesNotFireCallback",
            "TestAddClient_RestoreFromConfig_SeedsHashSoSubsequentIdenticalDiscoveryNoOps",
        ],
    },
    "governance-reset": {
        "cwd": "plugins/governance",
        "packages": ["."],
        "expected": ["TestGovernancePlugin_StartResetWorkers"],
    },
    "mcp-app-fixture": {
        "cwd": "examples/mcps/mcp-apps-test-server",
        "packages": ["."],
        "expected": [
            "TestStdioToolListMetadataVariants",
            "TestStdioOpenViewValidatesSuppliedMessageType",
            "TestStdioStateCallbacksPersistWithinOneFixtureProcess",
            "TestStdioResourceReturnsExecutablePinnedHostFixture",
        ],
        "gowork_off": True,
    },
    "codemode-error-fixture": {
        "cwd": "examples/mcps/error-test-server",
        "packages": ["."],
        "expected": ["TestStdioReturnErrorPrefixPayload"],
        "gowork_off": True,
    },
    "global-proxy-isolated": {
        "cwd": "core",
        "packages": ["./mcp"],
        "expected": ["TestBuildTLSHTTPClientUsesGlobalProxy"],
    },
}

STOCK_SUITES = {
    "broad-mcp-schemas",
    "common-flat-cache",
    "common-config-authority",
    "common-http-auth",
    "configstore-mcp-tables",
    "codemode-existing-markers",
    "codemode-guardrail",
    "governance-reset",
    "global-proxy-isolated",
}
CANDIDATE_ONLY_SUITES = set(SUITES) - STOCK_SUITES
NO_TEST_FILE_PACKAGES = {
    "github.com/maximhq/bifrost/core/mcp/utils": "stable source package has no *_test.go files",
}

EXCLUDED_TESTS = [{
    "test_id": "TestSourceOfTruthConfigJSON_InvalidOpenAPIDeclarationKeepsStoredClient",
    "reason": "This test exists only in later development history and is absent from the pinned stable 2.3 source; it is intentionally not imported into this lane.",
}]


def parse_json_events(raw: str):
    events = []
    malformed = []
    for number, line in enumerate(raw.splitlines(), 1):
        if not line.strip():
            continue
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            malformed.append({"line": number, "text": line[:500]})
            continue
        if not isinstance(event, dict) or not isinstance(event.get("Action"), str):
            malformed.append({"line": number, "text": line[:500]})
            continue
        events.append(event)
    return events, malformed


def summarize(events, malformed, expected):
    run_ids = set()
    terminal = {}
    package_runs = set()
    package_terminal = {}
    output_by_test = {}
    output_by_package = {}
    for event in events:
        action = event.get("Action")
        package = event.get("Package", "")
        test_id = event.get("Test")
        if package:
            package_runs.add(package)
        if action == "output":
            output = event.get("Output", "").rstrip()
            if test_id:
                output_by_test.setdefault((package, test_id), []).append(output)
            elif package:
                output_by_package.setdefault(package, []).append(output)
        if action in ("pass", "fail", "skip") and test_id:
            terminal.setdefault((package, test_id), action)
        if action == "run" and test_id:
            run_ids.add((package, test_id))
        if action in ("pass", "fail", "skip") and not test_id and package:
            package_terminal[package] = action

    run_test_names = sorted({test for _, test in run_ids})
    run_package_names = sorted({package for package, _ in run_ids})
    terminal_tests = sorted({test for _, test in terminal})
    passed_tests = sorted({test for (_, test), action in terminal.items() if action == "pass"})
    failed_tests = sorted({test for (_, test), action in terminal.items() if action == "fail"})
    skipped_tests = sorted({test for (_, test), action in terminal.items() if action == "skip"})
    expected_set = set(expected)
    missing = sorted(expected_set - set(run_test_names))
    wrong_terminal = sorted(name for name in expected_set if not any(
        test == name and action == "pass" for (_, test), action in terminal.items()))
    required_skips = sorted(expected_set & set(skipped_tests))
    test_ids_without_terminal = sorted(f"{package}::{test}" for package, test in run_ids
                                       if (package, test) not in terminal)
    failures = [{"package": package, "test": test} for (package, test), action in sorted(terminal.items())
                if action == "fail"]
    skips = [{"package": package, "test": test,
              "reason": "\n".join(line for line in output_by_test.get((package, test), [])
                                    if line and "=== RUN" not in line and "--- SKIP:" not in line).strip()
                        or "Go test emitted skip without a reason"}
             for (package, test), action in sorted(terminal.items()) if action == "skip"]
    package_failures = sorted(package for package, action in package_terminal.items() if action == "fail")
    package_skips = sorted(package for package, action in package_terminal.items() if action == "skip")
    package_terminal_missing = sorted(package for package in package_runs if package not in package_terminal)
    package_skip_inventory = []
    for package in package_skips:
        reason = "\n".join(output_by_package.get(package, [])).strip()
        no_test_files = "[no test files]" in reason
        declared = package in NO_TEST_FILE_PACKAGES
        package_skip_inventory.append({
            "package": package,
            "reason": reason or "Go package emitted skip without a reason",
            "classification": "declared-no-test-files" if no_test_files and declared else "unexpected",
        })
    unexpected_package_skips = [item for item in package_skip_inventory
                                if item["classification"] != "declared-no-test-files"]
    return {
        "json_event_count": len(events),
        "malformed_json_lines": malformed,
        "test_run_event_count": len(run_ids),
        "test_run_packages": run_package_names,
        "terminal_test_event_count": len(terminal),
        "test_ids_run": run_test_names,
        "test_ids_terminal": terminal_tests,
        "test_ids_passed": passed_tests,
        "test_ids_failed": failed_tests,
        "test_ids_skipped": skipped_tests,
        "test_ids_without_terminal_event": test_ids_without_terminal,
        "package_terminal_events": package_terminal,
        "package_run_event_count": len(package_runs),
        "package_terminal_missing": package_terminal_missing,
        "package_failures": package_failures,
        "package_skips": package_skips,
        "package_skip_inventory": package_skip_inventory,
        "unexpected_package_skips": unexpected_package_skips,
        "required_test_ids": sorted(expected_set),
        "missing_required_test_ids": missing,
        "required_test_ids_not_passed": wrong_terminal,
        "required_test_ids_skipped": required_skips,
        "failures": failures,
        "skips": skips,
    }


def run_suite(source, root, report_dir, suite_name):
    suite = SUITES[suite_name]
    if suite.get("broad"):
        pattern = suite["pattern"]
    else:
        pattern = "^(" + "|".join(re.escape(name) for name in suite["expected"]) + ")$"
    args = ["go", "test", "-race", "-json", "-count=1", "-run", pattern, *suite["packages"]]
    cwd = root / suite["cwd"]
    env = os.environ.copy()
    env["GOWORK"] = "off" if suite.get("gowork_off") else os.environ.get("BIFROST_GO_WORKSPACE", "off")
    expected_packages = []
    package_inventory_exit_code = 0
    if suite.get("broad"):
        inventory = subprocess.run(["go", "list", *suite["packages"]], cwd=cwd, env=env,
                                   text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        package_inventory_exit_code = inventory.returncode
        expected_packages = sorted(line.strip() for line in inventory.stdout.splitlines() if line.strip())
        report_dir.mkdir(parents=True, exist_ok=True)
        (report_dir / f"{source}-{suite_name}-packages.txt").write_text(inventory.stdout)
        (report_dir / f"{source}-{suite_name}-packages.stderr.log").write_text(inventory.stderr)
    process = subprocess.run(args, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE,
                             stderr=subprocess.PIPE, check=False)
    report_dir.mkdir(parents=True, exist_ok=True)
    stdout_path = report_dir / f"{source}-{suite_name}.jsonl"
    stderr_path = report_dir / f"{source}-{suite_name}.stderr.log"
    stdout_path.write_text(process.stdout)
    stderr_path.write_text(process.stderr)
    events, malformed = parse_json_events(process.stdout)
    summary = summarize(events, malformed, suite["expected"])
    summary.update({
        "source_role": source,
        "source_head": subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"], text=True).strip(),
        "suite": suite_name,
        "cwd": suite["cwd"],
        "command": args,
        "exit_code": process.returncode,
        "expected_package_inventory": expected_packages,
        "expected_package_inventory_matches_pinned_stable": (not suite.get("broad") or set(expected_packages) == EXPECTED_BROAD_PACKAGES),
        "package_inventory_exit_code": package_inventory_exit_code,
        "observed_package_inventory": sorted(summary["package_terminal_events"]),
        "stderr_log": stderr_path.name,
        "jsonl_log": stdout_path.name,
        "passed": process.returncode == 0 and not malformed and not summary["missing_required_test_ids"]
                  and not summary["required_test_ids_not_passed"]
                  and not summary["required_test_ids_skipped"]
                  and not summary["test_ids_without_terminal_event"]
                  and not summary["failures"]
                  and not summary["package_failures"]
                  and not summary["unexpected_package_skips"]
                  and not summary["package_terminal_missing"]
                  and not summary["skips"]
                  and (bool(summary["test_run_event_count"] and summary["terminal_test_event_count"])
                       if suite.get("broad") else True),
    })
    (report_dir / f"{source}-{suite_name}-summary.json").write_text(json.dumps(summary, indent=2) + "\n")
    return summary


def source_summaries(report_dir, source):
    result = {}
    for path in report_dir.glob(f"{source}-*-summary.json"):
        name = path.name[len(source) + 1:-len("-summary.json")]
        result[name] = json.loads(path.read_text())
    return result


def suite_names_for(source):
    if source == "stock":
        return STOCK_SUITES
    if source == "candidate":
        return set(SUITES)
    raise ValueError(f"unknown source role {source!r}")


def classify_broad(stock, candidate):
    known_failure = {"package": "github.com/maximhq/bifrost/core/mcp",
                     "test": "TestBuildTLSHTTPClientUsesGlobalProxy"}

    def full_inventory(result):
        package_skip_names = sorted(item["package"] for item in result["package_skip_inventory"])
        return (result["json_event_count"] > 0
                and result["package_inventory_exit_code"] == 0
                and result["expected_package_inventory_matches_pinned_stable"]
                and set(result["expected_package_inventory"]) == EXPECTED_BROAD_PACKAGES
                and result["observed_package_inventory"] == result["expected_package_inventory"]
                and result["test_run_event_count"] > 0
                and set(result["test_run_packages"]) == EXPECTED_BROAD_PACKAGES - set(NO_TEST_FILE_PACKAGES)
                and result["terminal_test_event_count"] > 0
                and result["package_run_event_count"] > 0
                and result["package_terminal_missing"] == []
                and result["test_ids_without_terminal_event"] == []
                and result["malformed_json_lines"] == []
                and package_skip_names == sorted(NO_TEST_FILE_PACKAGES)
                and all(item["classification"] == "declared-no-test-files"
                        for item in result["package_skip_inventory"])
                and result["unexpected_package_skips"] == [])

    if not full_inventory(stock) or not full_inventory(candidate):
        raise RuntimeError("broad MCP/schemas JSON inventory is empty, malformed, incomplete, or has an undeclared package skip")

    stock_failures = stock["failures"]
    candidate_failures = candidate["failures"]
    inherited = (
        stock_failures == [known_failure]
        and candidate_failures == [known_failure]
        and stock["exit_code"] == candidate["exit_code"] == 1
        and stock["package_failures"] == candidate["package_failures"] == [known_failure["package"]]
        and stock["skips"] == candidate["skips"] == []
        and stock["unexpected_package_skips"] == candidate["unexpected_package_skips"] == []
        and stock["package_terminal_missing"] == candidate["package_terminal_missing"] == []
        and stock["observed_package_inventory"] == candidate["observed_package_inventory"]
        and stock["test_ids_failed"] == candidate["test_ids_failed"] == [known_failure["test"]]
    )
    passed = (
        stock["exit_code"] == candidate["exit_code"] == 0
        and stock_failures == candidate_failures == []
        and stock["package_failures"] == candidate["package_failures"] == []
        and stock["skips"] == candidate["skips"] == []
        and stock["package_terminal_missing"] == candidate["package_terminal_missing"] == []
        and stock["observed_package_inventory"] == candidate["observed_package_inventory"]
    )
    if inherited:
        return "inherited-failure-reproduced-on-stock-and-candidate; isolated-controls-pass"
    if passed:
        return "passed-on-stock-and-candidate"
    raise RuntimeError(f"broad MCP/schemas results differ from the reviewed inherited failure: "
                       f"stock(rc={stock['exit_code']}, failures={stock_failures}), "
                       f"candidate(rc={candidate['exit_code']}, failures={candidate_failures})")


def compare_sources(report_dir):
    stock = source_summaries(report_dir, "stock")
    candidate = source_summaries(report_dir, "candidate")
    if set(stock) != STOCK_SUITES or set(candidate) != set(SUITES):
        raise RuntimeError(f"role-specific Go suite summary inventory is incomplete: "
                           f"stock missing={sorted(STOCK_SUITES - set(stock))}, "
                           f"stock extra={sorted(set(stock) - STOCK_SUITES)}, "
                           f"candidate missing={sorted(set(SUITES) - set(candidate))}, "
                           f"candidate extra={sorted(set(candidate) - set(SUITES))}")
    for role, summaries in (("stock", stock), ("candidate", candidate)):
        for name, result in summaries.items():
            if result.get("source_head") != PINNED_SOURCES[role]:
                raise RuntimeError(f"{role}/{name}: Go result is bound to {result.get('source_head')}, expected {PINNED_SOURCES[role]}")
            if result.get("malformed_json_lines"):
                raise RuntimeError(f"{role}/{name}: malformed go test JSON output")
            if name not in ("broad-mcp-schemas", "global-proxy-isolated") and not result.get("passed"):
                raise RuntimeError(f"{role}/{name}: required semantic test selection did not pass")
            if name == "global-proxy-isolated" and not result.get("passed"):
                raise RuntimeError(f"{role}: isolated global proxy control did not pass")

    stock_broad = stock["broad-mcp-schemas"]
    candidate_broad = candidate["broad-mcp-schemas"]
    broad_status = classify_broad(stock_broad, candidate_broad)
    stock_failures = stock_broad["failures"]

    verdict = {
        "status": "semantic-selections-passed-with-inherited-broad-failure" if stock_failures else "passed",
        "broad_suite_status": broad_status,
        "intentionally_excluded_later_development_tests": EXCLUDED_TESTS,
        "source_inventories": {
            "stock": {"executed": sorted(STOCK_SUITES), "candidate_only": [
                {"suite": name, "status": "not-applicable-not-run",
                 "reason": "candidate-only selections contain IDs introduced on candidate; common IDs run in shared suites."}
                for name in sorted(CANDIDATE_ONLY_SUITES)]},
            "candidate": {"executed": sorted(SUITES), "candidate_only": []},
        },
        "stock": {name: {"exit_code": value["exit_code"], "passed": value["passed"],
                         "test_run_event_count": value["test_run_event_count"],
                         "terminal_test_event_count": value["terminal_test_event_count"],
                         "expected_package_inventory": value.get("expected_package_inventory", []),
                         "observed_package_inventory": value.get("observed_package_inventory", []),
                         "test_ids_passed": value["test_ids_passed"], "failures": value["failures"],
                         "skips": value["skips"], "package_skip_inventory": value["package_skip_inventory"]}
                  for name, value in stock.items()},
        "candidate": {name: {"exit_code": value["exit_code"], "passed": value["passed"],
                             "test_run_event_count": value["test_run_event_count"],
                             "terminal_test_event_count": value["terminal_test_event_count"],
                             "expected_package_inventory": value.get("expected_package_inventory", []),
                             "observed_package_inventory": value.get("observed_package_inventory", []),
                             "test_ids_passed": value["test_ids_passed"], "failures": value["failures"],
                             "skips": value["skips"], "package_skip_inventory": value["package_skip_inventory"]}
                      for name, value in candidate.items()},
        "inherited_checks_are_not_global_green": bool(stock_failures),
    }
    (report_dir / "go-test-comparison.json").write_text(json.dumps(verdict, indent=2) + "\n")
    return verdict


def run_all(source, root, report_dir):
    summaries = []
    for suite_name in sorted(suite_names_for(source)):
        summaries.append(run_suite(source, root, report_dir, suite_name))
    failed = [result["suite"] for result in summaries if not result["passed"]]
    print(json.dumps({"source": source, "suites": len(summaries), "failed_or_inherited": failed}, sort_keys=True))
    return 0 if not failed else 1


def self_check():
    assert "configstore-mcp-tables" in STOCK_SUITES
    assert SUITES["config-authority-lifecycle"]["expected"] == [
        "TestLoadConfig_FirstBootVirtualMCPAttachesFileDeclaredVirtualKey"]
    assert {
        "TestSourceOfTruthConfigJSON_MCPMissingLeavesDBUntouched",
        "TestSourceOfTruthConfigJSON_MCPPresentEmptyPrunesDB",
    } <= set(SUITES["common-config-authority"]["expected"])
    assert not (STOCK_SUITES & CANDIDATE_ONLY_SUITES)
    assert "core-mcp-contract" in CANDIDATE_ONLY_SUITES

    sample = [
        # Go writes the skip reason before the terminal skip event.
        {"Action": "run", "Package": "example/pkg", "Test": "TestOne"},
        {"Action": "output", "Package": "example/pkg", "Test": "TestOne", "Output": "=== RUN TestOne\n"},
        {"Action": "pass", "Package": "example/pkg", "Test": "TestOne"},
        {"Action": "output", "Package": "example/pkg", "Test": "TestOptional", "Output": "    optional_test.go:12: missing optional tool\n"},
        {"Action": "skip", "Package": "example/pkg", "Test": "TestOptional"},
        {"Action": "pass", "Package": "example/pkg"},
    ]
    summary = summarize(sample, [], ["TestOne"])
    assert summary["test_ids_passed"] == ["TestOne"]
    assert summary["skips"][0]["reason"] == "optional_test.go:12: missing optional tool"
    assert summary["package_run_event_count"] == 1
    assert summary["package_terminal_missing"] == []
    missing = summarize(sample[3:], [], ["TestOne"])
    assert missing["missing_required_test_ids"] == ["TestOne"]
    assert missing["required_test_ids_not_passed"] == ["TestOne"]
    malformed_events, malformed = parse_json_events('{"Action":"run"}\nnot-json\n')
    assert len(malformed_events) == 1 and len(malformed) == 1
    empty, empty_malformed = parse_json_events("")
    empty_summary = summarize(empty, empty_malformed, [])
    assert empty_summary["test_run_event_count"] == 0
    assert empty_summary["terminal_test_event_count"] == 0

    def broad_result(*, exit_code=0, events=None, malformed=None, failures=None, package_failures=None,
                     skips=None, unexpected_package_skips=None, package_terminal_missing=None):
        if events is None:
            events = []
            for package in sorted(EXPECTED_BROAD_PACKAGES):
                if package.endswith("/utils"):
                    events.extend((
                        {"Action": "output", "Package": package, "Output": "? github.com/maximhq/bifrost/core/mcp/utils [no test files]\n"},
                        {"Action": "skip", "Package": package},
                    ))
                else:
                    events.extend((
                        {"Action": "run", "Package": package, "Test": "TestStable"},
                        {"Action": "pass", "Package": package, "Test": "TestStable"},
                        {"Action": "pass", "Package": package},
                    ))
        result = summarize(events, malformed or [], [])
        result.update({"exit_code": exit_code,
                       "expected_package_inventory": sorted(EXPECTED_BROAD_PACKAGES),
                       "expected_package_inventory_matches_pinned_stable": True,
                       "package_inventory_exit_code": 0,
                       "observed_package_inventory": sorted(result["package_terminal_events"]),
                       "failures": failures if failures is not None else result["failures"],
                       "package_failures": package_failures if package_failures is not None else result["package_failures"],
                       "skips": skips if skips is not None else result["skips"],
                       "unexpected_package_skips": unexpected_package_skips if unexpected_package_skips is not None else result["unexpected_package_skips"],
                       "package_terminal_missing": package_terminal_missing if package_terminal_missing is not None else result["package_terminal_missing"]})
        return result

    good = broad_result()
    assert good["observed_package_inventory"] == sorted(EXPECTED_BROAD_PACKAGES)
    assert good["package_skip_inventory"] == [{
        "package": "github.com/maximhq/bifrost/core/mcp/utils",
        "reason": "? github.com/maximhq/bifrost/core/mcp/utils [no test files]",
        "classification": "declared-no-test-files",
    }]
    assert classify_broad(good, good) == "passed-on-stock-and-candidate"
    for bad in (
        broad_result(exit_code=2, events=[]),  # build/argument failure with no test JSON
        broad_result(events=[{"Action": "run", "Package": "pkg", "Test": "TestTruncated"}]),
        broad_result(malformed=[{"line": 1, "text": "{\\\"Action\\\":\\\"run\\\""}]),
        broad_result(skips=[{"package": "pkg", "test": "TestRequired", "reason": "guard off"}]),
        broad_result(unexpected_package_skips=[{"package": "pkg", "reason": "unexpected"}]),
    ):
        try:
            classify_broad(bad, bad)
        except RuntimeError:
            pass
        else:
            raise AssertionError("invalid broad test inventory was incorrectly accepted")

    proxy = {"package": "github.com/maximhq/bifrost/core/mcp", "test": "TestBuildTLSHTTPClientUsesGlobalProxy"}
    inherited_events = []
    for package in sorted(EXPECTED_BROAD_PACKAGES):
        if package.endswith("/utils"):
            inherited_events.extend((
                {"Action": "output", "Package": package, "Output": "? github.com/maximhq/bifrost/core/mcp/utils [no test files]\n"},
                {"Action": "skip", "Package": package},
            ))
        elif package == proxy["package"]:
            inherited_events.extend((
                {"Action": "run", "Package": package, "Test": proxy["test"]},
                {"Action": "fail", "Package": package, "Test": proxy["test"]},
                {"Action": "fail", "Package": package},
            ))
        else:
            inherited_events.extend((
                {"Action": "run", "Package": package, "Test": "TestStable"},
                {"Action": "pass", "Package": package, "Test": "TestStable"},
                {"Action": "pass", "Package": package},
            ))
    inherited = broad_result(exit_code=1, events=inherited_events, failures=[proxy],
                             package_failures=[proxy["package"]])
    inherited["expected_package_inventory"] = sorted(EXPECTED_BROAD_PACKAGES)
    inherited["observed_package_inventory"] = sorted(inherited["package_terminal_events"])
    assert classify_broad(inherited, inherited).startswith("inherited-failure-reproduced")
    print("PASS Go JSON inventory self-check: ordered skip reason and empty, truncated, failed-build, skipped-required negative controls")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--self-check", action="store_true")
    parser.add_argument("--run", choices=SUITES)
    parser.add_argument("--run-all", action="store_true")
    parser.add_argument("--source", choices=("stock", "candidate"))
    parser.add_argument("--root", type=Path)
    parser.add_argument("--report-dir", type=Path)
    parser.add_argument("--compare", action="store_true")
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return 0
    if args.compare:
        compare_sources(args.report_dir)
        return 0
    if args.run_all:
        if not (args.source and args.root and args.report_dir):
            parser.error("--run-all requires --source, --root and --report-dir")
        return run_all(args.source, args.root.resolve(), args.report_dir.resolve())
    if not (args.run and args.source and args.root and args.report_dir):
        parser.error("--run requires --source, --root and --report-dir")
    summary = run_suite(args.source, args.root.resolve(), args.report_dir.resolve(), args.run)
    print(json.dumps({"suite": args.run, "source": args.source, "passed": summary["passed"],
                      "run_events": summary["test_run_event_count"], "failures": summary["failures"],
                      "skips": summary["skips"]}, sort_keys=True))
    return 0 if summary["passed"] else 1


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, subprocess.CalledProcessError, RuntimeError, ValueError) as error:
        print(f"Go qualification failed: {error}", file=sys.stderr)
        raise SystemExit(1)
