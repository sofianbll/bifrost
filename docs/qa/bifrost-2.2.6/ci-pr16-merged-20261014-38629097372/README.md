# PR #16 post-merge qualification — 2026-10-14

The normal merge commit `1dcbd14d5875a14dc780f00fc2edd22cfaee66a8` passed
[GitHub Actions](https://github.com/sofianbll/bifrost/actions/runs/38629097372).
Both MCP race suites and both AMD64/ARM64 builds passed; publication was skipped.
The real-App campaign passed 80/80 and reference-host rendering passed 22/22.
The strict official/corrected Excalidraw direct/gateway comparison passes,
including fixed-camera save/read/restore and deliberate displacement detection.

The manifest retains tested image/source identity and version, routes, checkpoint
reports, source/HTML hashes and copied-file hashes. Captures are synthetic CI views.
Raw gateway/Newman logs and console dumps are excluded. JSON credential scanning
passed before copying. This sanitized snapshot is preserved locally; export requires owner approval;
the GitHub artifact has a 14-day retention window.

This proof is for the merged PR #16 source, before the later sorted-JSON convention
correction in PR #19. It is not a result for that newer candidate. Automated
reference-host verification is distinct from `codexVerification: not_run`.
No personal Codex App, production, release tag, image publication or deployment
was used or changed.

Static inspection of the CI captures confirms corrected restored labels are
centered, pristine official direct restoration shifts them, and the deliberate
Bifrost-label displacement is visible. This is inspection of CI artifacts,
not native Codex verification.
