#!/usr/bin/env python3
"""Bind independent zero-finding reviews to the exact stable qualification candidate."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys


SHA = re.compile(r"^[0-9a-f]{40}$")
REVIEWERS = re.compile(r"(?m)^Independent reviewer:\s*([^\s#]+)\s*$")
FINDINGS = re.compile(r"(?im)^Findings:\s*(\d+)\s*$")


def digest(path):
    value = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            value.update(block)
    return value.hexdigest()


def verify_review(axis, path, base, head):
    record = {"axis": axis, "path": str(path)}
    problems = []
    if not path.is_file():
        return {**record, "status": "missing", "problems": ["review report is absent"]}
    content = path.read_text(encoding="utf-8")
    record["sha256"] = digest(path)
    if not re.search(rf"(?m)^Review axis:\s*{re.escape(axis)}\s*$", content):
        problems.append(f"report does not declare Review axis: {axis}")
    if not re.search(rf"(?im)^Base:\s*`?{re.escape(base)}`?\s*$", content):
        problems.append("report does not declare the exact full stock base SHA")
    if not re.search(rf"(?im)^HEAD:\s*`?{re.escape(head)}`?\s*$", content):
        problems.append("report does not declare the exact full candidate HEAD SHA")
    reviewers = REVIEWERS.findall(content)
    if len(reviewers) != 1:
        problems.append("report must declare exactly one Independent reviewer ID")
    else:
        record["independent_reviewer"] = reviewers[0]
    findings = FINDINGS.findall(content)
    if findings != ["0"]:
        problems.append("report must declare exactly Findings: 0")
    else:
        record["findings"] = 0
    record["status"] = "passed" if not problems else "failed"
    record["problems"] = problems
    return record


def build_manifest(base, head, standards_path, spec_path):
    manifest = {
        "base": base,
        "head": head,
        "reviews": [verify_review("Standards", standards_path, base, head),
                    verify_review("Spec", spec_path, base, head)],
    }
    reviewer_ids = [review.get("independent_reviewer") for review in manifest["reviews"]]
    if all(reviewer_ids) and len(set(reviewer_ids)) != len(reviewer_ids):
        for review in manifest["reviews"]:
            review["status"] = "failed"
            review["problems"].append("Standards and Spec reviewers must be distinct independent agents")
    manifest["status"] = "passed" if all(review["status"] == "passed" for review in manifest["reviews"]) else "blocked"
    return manifest


def self_check():
    base = "9237ec06672406e437b3cb33fc793ffd0403b3ae"
    head = "e949886eb5212db2dc0ac7857e9934527ecb5044"
    standards = "\n".join(("Review axis: Standards", f"Base: {base}", f"HEAD: {head}",
                             "Independent reviewer: standards-agent-1", "Findings: 0", ""))
    spec = "\n".join(("Review axis: Spec", f"Base: {base}", f"HEAD: {head}",
                       "Independent reviewer: spec-agent-2", "Findings: 0", ""))
    import tempfile
    with tempfile.TemporaryDirectory() as temp:
        directory = Path(temp)
        standards_path = directory / "standards.md"
        spec_path = directory / "spec.md"
        standards_path.write_text(standards)
        spec_path.write_text(spec)
        manifest = build_manifest(base, head, standards_path, spec_path)
        assert manifest["status"] == "passed"
        assert all(review["sha256"] for review in manifest["reviews"])
        spec_path.write_text(spec.replace(head, "0" * 40))
        assert build_manifest(base, head, standards_path, spec_path)["status"] == "blocked"
        spec_path.write_text(spec.replace("spec-agent-2", "standards-agent-1"))
        duplicate_reviewers = build_manifest(base, head, standards_path, spec_path)
        assert duplicate_reviewers["status"] == "blocked"
        assert all(review["status"] == "failed" for review in duplicate_reviewers["reviews"])
    print("PASS review provenance self-check: exact base/head, axes, distinct reviewers, zero findings, file digests")


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base")
    parser.add_argument("--head")
    parser.add_argument("--standards", type=Path)
    parser.add_argument("--spec", type=Path)
    parser.add_argument("--output", type=Path)
    parser.add_argument("--self-check", action="store_true")
    args = parser.parse_args()
    if args.self_check:
        self_check()
        return 0
    if not all((args.base, args.head, args.standards, args.spec, args.output)):
        parser.error("--base, --head, --standards, --spec and --output are required")
    if not SHA.fullmatch(args.base) or not SHA.fullmatch(args.head):
        parser.error("base and head must be full lowercase 40-character commit SHAs")
    manifest = build_manifest(args.base, args.head, args.standards, args.spec)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(manifest, indent=2) + "\n")
    print(json.dumps({"status": manifest["status"], "reviews": manifest["reviews"]}, sort_keys=True))
    return 0 if manifest["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
