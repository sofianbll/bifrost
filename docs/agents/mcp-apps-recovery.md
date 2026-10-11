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

- [CI run 38166694381](https://github.com/sofianbll/bifrost/actions/runs/38166694381) tests `0ac9c547b348bc66d61c9f3983638a97ab000fb6`. Both MCP JUnit reports contain 699 cases, 29 skipped, zero failures and zero errors. Both architectures pass gateway/UI build, race checks, dynamic plugin compilation and the image probe.
- The deterministic AMD64 fixture passes all 37 MCP Apps and 2 Code Mode error requests, with exact execution counts and zero assertion failures. Six Newman request-level failures represent the intentionally tested HTTP 403 refusals; both runner exit codes are zero. The full/limited Virtual MCPs have distinct grants, JSON-RPC validation is scoped to MCP routes, and ten local harness-policy tests pass.
- Four real Apps pass all 79 protocol/API checks, including the native flagged Debug error and successful text-only nested script use. The total qualification is 79/80 because its separate reference-host render gate fails. The Debug package retains `Debug text content` while setting `isError`; looking for the word "error" was not a valid protocol assertion.
- All 16 render attempts and the checkpoint attempt fail at the wrapped Server-label selector, before tool calls. The four-connection UI check fails too, although its log records four distinct initialize/list sequences. There is no successful visual/checkpoint evidence. The next patch uses prefix label matching for Server, Tool and Input in every caller. Node syntax and renderer self-check pass; actual rendering requires the next CI run.
- [CI run 38168731546](https://github.com/sofianbll/bifrost/actions/runs/38168731546) at `3737338d8fc055ec505a951a8ab49a1005106249` repeats the green two-architecture MCP/image checks, 37+2 fixture cases and all 79 API checks. The four-connection browser check now passes. The remaining 16 renders and checkpoint stop on an immediate renderer TypeError: `.then()` was called after `await` had already returned an array, and the catch masked it as a discovery failure. The next one-line change uses the array's `includes()` and retains the original cause in diagnostics. The old expression throws in the local red/green check; the replacement returns the expected true/false values. Browser success still requires CI evidence.
- The prior cleanup stall is repaired: an actual local SSE regression checks client disconnect and proxy shutdown close upstream streams. CI now produces complete reports and removes its four containers, network, image and private files. The fixture artifact uploads immediately; fixture and real qualification have 10/20-minute limits.
- Local contribution builds passed on all three lower branches with generated stock 2.2.4 UI assets; the UI build/type check passed. Generated temporary assets, dependencies and binaries were removed. The original checkout remains clean at its preserved commit. All four new branches are pushed and track their fork remotes.
- The prior second-source absence is not diagnosed by granting both clients in the full Virtual MCP. Independent fresh SQLite checks reload both full-key relations with nonempty client IDs, and the actual governance store resolves access to both direct clients with only the first in a Virtual MCP. HTTP admission/tool-manager setup remains outside that reproduction. The old persistent-connection 403 is also unresolved.
- On October 11 the owner chose Codex Cloud for #3. The shared bootstrap now supports explicit `--codex` mode, failure status and platform/timing while preserving Claude behavior. Stub checks cover Codex/Claude modes, Go binary path lookup and existing gotestsum. No actual cloud task or environment was configured or published. CLI task listing worked but was empty; automatic approval rejected its environment selector because no precise environment selection was approved, and the browser requires sign-in.
- Three upstream PR bodies are prepared under `docs/agents/pr-drafts/`. No upstream PR, image tag, registry publication or deployment has been created. Actionlint 1.7.12 passes; shellcheck and pyflakes were unavailable.
- The owner authorized recoverable cleanup of the three lower worktrees. Native archive requests returned `queued`, and the directories remained present at verification. The chat was repinned. Cleanup remains unconfirmed until native artifact inventory shows archived worktrees.

## Completion gates

- [x] #4: stack assembled; each lower branch builds independently; old branch preserved; new branch names recorded on the fork.
- [x] #5–#7: local STDIO Apps fixture and exact wire cases for discovery, resources, native results, callbacks and permissions.
- [x] #8–#9: compatible errors, direct App calls with Code Mode, text-only scripts, documented limitations.
- [x] #10: empty-store grants red/green on the exact 2.2.4 base; focused race repetitions.
- [ ] #11: persistent-connection 403 reproduced and compared with stock upstream, or explicitly left unresolved with evidence limits.
- [x] #12: four real Apps protocol/API qualification passes in fork CI with exact package versions and redacted artifacts; visual gate remains #13.
- [ ] #13: real reference-host Playwright gate passes, including Virtual MCP, callbacks, direct calls with Code Mode, and connection-stall check.
- [ ] #3: actual Codex Cloud task, headless browser launch and cold/warm startup timing verified. The owner selected Codex instead of Claude; a bootstrap self-check alone cannot establish these.
- [ ] #14: approved release image, explicit Pulsar deployment, owner-selected dogfood period, final Codex render/edit/partial-input check, then upstream PRs.

Do not close a ticket merely because its code was ported. Record implemented, tested locally, CI-qualified and released as separate states. Production deployment and several days of dogfooding remain a separate owner gate; no tag or deployment is implied by a branch push.
