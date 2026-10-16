#!/usr/bin/env python3
"""Verify the stable workspace resolves every linked Bifrost module locally."""
import argparse
import json
from pathlib import Path
import sys


REQUIRED_MODULES = {
    "github.com/maximhq/bifrost/transports",
    "github.com/maximhq/bifrost/core",
    "github.com/maximhq/bifrost/framework",
    "github.com/maximhq/bifrost/plugins/governance",
}
PREFIX = "github.com/maximhq/bifrost"


def verify(root, manifest_path):
    root = root.resolve()
    rows = []
    for number, raw in enumerate(manifest_path.read_text().splitlines(), 1):
        fields = raw.split(maxsplit=1)
        if not fields:
            continue
        if len(fields) == 1:
            module, location = fields[0], None
        else:
            module, location = fields[0], fields[1].strip() or None
        is_bifrost = module == PREFIX or module.startswith(PREFIX + "/")
        if is_bifrost and not location:
            raise RuntimeError(f"malformed module manifest line {number}: {raw!r}")
        if is_bifrost:
            path = Path(location).resolve()
            try:
                path.relative_to(root)
            except ValueError:
                raise RuntimeError(f"Bifrost module {module} resolves outside the pinned source: {path}") from None
            if not path.is_dir() or not (path / "go.mod").is_file():
                raise RuntimeError(f"Bifrost module {module} has no local go.mod directory: {path}")
        rows.append({"module": module, "directory": location})
    available = {row["module"] for row in rows}
    missing = sorted(REQUIRED_MODULES - available)
    if missing:
        raise RuntimeError(f"required Bifrost workspace modules are missing: {missing}")
    return {"source_root": str(root), "required_local_modules": sorted(REQUIRED_MODULES),
            "bifrost_modules": [row for row in rows if row["module"] == PREFIX or row["module"].startswith(PREFIX + "/")],
            "external_modules": [row for row in rows if row["module"] != PREFIX and not row["module"].startswith(PREFIX + "/")],
            "all_bifrost_modules_local_to_source": True}


def self_check():
    import tempfile
    with tempfile.TemporaryDirectory() as temp:
        root = Path(temp)
        for module in REQUIRED_MODULES:
            (root / module.rsplit("/", 1)[-1]).mkdir()
        records = []
        for module in sorted(REQUIRED_MODULES):
            local = root / module.rsplit("/", 1)[-1]
            (local / "go.mod").write_text("module " + module + "\n")
            records.append(f"{module} {local}")
        manifest = root / "modules.txt"
        manifest.write_text("\n".join(records) + "\n" + "golang.org/x/tools \n")
        valid_report = verify(root, manifest)
        assert valid_report["all_bifrost_modules_local_to_source"]
        assert valid_report["external_modules"] == [{"module": "golang.org/x/tools", "directory": None}]
        manifest.write_text(manifest.read_text() + f"{PREFIX}/plugins/telemetry \n")
        try:
            verify(root, manifest)
        except RuntimeError as error:
            assert "malformed module manifest" in str(error)
        else:
            raise AssertionError("accepted a linked Bifrost module without a local directory")
        manifest.write_text("\n".join(records) + "\n" + f"{PREFIX}/plugins/telemetry /tmp/published-module\n")
        try:
            verify(root, manifest)
        except RuntimeError as error:
            assert "outside the pinned source" in str(error)
        else:
            raise AssertionError("accepted a Bifrost module outside the pinned source")
    print("PASS workspace provenance self-check: local Bifrost modules required, blank external directory accepted, blank/published Bifrost module rejected")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source-root", type=Path)
    parser.add_argument("--manifest", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--self-check", action="store_true")
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return 0
    if not (args.source_root and args.manifest and args.output):
        parser.error("--source-root, --manifest and --output are required")
    try:
        report = verify(args.source_root, args.manifest)
        args.output.write_text(json.dumps(report, indent=2) + "\n")
    except (OSError, RuntimeError) as error:
        print(f"workspace provenance verification failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
