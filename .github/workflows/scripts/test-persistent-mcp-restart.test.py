#!/usr/bin/env python3
"""Prove SIGTERM cleans up a real disposable gateway and its private store in CI."""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise RuntimeError("application execution is restricted to GitHub Actions")
    binary, variant, commit, output = sys.argv[1:]
    report = {"check": "real SIGTERM cleanup", "variant": variant, "commit": commit, "result": "fail"}
    with tempfile.TemporaryDirectory(prefix="bifrost-interruption-") as tmp:
        directory = Path(tmp)
        child_report = directory / "interrupted.json"
        command = [sys.executable, str(Path(__file__).with_name("test-persistent-mcp-restart.py")),
                   "--binary", binary, "--variant", variant, "--commit", commit, "--output", str(child_report)]
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
                process.send_signal(signal.SIGTERM)
                status = process.wait(timeout=20)
                interrupted = json.loads(child_report.read_text())
                if status != 143 or interrupted.get("result") != "interrupted" or interrupted.get("signal") != 15:
                    raise AssertionError("SIGTERM did not produce the explicit interrupted verdict and exit 143")
                if any(Path(f"/proc/{pid}").exists() for pid in child_pids):
                    raise AssertionError("interruption left a live gateway child")
                if list(directory.glob("bifrost-persistent-ci-*")):
                    raise AssertionError("interruption left the private gateway store or logs")
                report.update({"result": "pass", "child_was_running": True, "sqlite_store_existed": True,
                               "child_stopped": True, "private_files_removed": True,
                               "interrupted_verdict": "interrupted", "exit_code": status})
            finally:
                # The driver owns this isolated process group, including failure paths.
                try:
                    os.killpg(process.pid, signal.SIGKILL)
                except ProcessLookupError:
                    pass
                process.wait(timeout=5)
    target = Path(output)
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_text(json.dumps(report, indent=2) + "\n")
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
