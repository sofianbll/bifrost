# PR 15 CI recovery — 2026-10-14

Scope: fork GitHub settings, test fixtures and isolated CI qualification only.
Production Bifrost, the public Excalidraw service, authentication and user stores
are unchanged. Original checkout work is preserved.

## Initial observed failures

At `cbe42774d2d1430e9b594201e42f6d6c30242474`,
[run 38473730002](https://github.com/sofianbll/bifrost/actions/runs/38473730002)
passed both native MCP test jobs. Its AMD64 image and Go checks passed, but the
stock comparator did not obtain its complete tool catalog. Real-App wire
qualification passed; reference-host rendering passed 17/18 and rejected the
public App's shifted restored labels. ARM64 failed
`TestSQLite_VKMCPConfig_Reconciliation` while updating the fixture's VK hash
with `database is locked`. Publication was skipped.

Dependency Review initially returned HTTP 403 with its dependency-graph
prerequisite disabled. The fork's **Dependency graph** was enabled in
Settings → Advanced Security; the UI confirmed the saved setting, and the API
then returned HTTP 200 with an empty dependency diff. Its
[second attempt](https://github.com/sofianbll/bifrost/actions/runs/38473757098)
passed. No security check was removed or made optional. Dependabot updates,
authentication and secret protection settings were not changed.

## Changes under test

- The fork has zero registered CodSpeed runners. Restrict the managed CodSpeed
  job to `maximhq/bifrost`, retaining the fork's hosted build, race and MCP tests.
  The previously queued fork-only benchmark run was cancelled. This does not
  claim performance measurements on GitHub-hosted hardware.
- Isolate the reconciliation test's setup from background catalog/credential
  sweeps. Stop and join the catalog worker, then cancel and signal the credential
  sweeps before its database edits; leave normal
  connection-pool behavior and gateway/store implementation unchanged. A first
  local attempt with one connection was rejected after its diagnostic stack
  showed an existing store transaction reading through another connection.
  The final fixture passes 30 repetitions with `-race` in 11.972 seconds.
- Build actual Excalidraw source from reviewed commit
  `61bb966aa2ceee485aaa3badd91807c2edc6f236` with its frozen lockfile. Run its
  standalone seven tests and the actual-function browser checks. Start its
  bundled HTTP server as a fourth isolated Docker sidecar; the three existing
  pinned npm Apps are unchanged. The public App is not used by this qualification
  target and is not modified.
- Record App repository/commit plus source and built HTML hashes alongside
  the gateway identity. The checkpoint assertions, negative control, camera and
  screenshot crop remain unchanged. Actual-pointer edit and exact save/read
  remain required. New self-checks pin both Excalidraw route variants to the
  isolated sidecar.

## Observed qualification

At `c3e8e25610c8de82bc2e9366e1bbd979080a8991`,
[run 38478832332](https://github.com/sofianbll/bifrost/actions/runs/38478832332)
passed both architectures' MCP, Go race, UI build, dynamic-plugin and image
checks. The unchanged stock comparator passed: nine named red sentinels and
seven green invariants matched its immutable image. The earlier discovery
timeout was not reproduced; its cause is not established.

Real-App wire checks passed **80/80**, rendered checks **18/18**, and the
actual Excalidraw normalizer **10/10**, with seven standalone App tests passing.
All six checkpoint assertions passed, including exact save/read, constant
framing, before/after label alignment and rejection of a deliberate +24/-20
text displacement. [Sanitized reports, build identities, checkpoints and three
SVG-only captures](ci-pr15/manifest.json) retain this run's evidence. Full HTML,
private files and raw console dumps are excluded.

Dependency Review passed on this commit. CodSpeed was skipped on the fork as
intended. Workflow Lint found four unrelated harness hostnames that do not
represent runner requests: Gemini/Azure documentation links, an MCP URL passed
to Anthropic for server-side access, and a reserved `.example` OAuth callback.
Their existing classification list now records those uses; no network allowlist
is widened. The egress check passes locally (eight apt jobs and two harness jobs),
as do all 48 version-format checks and the CI/fixture/image-identity self-checks.
The final lint correction remains subject to CI on its own commit.

## Review and validation boundary

Independent `/code-review` axes, fixed at `cbe42774d2d1430e9b594201e42f6d6c30242474`:

- **Standards:** no code blocker or supported design smell. One documentation
  finding corrected above: only catalog cleanup joins its worker; credential
  sweep stop methods cancel/signal without joining their goroutines.
- **Spec:** no functional defect or scope expansion found. The same wording
  finding was corrected; protocol, rendering and negative-control criteria remain
  unchanged. Native Codex acceptance remains unexecuted.

Local checks also include Python syntax, whitespace and the 30-repetition focused
SQLite race loop. Automated reference-host validation and visual inspection of
its saved captures are separate from native Codex, which is **not run**. No image
tag, registry publication, merge or release is authorized by these changes.
