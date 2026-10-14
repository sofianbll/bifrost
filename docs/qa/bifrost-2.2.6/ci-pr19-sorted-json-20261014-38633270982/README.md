# PR #19 sorted-JSON qualification — 2026-10-14

The reviewed sorted-JSON code commit `c10746444bc35135fe7e830a3b630385ef412b1f` passed
[GitHub Actions](https://github.com/sofianbll/bifrost/actions/runs/38633270982).
Both MCP race suites and both AMD64/ARM64 builds passed; publication was skipped.
The real-App campaign passed 80/80 and reference-host rendering passed 22/22.
The strict official/corrected Excalidraw direct/gateway comparison passes,
including fixed-camera save/read/restore and deliberate displacement detection.

The manifest retains tested image/source identity and version, routes, checkpoint
reports, source/HTML hashes and copied-file hashes. Captures are synthetic CI views.
Raw gateway/Newman logs and console dumps are excluded. JSON credential scanning
passed before copying. This sanitized snapshot is preserved locally; export requires owner approval;
the GitHub artifact has a 14-day retention window.

This proof is for the exact PR #19 code after the sorted-JSON convention
correction. Any subsequent proof-only commit is distinguished from this tested
code SHA. Automated
reference-host verification is distinct from `codexVerification: not_run`.
No personal Codex App, production, release tag, image publication or deployment
was used or changed.
