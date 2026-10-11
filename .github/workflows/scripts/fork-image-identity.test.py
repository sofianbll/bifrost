#!/usr/bin/env python3
"""Exercise the actual workflow identity block without publishing anything."""
import os
from pathlib import Path
import subprocess
import tempfile
import textwrap

root = Path(__file__).resolve().parents[3]
workflow = (root / ".github/workflows/fork-image.yml").read_text()
block = workflow.split("      - name: Image identity\n", 1)[1].split("\n      - ", 1)[0]
script = textwrap.dedent(block.split("        run: |\n", 1)[1])
sha = "abcdef1234567890"
base = (root / "transports/version").read_text().strip()
with tempfile.TemporaryDirectory() as directory:
    output = Path(directory) / "output"
    for kind, ref, expected, publish in [
        ("tag", f"image/{base}-sofian.1", f"{base}+sofian.1.{sha[:12]}", "true"),
        ("tag", "image/custom_name", f"{base}+sofian.{sha[:12]}", "true"),
        ("branch", "dev", f"{base}+sofian.{sha[:12]}", "false"),
        ("branch", "feature/example", f"{base}+sofian.{sha[:12]}", "false"),
        ("tag", "image/999.0.0-sofian.1", None, None),
        ("tag", "image/bad tag", None, None),
    ]:
        output.write_text("")
        env = dict(os.environ, GITHUB_REF_TYPE=kind, GITHUB_REF_NAME=ref,
                   GITHUB_SHA=sha, GITHUB_RUN_NUMBER="42", GITHUB_OUTPUT=str(output))
        result = subprocess.run(["bash", "-e", "-c", script], cwd=root, env=env,
                                capture_output=True, text=True)
        if expected is None:
            assert result.returncode != 0, ref
        else:
            assert result.returncode == 0, result.stderr
            values = dict(line.split("=", 1) for line in output.read_text().splitlines())
            assert values["version"] == expected, values
            assert values["tag"] == (ref.removeprefix("image/") if kind == "tag" else f"dev-{sha[:12]}-42"), values
            assert values["publish"] == publish, values
print("PASS: tagged, custom, dev, feature-branch, mismatched and invalid image identities")
