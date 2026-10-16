#!/usr/bin/env python3
"""Bind independent zero-actionable-finding reviews to the exact stable qualification candidate."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys


SHA = re.compile(r"^[0-9a-f]{40}$")
REVIEWERS = re.compile(r"(?m)^Independent reviewer:\s*([^\s#]+)\s*$")
FINDINGS = re.compile(r"(?im)^Findings:\s*(\d+)\s*$")
ACTIONABLE = re.compile(r"(?im)^Actionable findings:\s*(\d+)\s*$")
NONBLOCKING = re.compile(r"(?im)^Nonblocking observations:\s*(\d+)\s*$")


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
    actionable = ACTIONABLE.findall(content)
    nonblocking = NONBLOCKING.findall(content)
    if len(findings) != 1:
        problems.append("report must declare exactly one total Findings count")
    elif actionable or nonblocking:
        if len(actionable) != 1 or len(nonblocking) != 1:
            problems.append("split counts must declare exactly one actionable and nonblocking count")
        else:
            record.update(findings=int(findings[0]), actionable_findings=int(actionable[0]),
                          nonblocking_observations=int(nonblocking[0]))
            if record["findings"] != record["actionable_findings"] + record["nonblocking_observations"]:
                problems.append("total findings must equal actionable plus nonblocking counts")
            if record["actionable_findings"] != 0:
                problems.append("report has actionable findings")
            marker = "## Verbatim independent review\n\n"
            hashes = re.findall(r"(?im)^Verbatim review SHA256:\s*([0-9a-f]{64})\s*$", content)
            if len(hashes) != 1 or content.count(marker) != 1:
                problems.append("split-count report must bind the verbatim independent review")
            else:
                actual_hash = hashlib.sha256(content.split(marker, 1)[1].encode()).hexdigest()
                record["verbatim_review_sha256"] = actual_hash
                if actual_hash != hashes[0]:
                    problems.append("verbatim independent review checksum mismatch")
    elif findings != ["0"]:
        problems.append("legacy report must declare Findings: 0; nonblocking observations require explicit split counts")
    else:
        record.update(findings=0, actionable_findings=0, nonblocking_observations=0)
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
        spec_path.write_text(spec)
        standards_path.write_text(standards.replace("Findings: 0", "Findings: 1"))
        assert build_manifest(base, head, standards_path, spec_path)["status"] == "blocked"
        original = "Accepted nonblocking lookup observation.\n"
        split = standards.replace("Findings: 0", "Findings: 1\nActionable findings: 0\nNonblocking observations: 1")
        split += "Verbatim review SHA256: " + hashlib.sha256(original.encode()).hexdigest() + "\n\n"
        split += "## Verbatim independent review\n\n" + original
        standards_path.write_text(split)
        split_manifest = build_manifest(base, head, standards_path, spec_path)
        assert split_manifest["status"] == "passed"
        assert split_manifest["reviews"][0]["findings"] == 1
        assert split_manifest["reviews"][0]["nonblocking_observations"] == 1
        for invalid in (split.replace("Actionable findings: 0", "Actionable findings: 1"),
                        split.replace("Findings: 1", "Findings: 2"),
                        split.replace(original, "Altered review.\n")):
            standards_path.write_text(invalid)
            assert build_manifest(base, head, standards_path, spec_path)["status"] == "blocked"
    print("PASS review provenance self-check: exact base/head, axes, distinct reviewers, zero actionable findings, disclosed observations, verbatim checksums")


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
