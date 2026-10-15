# Clean Bifrost 2.2.6 stack qualification

The clean A/B/C qualification completed successfully in GitHub Actions. This is an automated qualification report; it is not a release or production report. New proof files are conserved and versioned locally only while export authorization is pending. No production access, image publication, release, tag, deployment, paid-provider test or personal Codex App session is part of this work.

The code and harness identities are separate:

| Purpose | Commit |
| --- | --- |
| Upstream 2.2.6 baseline | `8b4fce4f1709d66f9208d02f50552da522535f9e` |
| Candidate A: Code Mode errors and deterministic regression | `f136c93fcf78f3db3df59974ae49cfc392412f51` |
| Candidate B: configuration reconciliation | `adb80e661560174025ff9f9c23b918cb377e14b9` |
| Candidate C: native Apps relay and sorted response encoders | `a369739b6c5c6cd0921bce4fa8e0b9a6ae6b6034` |
| Fork fixture/build harness | `1dcbd14d5875a14dc780f00fc2edd22cfaee66a8` |
| Executed workflow wrapper | `9a5c3e80963d8b33f8604dcf4670acb457de3aec` |

[PR #18](https://github.com/sofianbll/bifrost/pull/18) contains the fork-only wrapper. Root owns its readiness state and any merge. Matt Standards/Spec review found no code defect; the two initial pushed `[ci]` commit subjects retain a documented convention issue because the project's allowed build/tooling type is `[chore]`. History was preserved. The latest repin uses `[chore]`.

[Qualification run 38637028433](https://github.com/sofianbll/bifrost/actions/runs/38637028433) completed with success on all three immutable candidates. Each complete MCP mock/race suite reports 704 total tests, 675 passed, 29 skipped, no failure or error. All independent Go/UI builds, package race/plugin builds, runtime plugin loading and byte-for-byte built UI probes passed. B/C focused configuration and C MCP persistence checks passed. C additionally passed 39 fixture wire cases/149 assertions, the strict stock comparison (9 expected red sentinels and 7 passing invariants), real Apps 80/80, official reference-host checks 22/22, the actual checkpoint normalizer 10/10, and immediate/delayed initialization. All figures refer to the candidate commits below, not to the wrapper or a documentation commit.

C's tested image is `sha256:c6cfd9120839b04605aa31009c9dd7af4506994c24183bb44edcf7c2480f592d`, display version `2.2.6+qualification.a369739b6c5c`, candidate tree `6822150a7219b540e2c9e1e76d52f7e426fc714d`. No image was published. The automated report explicitly records native Codex verification as `not_run`. Static inspection of CI PNGs confirms centered corrected labels before/after restoration and visible displacement in the official direct restore; that file inspection is not a native Codex App test.

Each candidate builds its own UI and local Go modules. The temporary Dockerfile is the harness file with exactly the absent fork-only `compareVersions.test.ts` unit-test invocation removed. The UI build/typecheck, Go build, package race tests and plugin/UI probe remain. No fork UI or module source is copied into a candidate. Reports record candidate commit/tree/image ID/version separately from wrapper and harness commits, plus fixture and adapted Dockerfile hashes.

The runtime display version uses the first 12 candidate SHA characters; OCI revision and source provenance retain the full SHA. The wrapper asserts that the version including the binary's `v` prefix fits the stock 59-character startup banner. This changes build identity only and does not patch candidate source.

Only C implements Apps, so only C runs the complete wire fixture and strict stock 2.2.6 comparison, real Excalidraw/PDF/debug/monitor routes, and the pinned official reference host with official/corrected Excalidraw in direct/gateway legs. Lower A/B are not measured against an absent upper feature. Independent stack coverage is AMD64; the separate fork integration campaign covers AMD64/ARM64. Reference-host automation does not claim native Codex host verification.

Earlier runs are kept separately:

- [38631710488](https://github.com/sofianbll/bifrost/actions/runs/38631710488), wrapper `931f03f60d60ee907dd84085a41a25ed56d667b8`, was rejected before any job existed. `runner.temp` was incorrectly used in job-level environment evaluation. The first wrapper repair moves report-path initialization to a runner step via `GITHUB_ENV`, matching the [GitHub context availability reference](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts). No candidate test ran in that rejected run.
- [38632023979](https://github.com/sofianbll/bifrost/actions/runs/38632023979), wrapper `22824934ee91b265e34c1b39248203a329c24ed2`, was cancelled when review advanced C from `314e02b21dbc0ffc7be68ede049005c0276fe0b8` to `a369739b`. Its three MCP suites passed: 704 total, 675 passed, 29 skipped, no failure or error each. Builds were cancelled and no Apps gate ran. These are historical results for the old candidate, not qualification of the newer C. Sanitized XML, provenance and adapted Dockerfile are under `superseded-38632023979/`; XML stdout/stderr is omitted and fixture credentials are redacted.

- [38633020732](https://github.com/sofianbll/bifrost/actions/runs/38633020732), wrapper `01aa5da809dd6e242dca373e4405087ecbdafd06`, completed with failure on all three runtime image probes. Their complete MCP mock/race suites, focused checks, Go/UI builds and package race/plugin builds passed. The wrapper's `VERSION=2.2.6+qualification.<full SHA>` becomes 61 characters after the binary adds `v`; `main.go:119` calculates a negative padding count and panics before `/health`. The fix only shortens the display SHA to 12 and asserts the stock banner limit; full candidate identity is retained separately. Sanitized artifacts and startup panic excerpts are in `version-failure-38633020732/`. Wire and Apps gates were skipped, not passed. This is the second wrapper repair, distinct from the C source repin.

The final sanitized bundle is under `green-38637028433/`: 67 result/capture files including 41 unmodified PNGs, plus the three adapted Dockerfiles, artifact metadata and the original/sanitized file hash index. Standalone and embedded console/host logs and JUnit stdout/stderr are omitted; fixture authorization/setup values are redacted. Original artifacts stay in `/private/tmp/bifrost-stack-green-38637028433/`. `manifest.json` records every tested commit/tree/image/version, gate results and route/checkpoint identities. Final local validation parsed 34 JSON and 9 JUnit XML files, verified 67 original/sanitized artifact hash entries and 94 bundle hash entries, and found no fixture-token or GitHub-token pattern in the retained text. The 149 successful wire assertions are counted from the candidate Newman executions; expected HTTP error-response requests do not constitute failed assertions.

| Leg | MCP path | Saved/read checkpoint | Restored view | Alignment after restore |
| --- | --- | --- | --- | --- |
| Corrected direct (bypasses Bifrost) | `/mcp` | `516f7334526e41b982` | `ae94e458281a48bdb7` | pass |
| Corrected through C | `/mcp/excalidraw` | `69c1e9e5770e4ffd83` | `e4dba46154e8481c9a` | pass |
| Official direct (bypasses Bifrost) | `/mcp` | `4abc461cf82349c6b2` | `6bb9166f35844bd681` | expected failure |
| Official through C | `/mcp/excalidraw-official` | `2c6ba909aee7480189` | `32fa8d0f69da485bb5` | expected failure |

All four legs preserve the fixed camera/1400×1050 SVG viewport and exact seven-element readback after the pointer edit. Corrected labels remain within approximately (+0.66, -0.34) scene units of box centers. Official labels shift by approximately (-29.34, -12.84), (-35.84, -12.84), (-49.84, -12.84), identically with or without Bifrost. Every leg rejects the deliberately displaced label (+24, -20 scene units). The official restore is not called fixed merely because the comparison gate correctly accepted its expected negative result. Progressive inputs are controlled official-host notifications, not live model streaming.

The separate inherited [Fork Docker image run 38637028474](https://github.com/sofianbll/bifrost/actions/runs/38637028474) failed on AMD64 in attempt 1 before stock Newman: stock `/mcp` exposed only four Code Mode tools after 180 seconds. MCP AMD64/ARM64 and the ARM64 build passed. Root retained private raw logs under `/private/tmp/bifrost-38637028474-attempt1-fixture/` (artifact 11930240423) and reran only failed jobs at the same SHA to characterize the catalog behavior. This report does not claim the retry succeeded. That red stock-readiness result is separate from clean C's passing wire/stock/App qualification and does not implicate corrected App behavior. No invariant, stock image digest or readiness guard was relaxed.

Root/heartbeat owns PR #18 readiness, the inherited workflow retry, tracker changes and merges. Root reported an automatic approval-review rejection of exporting new QA reports/captures because export may include internal CI data without explicit authorization. These local proof files and the result-comment publication remain held until root relays authorization. No alternate branch, tool or export route was used.

When export authorization is resolved by root, the prepared result comment on PR #18 must begin exactly `> *This was generated by AI during triage.*`, with the run link and the AMD64/reference-host limits. Root/heartbeat retains tracker-state, readiness and merge decisions. Both permitted wrapper repair cycles have been used (context availability, display-version length); repinning C after code review is a source change, not another wrapper failure. Any new failure would be a handoff to root, not authorization for another wrapper edit. Never dispatch `pr-tests.yml` or `run-core-tests.yml`, which include paid providers. Bifrost 2.3 and upstream submission remain outside this campaign.
