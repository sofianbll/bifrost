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

These branches are being assembled as a stack. Their existence alone does not mean that builds, full CI or release checks have passed.

## Accepted decision

On 2026-10-11 the owner chose the compatible Chat error behavior: a nested tool error remains a value available to the script. A script execution failure marks the outer `executeToolCode` result as `isError`. The stale fatal-error wording in #8/#9/#1 must be read together with this decision; do not change upstream Chat tests to make fatal tool errors pass.

Native App results still preserve their own `isError`, `content`, `structuredContent` and `_meta`. A Code Mode script returns text and opens no App view. Progressive drawing remains a host responsibility.

## Evidence collected before implementation

[CI run 37288704360](https://github.com/sofianbll/bifrost/actions/runs/37288704360) at `9f064fa1f`: image/build checks passed on AMD64 and ARM64; MCP tests failed on both and publication was skipped. Downloaded JUnit reports each contain 698 test cases, 29 skipped and the same 7 failures (five failing leaf cases plus their two parents), all in the Code Mode error-server scenarios.

Three local Claude session logs from October 1–5 were inspected for this project. They confirm the existing specification, CI work and the unpushed cloud setup. Old 2.2.3 screenshots and QA reports remain historical evidence; they are not qualification of this 2.2.4 stack.

## Completion gates

- [ ] #4: stack assembled; each lower branch builds independently; old branch preserved; new branch names recorded on the fork.
- [ ] #5–#7: local STDIO Apps fixture and exact wire cases for discovery, resources, native results, callbacks and permissions.
- [ ] #8–#9: compatible errors, direct App calls with Code Mode, text-only scripts, documented limitations.
- [ ] #10: empty-store grants red/green on the exact 2.2.4 base; focused race repetitions.
- [ ] #11: persistent-connection 403 reproduced and compared with stock upstream, or explicitly left unresolved with evidence limits.
- [ ] #12: four real Apps qualified in fork CI with exact package versions and redacted artifacts.
- [ ] #13: real reference-host Playwright gate passes, including Virtual MCP, callbacks, direct calls with Code Mode, and connection-stall check.
- [ ] #3: actual Claude cloud session, headless browser launch and cold/warm startup timing verified. The setup script alone cannot establish these.
- [ ] #14: approved release image, explicit Pulsar deployment, owner-selected dogfood period, final Codex render/edit/partial-input check, then upstream PRs.

Do not close a ticket merely because its code was ported. Record implemented, tested locally, CI-qualified and released as separate states. Production deployment and several days of dogfooding remain a separate owner gate; no tag or deployment is implied by a branch push.
