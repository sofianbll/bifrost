#!/usr/bin/env python3
"""Calibrate SIGTERM cleanup against the old script and the corrected real gateway probe."""
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time


# This test-only wrapper controls the external process boundary, never Bifrost's application logic.
CLEANUP_SIGNAL = """
import os, runpy, signal, subprocess, sys
terminate = subprocess.Popen.terminate
wait = subprocess.Popen.wait
fired = False

def stop_before_terminate(child):
    os.kill(child.pid, signal.SIGSTOP)
    return terminate(child)

def signal_during_wait(child, *args, **kwargs):
    global fired
    if not fired:
        fired = True
        os.kill(os.getpid(), signal.SIGTERM)
    return wait(child, *args, **kwargs)

subprocess.Popen.terminate = stop_before_terminate
subprocess.Popen.wait = signal_during_wait
script = sys.argv.pop(1)
runpy.run_path(script, run_name="__main__")
"""


def running(pid):
    try:
        return Path(f"/proc/{pid}/stat").read_text().split()[2] != "Z"
    except FileNotFoundError:
        return False


def check_cleanup(script, binary, variant, commit, legacy, during_cleanup=False):
    with tempfile.TemporaryDirectory(prefix="bifrost-interruption-") as tmp:
        directory = Path(tmp)
        child_report = directory / "interrupted.json"
        command = [sys.executable]
        if during_cleanup:
            command.extend(["-c", CLEANUP_SIGNAL])
        command.extend([str(script), "--binary", binary, "--variant", variant,
                        "--commit", commit, "--output", str(child_report)])
        environment = dict(os.environ, TMPDIR=tmp)
        with (directory / "private-driver.log").open("wb") as log:
            process = subprocess.Popen(command, env=environment, stdout=log, stderr=log, start_new_session=True)
            child_pids = []
            try:
                deadline = time.monotonic() + 60
                while process.poll() is None and time.monotonic() < deadline:
                    children = Path(f"/proc/{process.pid}/task/{process.pid}/children").read_text().split()
                    stores = list(directory.glob("bifrost-persistent-ci-*/config.db"))
                    if children and stores:
                        child_pids = [int(pid) for pid in children]
                        break
                    time.sleep(.1)
                if not child_pids:
                    raise AssertionError("no running gateway with a disposable SQLite store before interruption")
                if not during_cleanup:
                    process.send_signal(signal.SIGTERM)
                status = process.wait(timeout=90 if during_cleanup else 20)
                child_alive = any(running(pid) for pid in child_pids)
                private_files = bool(list(directory.glob("bifrost-persistent-ci-*")))
                interrupted = json.loads(child_report.read_text()) if child_report.exists() else {}
                if legacy:
                    if not child_alive:
                        raise AssertionError("pristine old script did not leave the real gateway child alive")
                    if not during_cleanup and (status != -15 or not private_files or child_report.exists()):
                        raise AssertionError("pristine old script did not demonstrate the initial SIGTERM defect")
                    if during_cleanup and (status != 143 or interrupted.get("result") != "interrupted"
                                           or interrupted.get("signal") != 15):
                        raise AssertionError("pristine old script did not receive SIGTERM during normal cleanup")
                else:
                    if status != 143 or interrupted.get("result") != "interrupted" or interrupted.get("signal") != 15:
                        raise AssertionError("SIGTERM did not produce the explicit interrupted verdict and exit 143")
                    if child_alive or private_files:
                        raise AssertionError("interruption left a live gateway child or private store/logs")
                result = {"kind": "legacy red control" if legacy else "corrected green control", "exit_code": status,
                          "signal_phase": "normal cleanup" if during_cleanup else "initial boot",
                          "injection": "SIGSTOP child at terminate, SIGTERM parent at wait" if during_cleanup else "SIGTERM parent",
                          "script_sha256": hashlib.sha256(script.read_bytes()).hexdigest(),
                          "child_was_running": True, "sqlite_store_existed": True,
                          "child_left_running": child_alive, "private_files_left": private_files,
                          "interrupted_verdict": interrupted.get("result")}
            finally:
                # The driver owns this isolated process group, including legacy-red and failure paths.
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait(timeout=5)
                deadline = time.monotonic() + 5
                while any(running(pid) for pid in child_pids) and time.monotonic() < deadline:
                    time.sleep(.1)
                if any(running(pid) for pid in child_pids):
                    raise AssertionError("driver failed to stop its isolated child process group")
        result["driver_stopped_remaining_children"] = True
    result["driver_removed_private_files"] = not directory.exists()
    return result


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise RuntimeError("application execution is restricted to GitHub Actions")
    binary, variant, commit, output, legacy_script, legacy_cleanup_script = sys.argv[1:]

    def interrupt(_signum, _frame):
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        signal.signal(signal.SIGINT, signal.SIG_IGN)
        raise KeyboardInterrupt

    signal.signal(signal.SIGTERM, interrupt)
    signal.signal(signal.SIGINT, interrupt)
    current = Path(__file__).with_name("test-persistent-mcp-restart.py")
    report = {"check": "real SIGTERM cleanup red/green", "variant": variant, "commit": commit,
              "legacy_startup_script_commit": "d7416ed5b3b13a2f961c9b6e60d83e8a6c56c65e",
              "legacy_cleanup_script_commit": "f4527a9ab72210952c03e37ba30e200c5cea5bb4",
              "initial_boot": {"before": check_cleanup(Path(legacy_script), binary, variant, commit, True),
                               "after": check_cleanup(current, binary, variant, commit, False)},
              "normal_cleanup": {"before": check_cleanup(Path(legacy_cleanup_script), binary, variant, commit, True, True),
                                 "after": check_cleanup(current, binary, variant, commit, False, True)},
              "result": "pass"}
    target = Path(output)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
