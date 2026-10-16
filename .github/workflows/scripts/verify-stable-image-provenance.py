#!/usr/bin/env python3
"""Extract stable gateway binaries and prove local module/compiler provenance."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

REQUIRED_MODULES = {
    "github.com/maximhq/bifrost/transports",
    "github.com/maximhq/bifrost/core",
    "github.com/maximhq/bifrost/framework",
    "github.com/maximhq/bifrost/plugins/governance",
}
EXPECTED_COMPILER = "go1.27.0"


def parse_build_info(output):
    lines = output.splitlines()
    compiler = next((line.split(":", 1)[1].strip().split()[0] for line in lines
                     if ": go1." in line and len(line.split(":", 1)) == 2), "")
    modules = {}
    for line in lines:
        fields = line.split()
        if len(fields) >= 3 and fields[0] in ("mod", "dep"):
            modules[fields[1]] = fields[2]
    bifrost_modules = {path: version for path, version in modules.items()
                       if path == "github.com/maximhq/bifrost" or path.startswith("github.com/maximhq/bifrost/")}
    problems = []
    missing = sorted(REQUIRED_MODULES - {path for path, version in bifrost_modules.items() if version == "(devel)"})
    if missing:
        problems.append(f"required local Bifrost modules missing: {missing}")
    published = sorted(path for path, version in bifrost_modules.items() if version != "(devel)")
    if published:
        problems.append(f"linked Bifrost modules are published/non-local: {published}")
    if compiler != EXPECTED_COMPILER:
        problems.append(f"image compiler is {compiler!r}, expected {EXPECTED_COMPILER!r}")
    return {"compiler": compiler, "modules": modules, "bifrost_modules": bifrost_modules,
            "validation_errors": problems}


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args], text=True).strip()


def sha256(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def record_images(report_dir, run_id, stock_root, candidate_root, stock_sha, candidate_sha):
    records = []

    def persist():
        (report_dir / "gateway-image-provenance.json").write_text(json.dumps(records, indent=2) + "\n")

    for role, root, expected_sha in (("stock", stock_root, stock_sha),
                                     ("candidate", candidate_root, candidate_sha)):
        if git(root, "rev-parse", "HEAD") != expected_sha:
            raise RuntimeError(f"{role} source HEAD changed before image inspection")
        dirty = git(root, "status", "--porcelain", "--untracked-files=all")
        if dirty:
            raise RuntimeError(f"{role} source worktree is dirty before image inspection: {dirty}")
        image = f"bifrost:{role}-{run_id}"
        image_id = subprocess.check_output(["docker", "image", "inspect", "--format", "{{.Id}}", image], text=True).strip()
        dockerfile = root / "transports/Dockerfile.local"
        record = {
            "role": role,
            "source": expected_sha,
            "source_tree": git(root, "rev-parse", "HEAD^{tree}"),
            "tracked_status": "clean",
            "image": image,
            "image_id": image_id,
            "binary_sha256": None,
            "dockerfile": "transports/Dockerfile.local",
            "dockerfile_sha256": sha256(dockerfile),
            "compiler": None,
            "modules": {},
            "bifrost_modules": {},
            "build_info": None,
            "validation_errors": [],
        }
        records.append(record)
        persist()
        container = f"stable23-build-info-{role}-{run_id}"
        subprocess.run(["docker", "create", "--name", container, image], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        try:
            with tempfile.TemporaryDirectory(prefix=f"stable23-image-{role}-") as temp:
                binary = Path(temp) / "gateway-main"
                subprocess.run(["docker", "cp", f"{container}:/app/main", str(binary)], check=True,
                               stdout=subprocess.DEVNULL, stderr=subprocess.PIPE, text=True)
                info = subprocess.check_output(["go", "version", "-m", str(binary)], text=True)
                parsed = parse_build_info(info)
                binary_sha = sha256(binary)
        finally:
            subprocess.run(["docker", "rm", container], check=True, stdout=subprocess.DEVNULL,
                           stderr=subprocess.DEVNULL)
        record.update({
            "binary_sha256": binary_sha,
            "compiler": parsed["compiler"],
            "modules": parsed["modules"],
            "bifrost_modules": parsed["bifrost_modules"],
            "build_info": info,
            "validation_errors": parsed["validation_errors"],
        })
        persist()
        if parsed["validation_errors"]:
            raise RuntimeError(f"{role} binary provenance failed: {parsed['validation_errors']}")


def self_check():
    valid = """/app/main: go1.27.0
\tmod github.com/maximhq/bifrost/transports (devel)
\tdep github.com/maximhq/bifrost/core (devel)
\tdep github.com/maximhq/bifrost/framework (devel)
\tdep github.com/maximhq/bifrost/plugins/governance (devel)
\tdep github.com/maximhq/bifrost/plugins/telemetry (devel)
"""
    assert parse_build_info(valid)["validation_errors"] == []
    published = valid.replace("dep github.com/maximhq/bifrost/core (devel)",
                              "dep github.com/maximhq/bifrost/core v2.3.0")
    assert any("published/non-local" in error for error in parse_build_info(published)["validation_errors"])
    suffixed = valid.replace("mod github.com/maximhq/bifrost/transports (devel)",
                             "mod github.com/maximhq/bifrost/transports/v2 (devel)")
    assert any("required local" in error for error in parse_build_info(suffixed)["validation_errors"])
    wrong_version = valid.replace("go1.27.0", "go1.27.2")
    assert any("compiler" in error for error in parse_build_info(wrong_version)["validation_errors"])
    print("PASS stable image provenance self-check: unsuffixed module, four local modules, no published dependencies, Go 1.27.0")


if __name__ == "__main__":
    if sys.argv[1:] == ["--self-check"]:
        self_check()
        raise SystemExit(0)
    try:
        record_images(Path(os.environ["REPORT_DIR"]), os.environ["GITHUB_RUN_ID"],
                      Path("stock"), Path("candidate"), os.environ["STOCK_SOURCE"],
                      os.environ["CANDIDATE_SOURCE"])
    except (OSError, KeyError, RuntimeError, subprocess.CalledProcessError) as error:
        print(f"image provenance verification failed: {error}", file=sys.stderr)
        raise SystemExit(1)
