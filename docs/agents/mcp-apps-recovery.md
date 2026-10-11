# MCP Apps recovery — 2026-10-11

This is the fork's recovery ledger. The authoritative scope is [spec #1](https://github.com/sofianbll/bifrost/issues/1) and its existing tickets #2–#14. It is not a new feature request or proof of release.

## Preserved starting point

- Original checkout: `feature/mcp-apps-native` at `71174d96ae4763e1167f9acdb636df20e704bb41`, clean, one commit ahead of its remote.
- That last local commit contains the cloud-session setup; it is preserved on the old branch and carried into the fork-only layer.
- The fork had two remote branches and no pull requests when checked on 2026-10-11. The prototype branch exists only locally.
- New contribution base: `transports/v2.2.4^{commit}` = `ed8371a9779bfbc8aa689d4d77964cf8ce9308bf`. The annotated tag object is not the source commit. `go.work` is generated locally and is not tracked upstream.
- No production data, deployment, authentication or existing branch was changed by this recovery.

## Branch ownership

| Branch | Purpose | Fork-only material allowed |
| --- | --- | --- |
| `codex/mcp-errors-224` | Script-error result flag, preserving upstream Chat tool-error values | No |
| `codex/mcp-firstboot-224` | Empty-store Virtual MCP and VK reconciliation, with 2.2.4 governance ordering | No |
| `codex/mcp-apps-relay-224` | Native Apps relay, deterministic fixture, wire regression cases and MCP docs | No |
| `codex/mcp-apps-fork-224` | Integration, image/cloud tooling and external-server/render gates | Yes |

The four branches are assembled as a stack and exist on the fork. The three contribution branches compile independently with the generated, identical 2.2.4 UI assets. Their existence and unit checks do not establish App rendering or release readiness.

## Accepted decision

On 2026-10-11 the owner chose the compatible Chat error behavior: a nested tool error remains a value available to the script. A script execution failure marks the outer `executeToolCode` result as `isError`. Native Go handler exceptions retain the upstream runtime-failure behavior; remote protocol-marked MCP errors remain script values. The stale fatal-error wording in #8/#9/#1 must be read together with this decision; do not change upstream Chat tests to make fatal tool errors pass.

Native App results still preserve their own `isError`, `content`, `structuredContent` and `_meta`. A Code Mode script returns text and opens no App view. Progressive drawing remains a host responsibility.

## Evidence collected before implementation

[CI run 37288704360](https://github.com/sofianbll/bifrost/actions/runs/37288704360) at `9f064fa1f`: image/build checks passed on AMD64 and ARM64; MCP tests failed on both and publication was skipped. Downloaded JUnit reports each contain 698 test cases, 29 skipped and the same 7 failures (five failing leaf cases plus their two parents), all in the Code Mode error-server scenarios.

Three local Claude session logs from October 1–5 were inspected for this project. They confirm the existing specification, CI work and the unpushed cloud setup. Old 2.2.3 screenshots and QA reports remain historical evidence; they are not qualification of this 2.2.4 stack.

## Current verification

- [CI run 38161344427](https://github.com/sofianbll/bifrost/actions/runs/38161344427) tests code at `b99427f3d49558fdc5c312ffbaa672163a40a228`. Both MCP JUnit reports contain 699 cases, 29 skipped, zero failures and zero errors. Both architectures pass the gateway/UI build, race checks and dynamic plugin/image probe.
- The AMD64 fixture failed: 35 of 37 selected MCP Apps requests executed, plus both Code Mode error requests. The collection's generic LLM content assertion rejected JSON-RPC envelopes; six HTTP 403 cases also inherited a conflicting global HTTP 200 expectation. Aggregate second-source discovery/calls and limited-key expectations failed too. These separate harness and access/configuration defects must be resolved before qualification.
- Real-server qualification recorded 78 passing API probes and one failing nested Debug tool-error assertion. The existing Excalidraw API probe passed. The browser then stalled after reference-host discovery until the job timeout cancelled AMD64 at 18:51 UTC. Only one console report was uploaded, with no tool-call/render result; the 16 renders, checkpoint round-trip and four-connection check are unverified. The ARM64 build and both MCP test jobs passed; publication was skipped.
- Local contribution builds passed on each branch with generated stock 2.2.4 UI assets; `npm run build-enterprise` passed build and type checking. Temporary build assets, dependencies and binaries were removed afterwards.
- The local runner fixes select each Code Mode client's advertised native tool and locate the reference host's App frames without CSS Module class names. The mapping regression was observed red, then the Python/Node self-checks passed. They are not yet qualified by the current remote run.
- The runner retains redacted early failures, gateway logs and the harness quiet log. The real early-failure artifact path and self-check pass without Docker or harness execution. The next fixture step uploads diagnostics immediately, enforces actual Newman execution counts, and has a 10-minute limit; real qualification has a 20-minute limit. The count guard rejects the observed 35/37 report. App gates use `!cancelled()` while report upload retains `always()`; actionlint 1.7.12 passes (shellcheck and pyflakes were unavailable). The cloud setup logic self-test also passes; an actual Claude cloud run is still required.
- The next qualification repair recognizes JSON-RPC only on MCP routes, tags the six 403 cases, and separates full/limited Virtual MCP grants. Ten focused harness-policy tests pass and all 37 fixture requests remain selected. The full Virtual MCP explicitly includes both clients; it does not diagnose why direct full-key grants alone did not expose the second source. A fresh SQLite loader test reloads both full-key MCP relations correctly, including nonempty client IDs, so loader persistence is not an established cause.
- The pinned Debug package sets `isError` while retaining `Debug text content`; the old word-matching assertion was incorrect. The replacement verifies a native flagged error and successful text-only script use of that result. A local SSE regression verifies client disconnect and proxy shutdown close upstream streams. The renderer now closes owned browser contexts on form failure and active proxy connections during cleanup, with bounded form actions. Python/Node self-checks pass; these changes still require actual CI qualification.
- Three upstream PR bodies are prepared under `docs/agents/pr-drafts/`. No upstream PR, image tag, registry publication or deployment has been created.
- The owner explicitly authorized recoverable cleanup of the three contribution worktrees. Native Codex archive requests returned `queued`; the directories remained present at verification. The chat was repinned. Treat cleanup as unconfirmed until the native artifact inventory shows archived worktrees.

## Completion gates

- [x] #4: stack assembled; each lower branch builds independently; old branch preserved; new branch names recorded on the fork.
- [ ] #5–#7: local STDIO Apps fixture and exact wire cases for discovery, resources, native results, callbacks and permissions.
- [ ] #8–#9: compatible errors, direct App calls with Code Mode, text-only scripts, documented limitations.
- [x] #10: empty-store grants red/green on the exact 2.2.4 base; focused race repetitions.
- [ ] #11: persistent-connection 403 reproduced and compared with stock upstream, or explicitly left unresolved with evidence limits.
- [ ] #12: four real Apps qualified in fork CI with exact package versions and redacted artifacts.
- [ ] #13: real reference-host Playwright gate passes, including Virtual MCP, callbacks, direct calls with Code Mode, and connection-stall check.
- [ ] #3: actual Claude cloud session, headless browser launch and cold/warm startup timing verified. The setup script alone cannot establish these.
- [ ] #14: approved release image, explicit Pulsar deployment, owner-selected dogfood period, final Codex render/edit/partial-input check, then upstream PRs.

Do not close a ticket merely because its code was ported. Record implemented, tested locally, CI-qualified and released as separate states. Production deployment and several days of dogfooding remain a separate owner gate; no tag or deployment is implied by a branch push.
