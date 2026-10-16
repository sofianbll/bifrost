#!/usr/bin/env python3
"""Diagnostic-only observer of frozen driver readiness; never runs qualification."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time

STOCK = '9237ec06672406e437b3cb33fc793ffd0403b3ae'
CANDIDATE = 'e33b67ab6fd26e1e1b83fbc0a84c0b2f946fdb05'
DRIVER = 'b838f2e5c3d3cf2340b7fee6c0cfcf2ec1ad0efb'


class DiagnosticFinished(Exception):
    pass


def source_identity(root, expected):
    head = subprocess.check_output(['git', '-C', str(root), 'rev-parse', 'HEAD'], text=True).strip()
    status = subprocess.check_output(['git', '-C', str(root), 'status', '--porcelain'], text=True).strip()
    if head != expected or status or (root / '.env').exists():
        raise RuntimeError('diagnostic requires exact clean sources without .env')
    return {'source': head, 'status': status}


def load_driver(root):
    wrapper = root / '.github/workflows/scripts/test-mcp-apps-fixture.sh'
    source = wrapper.read_text().split("<<'PY'\n", 1)[1].rsplit('\nPY', 1)[0]
    namespace = {'__name__': 'isolated_diagnostic_frozen_driver'}
    os.environ['BIFROST_REPO_ROOT'] = str(root)
    exec(compile(source, str(wrapper), 'exec'), namespace)
    return namespace


def observe(root, image, output, attempt):
    ns = load_driver(root)
    output.mkdir(parents=True, exist_ok=False)
    started = time.monotonic()
    events = []
    original_rpc = ns['rpc']
    original_ready = ns['ready']
    original_wait_health = ns['wait_health']
    outcome = {'attempt': attempt, 'original_readiness_failure': None,
               'modern_empty_observed': False, 'modern_populated_observed': False,
               'stable_compatibility_qualified': False}

    def record(kind, **fields):
        event = {'kind': kind, 'elapsed_ms': round((time.monotonic() - started) * 1000, 3),
                 'timestamp_utc': datetime.now(timezone.utc).isoformat(), **ns['redact'](fields)}
        events.append(event)
        with (output / 'timeline.jsonl').open('a') as log:
            log.write(json.dumps(event) + '\n')
        return event

    def traced_rpc(url, path, method, params, token):
        try:
            status, body = original_rpc(url, path, method, params, token)
            names = sorted(t.get('name', '') for t in body.get('result', {}).get('tools', []))
            record('original_readiness_rpc', path=path, method=method, status=status,
                   tool_names=names, tool_count=len(names), error=body.get('error'))
            if path.endswith('fixture-modern') and method == 'tools/list' and status == 200:
                outcome['modern_empty_observed'] |= not names
                outcome['modern_populated_observed'] |= 'mcp_apps_fixture_modern-open_view' in names
            return status, body
        except Exception as error:
            record('original_transport_error', path=path, error_type=type(error).__name__, message=str(error))
            raise

    def traced_health(url):
        record('health_wait_started')
        original_wait_health(url)
        record('health_ready')

    def instrumented_ready(url, scopes):
        record('original_ready_started')
        try:
            original_ready(url, scopes)
            record('original_readiness_passed')
        except Exception as error:
            outcome['original_readiness_failure'] = str(error)
            record('original_readiness_failed', error_type=type(error).__name__, message=str(error))
        deadline = time.monotonic() + 180
        stable = 0
        samples = 0
        required = {
            '/mcp/mcp-apps-fixture': {'mcp_apps_fixture-open_view'},
            '/mcp/mcp-apps-fixture-second': {'mcp_apps_fixture_second-open_view'},
            '/mcp/mcp-apps-fixture-modern': {'mcp_apps_fixture_modern-open_view',
                                               'mcp_apps_fixture_modern-save_state',
                                               'mcp_apps_fixture_modern-read_state'},
            '/mcp/mcp-apps-full-virtual': {'mcp_apps_fixture-open_view', 'mcp_apps_fixture_second-open_view'},
            '/mcp/mcp-apps-limited-virtual': {'mcp_apps_fixture-open_view'},
            '/mcp': {'mcp_apps_fixture_modern-open_view', 'mcp_apps_fixture_second-open_view', 'executeToolCode'},
            '/mcp/error-test-server': {'executeToolCode'},
        }
        while time.monotonic() < deadline:
            all_ready = True
            try:
                status, data = ns['setup_request'](url, '/api/mcp/clients', 'GET')
                clients = [{'name': c.get('config', {}).get('name', c.get('name')),
                            'state': c.get('state'),
                            'tool_names': [t.get('name') for t in c.get('tools', [])],
                            'error': c.get('error', c.get('last_error'))}
                           for c in data.get('clients', [])]
                record('client_discovery', status=status, clients=clients)
            except Exception as error:
                record('client_discovery_error', error_type=type(error).__name__, message=str(error))
                all_ready = False
            for path, expected in required.items():
                role = 'limited' if path.endswith('limited-virtual') else 'codemode_errors' if path.endswith('error-test-server') else 'full'
                try:
                    status, body = original_rpc(url, path, 'tools/list', {}, ns['KEYS'][role])
                    names = {t.get('name', '') for t in body.get('result', {}).get('tools', [])}
                    complete = status == 200 and expected <= names
                    if role == 'limited' and any(name.endswith('fail_tool') for name in names):
                        complete = False
                    record('catalog_observation', path=path, credential_role=role, status=status,
                           tool_count=len(names), tool_names=sorted(names), expected_names=sorted(expected),
                           source_requirements_met=complete, error=body.get('error'))
                    all_ready = all_ready and complete
                    if path.endswith('fixture-modern') and status == 200:
                        outcome['modern_empty_observed'] |= not names
                        outcome['modern_populated_observed'] |= expected <= names
                except Exception as error:
                    record('catalog_transport_error', path=path, error_type=type(error).__name__, message=str(error))
                    all_ready = False
            try:
                status, body = original_rpc(url, '/mcp', 'tools/list', {}, ns['KEYS']['denied'])
                names = sorted(t.get('name', '') for t in body.get('result', {}).get('tools', []))
                record('denied_control', status=status, tool_names=names, passed=status == 200 and not names)
                all_ready = all_ready and status == 200 and not names
            except Exception as error:
                record('denied_control_error', message=str(error))
                all_ready = False
            samples += 1
            stable = stable + 1 if all_ready else 0
            if stable >= 3:
                outcome['complete_controlled_catalog_observed'] = True
                break
            if samples % 20 == 0:
                print(json.dumps({'diagnostic_attempt': attempt, 'elapsed_seconds': round(time.monotonic() - started),
                                  'modern_empty_seen': outcome['modern_empty_observed']}), flush=True)
            time.sleep(0.25 if samples < 20 else 1)
        outcome['observation_samples'] = samples
        outcome['catalog_deadline_seconds'] = 180
        record('diagnostic_observation_finished', **outcome)
        raise DiagnosticFinished('Stop before Newman, Code Mode wire or host execution')

    def no_qualification(*args, **kwargs):
        raise RuntimeError('Diagnostic scope forbids Newman, host and full qualification execution')

    original_run = ns['run']

    def bounded_command(command, *args, **kwargs):
        if not command or command[:2] != ['docker', 'logs']:
            no_qualification()
        return original_run(command, *args, **kwargs)

    ns['rpc'] = traced_rpc
    ns['wait_health'] = traced_health
    ns['ready'] = instrumented_ready
    ns['run_reference_host'] = no_qualification
    ns['run_firstboot_harness_phase'] = no_qualification
    ns['run'] = bounded_command
    try:
        ns['main']([image, str(output)])
        raise RuntimeError('Frozen driver returned without diagnostic stop boundary')
    except DiagnosticFinished:
        outcome['diagnostic_stop_before_qualification'] = True
    except Exception as error:
        outcome['preparation_error'] = str(error)
        record('diagnostic_preparation_error', error_type=type(error).__name__, message=str(error))
        raise
    finally:
        (output / 'observations.json').write_text(json.dumps({**outcome, 'events': events}, indent=2) + '\n')
    return outcome


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--driver-root', type=Path, required=True)
    parser.add_argument('--image', required=True)
    parser.add_argument('--report-dir', type=Path, required=True)
    args = parser.parse_args()
    if os.environ.get('GITHUB_ACTIONS') != 'true' or os.environ.get('GITHUB_REPOSITORY') != 'sofianbll/bifrost':
        raise RuntimeError('Exact-image diagnostic is restricted to existing fork Actions')
    args.report_dir.mkdir(parents=True, exist_ok=True)
    identity = source_identity(args.driver_root.resolve(), DRIVER)
    manifest = {'purpose': 'Bounded observation only; no correction or qualification',
                'candidate_source': CANDIDATE, 'stock_reference': STOCK, 'driver_source': identity,
                'diagnostic_orchestration_source': os.environ['GITHUB_SHA'],
                'max_startups': 3, 'catalog_deadline_seconds_per_startup': 180,
                'provider_calls': 0, 'newman_executed': False, 'host_executed': False,
                'stable_compatibility_qualified': False,
                'script_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest()}
    (args.report_dir / 'diagnostic-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    rows = []
    try:
        for attempt in range(1, 4):
            row = observe(args.driver_root.resolve(), args.image, args.report_dir / f'startup-{attempt:03}', attempt)
            rows.append(row)
            print(json.dumps(row), flush=True)
            if row['original_readiness_failure'] == 'MCP tools/list readiness failed on /mcp/mcp-apps-fixture-modern (HTTP 200, tools=0)':
                return 1
        return 0
    finally:
        (args.report_dir / 'diagnostic-summary.json').write_text(json.dumps({**manifest, 'attempts': rows}, indent=2) + '\n')


if __name__ == '__main__':
    raise SystemExit(main())
