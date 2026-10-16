#!/usr/bin/env python3
"""Bind the driver Actions run to the exact fork candidate with a commit status."""
import json
import os
import re
import sys
import urllib.error
import urllib.request

REPOSITORY = "sofianbll/bifrost"
CONTEXT = "stable-23-compatibility/qualification"


def publish(state):
    if state not in {"pending", "success", "failure"}:
        raise ValueError("status must be pending, success or failure")
    if os.environ.get("GITHUB_REPOSITORY") != REPOSITORY:
        raise RuntimeError("status publication is restricted to the configured fork")
    candidate = os.environ.get("CANDIDATE_SOURCE", "")
    if not re.fullmatch(r"[0-9a-f]{40}", candidate):
        raise RuntimeError("candidate source must be an immutable commit SHA")
    token = os.environ.get("GITHUB_TOKEN", "")
    if not token:
        raise RuntimeError("GitHub Actions token is unavailable")
    run_url = f"https://github.com/{REPOSITORY}/actions/runs/{os.environ['GITHUB_RUN_ID']}"
    descriptions = {
        "pending": "Stable 2.3 source-pinned qualification is running",
        "success": "Stable 2.3 qualification passed; review the immutable run artifact",
        "failure": "Stable 2.3 qualification failed or was blocked; review the run artifact",
    }
    payload = {
        "state": state,
        "context": CONTEXT,
        "description": descriptions[state],
        "target_url": run_url,
    }
    request = urllib.request.Request(
        f"https://api.github.com/repos/{REPOSITORY}/statuses/{candidate}",
        data=json.dumps(payload).encode(),
        headers={
            "Accept": "application/vnd.github+json",
            "Authorization": f"Bearer {token}",
            "X-GitHub-Api-Version": "2022-11-28",
            "Content-Type": "application/json",
            "User-Agent": "stable23-qualification-driver",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=30) as response:
            if response.status not in (200, 201):
                raise RuntimeError(f"GitHub status API returned HTTP {response.status}")
    except urllib.error.HTTPError as error:
        raise RuntimeError(f"GitHub status API returned HTTP {error.code}") from None
    except urllib.error.URLError as error:
        raise RuntimeError(f"GitHub status API request failed: {error.reason}") from None
    print(f"Published {state} status `{CONTEXT}` for {candidate}; run={run_url}")


if __name__ == "__main__":
    try:
        publish(sys.argv[1])
    except (IndexError, RuntimeError, ValueError) as error:
        print(f"candidate status publication failed: {error}", file=sys.stderr)
        raise SystemExit(1)
