# Bifrost 2.2.3 — validation de notre intégration MCP Apps

Architecture overview for reviewers: [MCP Apps component diagram](mcp-apps-architecture.md).

## Current qualification status — 2026-09-27

This section supersedes the current-status statements in the historical campaigns
below. Their results remain evidence for the revisions they tested, not for every
later change. The September 27 review inspected the repository, saved reports,
screenshots and conversation history. Focused tests and a new native ARM64
reference-host trial then ran as described below; the full September 25 campaigns
were not rerun.

### Excalidraw plugin resource failure — 2026-09-27

The architecture diagram initially did **not** render in the selected remote
Bifrost plugin. At that stage, only the standalone SVG export was visually verified.

Codex's desktop log records `resources/read` failures for
`ui://excalidraw/mcp-app.html`. A separate read-only probe of the configured
dedicated gateway endpoint found conflicting tool metadata: `_meta.ui.resourceUri`
used the namespaced `ui://bifrost/...` URI, but `_meta["ui/resourceUri"]` retained
the upstream URI. Reading the former returned HTML with MIME
`text/html;profile=mcp-app`; reading the latter returned JSON-RPC `-32002`
(`resource not found`). This confirms a gateway defect matching the host's
failed URI; it does not establish the selected connector's cache state.

The local fix rewrites the existing compatibility field in `rewriteMCPAppTool`
alongside the nested field. It preserves per-source routing and the existing
resource authorization path. `TestMCPAppLegacyResourceURIMatchesNativeRoute`
reproduced the error before the fix and passes afterward: both advertised fields
resolve through the gateway, including two sources sharing one upstream URI.
The smoke probe now expects this equality; historical reports' assertion that
the raw compatibility URI was preserved was insufficient and is superseded.
The complete HTTP handler package passed locally with `go test -race` (59.358s);
both review axes reported no actionable finding. The Python smoke script's syntax
was checked, but its full live scenario was not rerun against production.

**Published and registry-verified:** `ghcr.io/sofianbll/bifrost:2.2.3-sofian.3`,
source `6370fa0221e73c35cf479da101bb6aec8ae44ae9`, for Linux AMD64 and ARM64.
The [release workflow](https://github.com/sofianbll/bifrost/actions/runs/36337890059)
passed the UI build, Go race suites, runtime image tests and publication on both
architectures. Immutable image reference:
`ghcr.io/sofianbll/bifrost@sha256:4deff429c615077ed6f7e8beeef254c5719cb5cc8814c8ddd085e964a1f5af3a`.
The previous `.2` image lacks this fix and remains available for rollback.
The agent did not deploy the image; the user subsequently reported installing it.

**Post-update host retest:** the dedicated gateway now advertises identical
namespaced URIs in both metadata fields and serves the App HTML. The remote
Bifrost plugin still requested the old URI and failed. The user clarified that
the intended connection was the locally configured `codex-bifrost` MCP server.
Through that connection, the same architecture diagram produced checkpoint
`7ce4b433891c41d197`. Codex logs at 18:30:14–18:30:19 UTC record a successful
resource read and `widget_running` for `server=codex-bifrost`; the user explicitly
confirmed seeing the actual blocks and arrows. This verifies rendering through
`codex-bifrost`, not through the separate remote plugin. The remote plugin's
retained URI remains unexplained; no cache or connection settings were changed.

### Fork version and rebuilt UI — 2026-09-27

Release source: `1fdc8b8700300f5d443939bb8f3fd95cbbb2db39`, including MCP fixes
in `50e9bd199`, version/UI fixes in `08551c19b`, and the test-fixture repair below.
Tag: `image/2.2.3-sofian.2`. Enterprise navigation remains unchanged.

**Published and registry-verified:** `ghcr.io/sofianbll/bifrost:2.2.3-sofian.2`
for `linux/amd64` and `linux/arm64`. Embedded version:
`v2.2.3+sofian.2.1fdc8b870030`.

Immutable image reference:
`ghcr.io/sofianbll/bifrost@sha256:171f51637fb8668361aada6edb42aecf5cac06d483eb18f44d09396b1a3f1671`.

- `npm ci`, UI build, TypeScript checks and embedding completed locally.
- Version regression: four assertions failed before the fix; all six pass after
  ignoring build metadata. No added dependency.
- Workflow identity test passes for the versioned release tag, legacy custom tag,
  dev branch, mismatched upstream base and invalid Docker tag.
- Chromium E2E on an isolated providerless gateway passed: same-base fork shows
  no update card; simulated upstream `v2.2.4` shows the update card. API version
  and upstream release responses were intercepted for these two scenarios.
- Browser screenshots: [same base](qa/bifrost-2.2.3/fork-no-false-update-2026-09-27.png)
  and [newer upstream](qa/bifrost-2.2.3/upstream-update-2026-09-27.png).
- Two review axes (standards/spec) reported no actionable findings in the MCP fixes;
  release/version diff review reported no material issue.
- [Linux publication workflow](https://github.com/sofianbll/bifrost/actions/runs/36329986512)
  attempt 1 passed ARM64 but failed AMD64 in the existing
  `TestSQLite_VKMCPConfig_Reconciliation` (`database is locked` while updating
  the VK hash). MCP, schemas, handlers and server packages passed on AMD64.
  One failed-job-only retry reproduced the same failure. A test-only pool-cap
  attempt was rejected after exposing a nested-connection deadlock in fixture
  creation. The final repair writes only the fixture's `config_hash` column with
  an atomic UPDATE and requires exactly one affected row. It retains every
  reconciliation assertion and changes no production SQLite setting. The focused
  test passed 20 consecutive runs with `-race` (7.655s). The new immutable image tag
  `2.2.3-sofian.2` carries this test-only repair; `.1` remains the failed candidate.
- [Release .2 workflow](https://github.com/sofianbll/bifrost/actions/runs/36331623306)
  completed successfully on commit `1fdc8b8700300f5d443939bb8f3fd95cbbb2db39`.
  Both native Linux jobs passed the UI build/version tests, Go race suites for
  MCP, schemas, handlers, server and lib, and runtime image checks including
  health, UI assets and dynamic plugin loading. The tested images were published
  without rebuilding. A separate registry read confirmed the digest above and
  both platform manifests. This does not replace the earlier visual four-App
  qualification or remove the documented script-to-App limitation.
- Pulsar/production was not changed.

### Corrected native candidate: live qualification — 2026-09-27

**The corrected macOS ARM64 binary passed 35/35 focused HTTP/MCP checks.** The
previous local binary passed 15/35 with the same checker. Each run started once
against its own empty database and used the installed official
`@modelcontextprotocol/server-debug` 2.0.3 package over stdio. No LLM provider was
called. Counts are assertions within one focused scenario, not 35 independent
application journeys.

- First boot: both file-declared Virtual MCP/VK attachments exist without restart.
- Dedicated `/mcp/debug`, named Virtual MCP `/mcp/candidate-apps`, and root `/mcp`:
  discovery, App HTML resources, native structured results and native errors pass.
- On all three routes, runtime and nested upstream failures return `isError=true`
  without a success announcement; simple and nested successful scripts still pass.
- Ungranted keys are rejected on dedicated/named routes and cannot discover Debug
  on the root route.

The [checker](../tests/mcpapps/qualify_candidate.py) creates synthetic keys,
isolates the subprocess environment from provider credentials, and stops its own
gateway process group after testing. Both runs stopped successfully; ports 18095
and 18096 were verified free afterwards. Existing QA and production instances were
not replaced.

Evidence: [baseline](qa/bifrost-2.2.3/errors-firstboot-baseline-2026-09-27.json),
[candidate](qa/bifrost-2.2.3/errors-firstboot-candidate-2026-09-27.json), and
[build manifest](qa/bifrost-2.2.3/errors-firstboot-manifest-2026-09-27.json).
The manifest binds the binary to base `c3f711b49`, the exact uncommitted Go source
patch and the checker hash. Build inputs were checked unchanged after compilation.

Binary: `/private/tmp/bifrost-candidate-20260927.WBFMzi/bifrost-http`.
SHA-256: `4106af5a5b681862798360b34b6051ecd71928118c2391caa18e98376ac1d325`.

This was a native MCP regression candidate. That trial did not compile or
visually qualify the dashboard UI or produce a Linux Docker image. It does
not requalify the four-App visual campaign or add support for opening Apps from
scripts. The source fixes are now committed in `50e9bd199`; the running long-lived
QA instance still uses its earlier binary. See the release section above for the
subsequent UI and Linux qualification.

### Code Mode errors: local source correction — 2026-09-27

The working tree now preserves nested tool failures after plugin post-hooks and
propagates the final `ChatToolMessage.IsError` into the MCP `tools/call` result.
An empty failed upstream response also receives an error message instead of the
old success fallback. This changes error signalling only; scripts still do not
open new App views or preserve App structured results.

Two regression tests failed before the correction: a nested in-process MCP tool
returned `isError=true` but the script reported `Execution completed successfully`,
and the gateway omitted the final error flag. Both passed after the correction.
Controls cover successful calls, plugin rejection and recovery, and empty errors.
The complete Starlark and HTTP handlers package suites passed with the race
detector (1.379 s and 59.525 s respectively):

```bash
go test -race ./core/mcp/codemode/starlark ./transports/bifrost-http/handlers -count=1
```

The source changes are committed in `50e9bd199` and have not been loaded into the
running QA gateway. Publication status is recorded above. The candidate-binary
observations in the table below remain historical
evidence for that binary, including its old error-signalling defect.

The provider harness contains three optional `codemode-errors` HTTP cases (runtime
failure, nested Debug failure, successful control). Augmentation and filtering
retain all three. Live provider-harness execution remains a manual release check;
the default provider profile must first contain a Code Mode client named `debug`
using official `server-debug` 2.0.3 with server-level Starlark bindings. These cases
need no LLM provider. On an isolated checkout, with port 8080 free:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
make dev APP_DIR=$(pwd)/tests/integrations/python
# Once the configured gateway is healthy, in another terminal:
make run-provider-harness-test FEATURE=codemode-errors INCLUDE_PREVIEW=1 PARALLEL=0
```

### Virtual MCP first boot: local source correction — 2026-09-27

Startup now reconciles `mcp.virtual_mcps` after `loadGovernanceConfig`, while MCP
clients still load before governance. This allows both direct MCP grants and
Virtual MCP/VK attachments to resolve during the first load. The existing merge,
hash, pruning and access policies are unchanged. No schema or deployed database
was changed.

`TestLoadConfig_VirtualMCPFirstBoot` uses a fresh temporary SQLite store through
the real `LoadConfig` entry point, in both `split` and `config.json` modes. Before
the fix, only each first-load VK attachment assertion failed; the restart passed.
After the fix, both first loads and restarts pass, including direct MCP grants.
The full `lib` package suite passed with the race detector in 72.506 s; after
sharing the final fixture with the HTTP harness, the focused race run also passed
in 4.169 s. `git diff --check` passed.

```bash
go test -race ./transports/bifrost-http/lib -count=1
go test -race ./transports/bifrost-http/lib -run '^TestLoadConfig_VirtualMCPFirstBoot$' -count=1
```

The [fixture](../tests/mcpapps/firstboot/config.json) is shared with the optional
`virtual-mcp-firstboot` provider-harness case. Its key is synthetic and its source
MCP client is disabled: the check covers persisted associations, not a rendered
App or an upstream connection. Harness augmentation and filtering retain the
single GET assertion. The live harness was not run.

For the manual HTTP release check, port 8080 must be free. Start exactly once
against a fresh temporary directory; a restart would mask the original defect:

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
qa_firstboot_dir=$(mktemp -d)
cp tests/mcpapps/firstboot/config.json "$qa_firstboot_dir/config.json"
make dev APP_DIR="$qa_firstboot_dir"
# Once healthy, in another terminal:
make run-provider-harness-test FEATURE=virtual-mcp-firstboot INCLUDE_PREVIEW=1 PARALLEL=0
```

This fix is committed in `50e9bd199` and remains undeployed on Pulsar. The
historical cold-start failure below remains evidence for the earlier binary.

### Previously tested candidate surfaces

This table describes earlier binaries. The release status above supersedes
their first-boot and error-signalling defects; their raw results are preserved.

| Surface | Evidence | Remaining limit |
| --- | --- | --- |
| Published fork `7c4191ee8` | [Workflow 36133668902](https://github.com/sofianbll/bifrost/actions/runs/36133668902): AMD64, ARM64 and publication jobs succeeded; GitHub checked September 27 | This image predates the aggregate/resource corrections in `c3f711b49` |
| Pulsar deployment | September 25 conversation recorded `mcp-apps-2.2.3-1`, version `v2.2.3-7c4191ee826f`, health and UI HTTP 200 | Historical observation; live production state was not rechecked September 27 |
| Local candidate `c3f711b49` | Aggregate App admission, resource-array normalization, callbacks and native debug logs implemented | Remote feature branch still pointed to `7c4191ee8` when checked September 27; candidate publication/deployment not established |
| Extended candidate protocol checks | [ARM64](qa/bifrost-2.2.3/mcp-apps-candidate-extended-arm64-2026-09-25.json) and [AMD64](qa/bifrost-2.2.3/mcp-apps-candidate-extended-amd64-2026-09-25.json): **46/47 each** | Both fail the nested Code Mode structured-result expectation; these are overlapping scenarios, not 94 distinct tests |
| Dedicated App rendering | User-confirmed Codex rendering in prior conversations; reference-host [render](qa/bifrost-2.2.3/mcp-apps-reference-host-render-2026-09-25.png) and [edit](qa/bifrost-2.2.3/mcp-apps-reference-host-edit-2026-09-25.png) captures inspected | Does not prove aggregate Virtual MCP rendering in Codex on the latest candidate |
| Normal mode: direct, named Virtual MCP and root `/mcp` | September 27 isolated campaign: **124/124 protocol assertions** after startup reconciliation; user-confirmed native Codex rendering on all three routes; reference-host captures show direct and combined rendering | First-boot association loss affected this earlier binary and is fixed in `.2`; the saved reference-host Virtual MCP capture is blank; in-place modification, visual edit/save and progressive drawing remain separate gates |
| Code Mode enabled, App called directly | Four Apps x three routes preserve discovery, HTML and structured results; all four render in the reference host. Native Codex screenshot confirms Monitor, Debug and Excalidraw; PDF native rendering now verified after the local upstream-viewer workaround below | Stock server-pdf 2.0.3 has a CSP compatibility defect in Codex; the successful native PDF retest uses a patched QA package, not stock upstream |
| App called inside `executeToolCode` | Four Apps x three routes execute but return text without structured content/App metadata; Debug upstream errors lose their marker | Creating Apps from scripts and reliable nested error signalling remain unqualified; PDF interaction with an existing view works after timeout adjustment |
| Progressive drawing | Persistent-connection A/B protocol checks recorded in [realtime research](mcp-apps-realtime-research.md) | No same-host capture of partial-input events establishes progressive rendering |

The extended reports record selected observations and assertions, not full
redacted request/response traces or an image digest. Their source-version binding
is therefore weaker than the initial campaign manifest. The screenshots document
visual states; the prior conversation additionally reports zoom, save and readback,
which the screenshots alone cannot prove.

At the start of this review, the extended smoke and handler tests were modified
but uncommitted, and the two extended reports and screenshots were untracked.
Do not discard them or present the older 23/23, 27/27 and 5/5 summaries as exhaustive
qualification. Provider calls, unrelated monorepo features and production changes
remain outside this validation scope.

Release gates: reconcile these test artifacts with a reviewed revision; complete
interactive aggregate validation against an identified candidate; settle and test
the nested Code Mode contract; then publish that exact candidate before considering
a separate production rollout. A successful protocol check is not a rendering check.

### PDF warning in native Codex: cause and verified local workaround — 2026-09-27

**Resolved in the local QA PDF package; Bifrost and Codex security policy unchanged.**
Codex logs at 14:01:45 UTC show the bundled PDF.js worker being rejected by
`script-src` because it uses a `data:text/javascript;base64,...` URL. Both the
worker and PDF.js fallback fail, yielding the warning shown by the user.
[Redacted native errors](qa/bifrost-2.2.3/pdf-native-csp-error-2026-09-27.txt).
The byte-read callback succeeded and command polling continued; this is distinct
from the earlier 30-second long-poll timeout.

The exact HTML read through aggregate Bifrost equals the upstream package file
(SHA-256 `3c8aa8ca4d27bf20429b8b3a8dd6521340594cc1c1cbf69f50e3d35908625be7`).
Thus the incompatible worker URL originates in `@modelcontextprotocol/server-pdf`
2.0.3, not in gateway rewriting. The reference host allowed `data:` scripts, which
explains why its earlier rendering passed while native Codex failed.

**Reproduction and regression:** an isolated copy of the reference host on
18092/18093 omits `data:` from its script policy (the relevant restriction observed
in Codex, not a complete Codex emulator). The unchanged viewer reproduces the
same warning and `Setting up fake worker failed` error. The [browser assertion](qa/bifrost-2.2.3/pdf-viewer-check-2026-09-27.mjs)
`assertPdfRendered(tab)` failed before the change and passed afterwards, requiring
both `of 15` and `Attention Is All You Need` in the rendered DOM and rejecting the
worker error. [Before](qa/bifrost-2.2.3/pdf-csp-before-2026-09-27.png) /
[after](qa/bifrost-2.2.3/pdf-csp-after-2026-09-27.png).

The [reproducible QA patch](qa/bifrost-2.2.3/patch-pdf-worker-2026-09-27.py) replaces
only the embedded worker URL construction with a Blob URL containing the same
bytes. It checks the original HTML hash and preserves a backup. The existing
policy already permits `blob:`; no CSP expansion or security bypass is applied.
Only the temporary `qa-pdf` server was reconnected to clear its HTML cache.
The patched HTML hash is
`61000177ba3cd2e212e47e858950bb068e9fdc2f9a46d9e20f632527088f817f`.

**Native verification:** fresh chat `01a0e334-bc94-7311-897d-4712f98da657` called
`pdf-display_pdf` once through `qa-code-apps`, with Code Mode enabled. Its initial
state request preceded the view mounting and returned `Viewer never connected`;
that was not the final result. After opening the completed chat, the same view
`042e0e73-54bb-402c-b689-863ad061914b` returned `pageCount=15`, `currentPage=5`,
`zoom=93`, `displayMode=fullscreen`. Its `get_screenshot(page=5)` callback returned
an image, which was inspected and saved as
[native PDF page 5](qa/bifrost-2.2.3/pdf-native-page5-after-2026-09-27.png).
This is a capture produced by the PDF App running in Codex, not a screenshot of
the entire Codex window. The old chat retained the pre-fix resource; its cached
failure must not be confused with this newly loaded, successful view.

This is a reversible local compatibility workaround for the upstream example.
No Bifrost product code, deployment, production instance, or installed Codex app
was changed. A stock package reinstall removes the workaround. It does not fix
the separate nested Code Mode output/error contract.

### Four Apps with Code Mode — 2026-09-27

The [four-App matrix](qa/bifrost-2.2.3/four-apps-code-mode-2026-09-27.json)
uses the same identified Darwin ARM64 binary (`c3f711b49`, SHA-256
`90a867bfb796e33bd77d069ecd16c7822ed5822546a066bc085dab6c1cbee4df`).
`go version -m` reports that revision and `vcs.modified=true`; this is a local
candidate, not a clean published release. System Monitor, PDF and Debug use the
installed official 2.0.3 stdio packages; Excalidraw uses its hosted HTTP endpoint
(the hosted revision is not established). All four client configurations explicitly
set `is_code_mode_client=true`, with server-level Starlark binding. No LLM provider
or production instance participates.

Two paths are deliberately qualified separately:

| Path | Four Apps x three routes | Reference-host visual result |
| --- | --- | --- |
| Native App tool while Code Mode is enabled | 12/12 calls preserve structured content; 12/12 expose App HTML/MIME and the Code Mode helper | All four rendered on combined `/mcp` |
| App tool inside `executeToolCode` | 12/12 scripts execute; **0/12 preserve outer structuredContent or App metadata** | A standalone Monitor script produced text only, with no App iframe (DOM count 0) |

Routes: dedicated `/mcp/<client>`, named Virtual MCP `/mcp/codeapps`, and root
`/mcp` with direct + Virtual MCP grants. Counts are overlapping observations,
not independent full application journeys. The protocol matrix lives in the JSON;
reference-host captures show [Monitor](qa/bifrost-2.2.3/code-mode-monitor-render-2026-09-27.png),
[PDF](qa/bifrost-2.2.3/code-mode-pdf-script-page2-2026-09-27.png),
[Debug](qa/bifrost-2.2.3/code-mode-debug-render-2026-09-27.png) and
[Excalidraw](qa/bifrost-2.2.3/code-mode-excalidraw-render-2026-09-27.png).
Monitor callbacks returned changing samples on all three routes. Debug connected,
received tool input/result events and retained result `_meta` in the reference host.
Its callback RPC succeeded; two UI automation attempts to activate its callback
button timed out, so this campaign does not claim that button was exercised.
Partial-input events were not observed in these complete-input calls.

**Confirmed script-path defects/limits:**

- `extractTextFromMCPResponse` in `core/mcp/codemode/starlark/utils.go` extracts
  content blocks into text; structured content and result metadata are not carried
  through. `executeToolCode` then formats that as a textual response. Monitor data
  survive because the upstream also emits JSON text; this does not preserve the
  original MCP Apps response contract. PDF/Excalidraw identifiers may survive in
  text without creating a corresponding displayed view.
- Debug `simulateError=true` preserves native `isError=true` on all three routes,
  but the script says **Execution completed successfully**, with no outer error
  flag. The nested code carries the flag into a ChatMessage, but
  `extractResultFromChatMessage` reads content and ignores `ChatToolMessage.IsError`.
- A root script with the denied VK cannot access Monitor (`undefined: monitor`,
  no available servers). **No data access was observed**, but this runtime error
  also lacks the outer MCP `isError` flag: `mcpserver.go` returns
  `mcp.NewToolResultText(resultText)` for the non-native helper path. This is an
  error-signalling defect, not a demonstrated permission bypass.

**PDF interaction and timeout:** the viewer's idle poll holds a request for 30 s,
which collides with Bifrost's default 30 s tool timeout. Logs showed timeout and
`isError`, and the upstream viewer permanently stopped polling on that result.
The initial script navigation was queued but not displayed. The temporary PDF
client alone was changed to **60 s**, through the runtime API and fixture config.
An idle poll then returned normally after **30.03 s**. A new native PDF view was
opened once; `result = pdf.interact(viewUUID=..., action="navigate", page=2)`
inside `executeToolCode` changed that same view from page 1 to page 2.
Thus a script can drive an already-open App even though its own response does not
instantiate an App. This timeout constraint belongs to the server/gateway
configuration; no evidence establishes it as specific to Code Mode. The earlier
normal-mode PDF test established short navigation, not an idle long-poll cycle.

**Native Codex:** chat `01a0e30a-0e3f-7123-b28a-ece7dd41e765` completed four native
App calls and four separate scripts via `qa-code-apps`. Results reproduce native
structured content versus textual script output. The later
[user screenshot](qa/bifrost-2.2.3/four-apps-native-user-confirmation-2026-09-27.png)
confirms native Monitor, Debug and Excalidraw rendering, and shows a warning in
the PDF card. The PDF diagnosis and corrected native retest are recorded below.
`_meta` was not exposed to that chat's model, but the raw
HTTP checks and reference-host Debug event show native metadata preservation;
model-visible omission alone is not evidence of gateway loss.

**Lifecycle and scope:** the old browser views, gateways on 18083/18084, adapters
and four old QA connectors were closed/removed. The current isolated gateway is
127.0.0.1:18085, adapter 18090 and sandbox 8081, with only `qa-code-apps` added.
A concurrent `desktop.conversationDetailMode` preference change was observed and
preserved. Six simultaneous same-origin connections left the reference host stuck
in discovery; limiting its UI to one combined connection resolved it (browser
connection saturation is a hypothesis). No product source was changed, no commit
or deployment was made. Temporary Code Mode services remain for native chat
inspection; browser control views are closed after the campaign.

Reproduction artifacts remain under `/private/tmp/bifrost-apps-qa-20260927/code-mode`,
with `probe-code-mode.py` and `code-rpc.py` in its parent. They depend on that local
fixture. Outstanding work is the nested response/error contract and broader
interactions; successful execution is not full App support. Native rendering evidence
and the PDF-specific qualification are recorded below.

### Additional official Apps — normal mode, 2026-09-27

The [official Apps report](qa/bifrost-2.2.3/official-apps-normal-2026-09-27.json)
records a separate loopback instance of the same candidate binary on port 18084,
with official npm packages `@modelcontextprotocol/server-system-monitor`,
`server-pdf` and `server-debug` pinned to **2.0.3**. They run as local stdio
subprocesses; all three clients have Code Mode disabled. No product source or
production service was changed. Packages were installed into the temporary QA
folder, not into project dependencies. Installation used `--ignore-scripts`.

- **System Monitor:** 21/21 protocol assertions across direct `/mcp/monitor`, named
  Virtual MCP `/mcp/officialapps` and combined VK `/mcp`. HTML/MIME, static data,
  app-only callback visibility, changing samples and denied-VK resource/callback
  rejection pass. The reference host rendered live curves on all three routes:
  [direct](qa/bifrost-2.2.3/system-monitor-reference-host-2026-09-27.png),
  [Virtual MCP](qa/bifrost-2.2.3/system-monitor-virtual-host-2026-09-27.png),
  [combined](qa/bifrost-2.2.3/system-monitor-combined-host-2026-09-27.png).
  Start/Stop was exercised. Metrics describe this Mac, where the MCP server runs.
- **Native Codex:** fresh chat `01a0e2fc-aecc-7fc2-9963-94445ad87949` made one
  successful native `monitor-get-system-info` call via `qa-official-apps`.
  Successful polling continued every two seconds after all reference-host Monitor
  views were stopped. This is consistent with the native widget polling, but
  attribution is inferred; the native graph was not visually inspected by the agent.
- **PDF:** the public default arXiv sample rendered through combined `/mcp`.
  Calling `pdf-interact` with the existing `viewUUID` and `navigate`, page 2,
  changed the existing viewer from page 1 to page 2 and updated model context.
  [Page-2 capture](qa/bifrost-2.2.3/pdf-same-view-page2-2026-09-27.png).
  An initial synthetic local-file attempt was correctly rejected because its path
  was outside the server's allowed list; no file-access permission was widened.
- **Debug:** 6/6 grouped protocol checks across direct, virtual and combined routes:
  three text blocks, structured content and result metadata preserved; intentional
  `isError` preserved. In the combined reference-host UI, `connected`, `ontoolinput`
  and `ontoolresult` were observed, plus a successful UI `debug-refresh` callback.
  `ontoolinputpartial` remained zero in this non-streamed test; it is not qualified.
  Host logging/message capabilities were absent in this reference host, which
  is not evidence that Bifrost loses them.

Temporary services: candidate `127.0.0.1:18084`, reference host/credential adapter
`127.0.0.1:18090`, plus the existing reference sandbox at `localhost:8081` used by
its compiled frontend. The new adapter also starts an unused sandbox on 18091.
The adapter injects temporary VKs server-side. Temporary local connector
`qa-official-apps` points to `/qa/combined`; existing Codex configuration was
backed up and a parsed comparison found no other changes. These services remain
running for user inspection. Cleanup remains outstanding.

The fixture lives in `/private/tmp/bifrost-apps-qa-20260927/official-normal`; the
Monitor check is rerunnable with `python3 /private/tmp/bifrost-apps-qa-20260927/check-official.py`
while that fixture exists. Virtual MCP assignment was performed after VK creation,
so this campaign does not retest or fix the earlier cold-start ordering defect.
PDF/Debug native Codex rendering, comprehensive PDF interactions, all Debug
capabilities and Code Mode remain separate tests. This is a bounded smoke campaign.

### Normal-mode qualification — 2026-09-27

The [normal-mode manifest](qa/bifrost-2.2.3/normal-mode-qualification-2026-09-27.json)
binds this campaign to the existing `c3f711b49` Darwin ARM64 binary and records the
redacted fixture. Both MCP clients explicitly have `is_code_mode_client: false`.
They are two independently configured client identities pointing to the same live
Excalidraw service, not two different App implementations. No LLM providers are
configured. Production was untouched; no product-code fix was made.

| VK profile | Direct grants | Virtual MCP grants | Observed catalogue |
| --- | --- | --- | --- |
| `full` | `excalidraw:*` | None | `/mcp` and `/mcp/excalidraw`: Excalidraw only; named Virtual MCP rejected |
| `virtual` | None | `codexmcp`: `excalidraw:read_me`, `second:*` | `/mcp` and `/mcp/codexmcp`: exactly that bundle; `/mcp/excalidraw`: `read_me` only |
| `aggregate` | `excalidraw:*` | Same `codexmcp` | `/mcp`: union without duplicate tool names; `/mcp/codexmcp`: only the bundle, even with wider direct grants |
| `limited` | `excalidraw:read_me` | None | `read_me` only; App calls, callbacks and UI resource denied |
| `denied` | None | None | Root catalogue is `[]`; client and named Virtual MCP routes reject access |
| Invalid or absent key | None | None | Authentication rejected |

The [matrix report](qa/bifrost-2.2.3/mcp-apps-normal-matrix-2026-09-27.json)
passes **124/124 assertions**, including the existing 43 checks. It records actual
catalogue names, denial responses and exact callback save/readback payloads. The
three routes preserve structured results and UI HTML/MIME. The checkpoint assertions
save/read raw JSON arrays; they do not validate restoration of a user-edited scene,
which expects `{elements: [...]}`. See the [upstream behavior study](excalidraw-mcp-behavior-research.md). Multi-source resources
remain distinct; ambiguous original callback names are rejected and namespaced
callbacks route correctly. This count is an overlapping assertion count, not 124
independent end-to-end journeys.

**Cold-start defect observed:** on an empty SQLite store, the first campaign passed
[38/43 checks](qa/bifrost-2.2.3/mcp-apps-normal-first-boot-2026-09-27.json).
Both Virtual MCP/VK associations logged `FOREIGN KEY constraint failed`. Startup
calls `loadMCPConfig` before `loadGovernanceConfig` in
`transports/bifrost-http/lib/config.go`; the VK rows therefore do not exist when
the association reconciler runs. Restarting after their creation established the
associations. The restarted fixture also narrowed the Virtual MCP's Excalidraw
grant from `*` to `read_me` to test route scoping and additive grants. The successful
matrix does **not** erase the first-boot failure. No cold-start fix was applied
during that campaign; the later local source correction is recorded above.

The saved official reference-host captures show the App on
[direct](qa/bifrost-2.2.3/mcp-apps-normal-direct-render-2026-09-27.jpg) and
[combined](qa/bifrost-2.2.3/mcp-apps-normal-combined-render-2026-09-27.jpg) routes.
The [Virtual MCP capture](qa/bifrost-2.2.3/mcp-apps-normal-virtual-render-2026-09-27.jpg)
shows a blank card and does not establish completed rendering in that host.
The separate user screenshot below confirms native Codex rendering on all three.
A temporary, fixed-route loopback proxy injected VKs server-side; the browser did
not receive their values. The API matrix authenticated directly to Bifrost.
The direct editor opened, but visual shape editing/save remains unverified:
automation rejected fractional iframe coordinates, including after a viewport
override, and keyboard selection did not change editor state. The override was
reset. Protocol save/readback is verified separately. Completed input was supplied
to the host, so these captures make no claim about progressive rendering.

The user separately authorized a fresh native Codex conversation with three
temporary local connectors. [Native-call evidence](qa/bifrost-2.2.3/mcp-apps-normal-native-codex-2026-09-27.md)
confirms `read_me` then `create_view` succeeded on all three, preserving each
`structuredContent.checkpointId`. The agent-visible registry matched the expected
model tools: two direct, three Virtual MCP, four combined. Raw tool-definition UI
metadata was not exposed to that agent. Computer Use refused access to the Codex
application during automated inspection. The user then supplied a
[native Codex screenshot](qa/bifrost-2.2.3/mcp-apps-normal-native-user-confirmation-2026-09-27.png)
showing all three Apps, confirming native rendering. Visual editing/save remains
unverified. In a subsequent water-cycle test, `create_view` + `restoreCheckpoint`
opened a new view when the user wanted the existing view modified; that requirement
was not fulfilled. The native report records the exact distinction and the lack of
proof for the claimed progressive drawing. Code Mode is excluded from this campaign;
the historical Code Mode findings above remain separate.

### Earlier reference-host trial — 2026-09-27

The [qualification manifest](qa/bifrost-2.2.3/qualification-2026-09-27.json)
records the binary SHA-256, source commit, official host revision, isolated
configuration and exact test commands. Both focused Go commands passed, as did
Python syntax, report-counter consistency and local documentation-link checks.
The gateway was compiled as a native Darwin ARM64 binary; this is not a new Linux
image validation and does not validate the dashboard.

The [render capture](qa/bifrost-2.2.3/mcp-apps-aggregate-render-2026-09-27.jpg)
and [editor capture](qa/bifrost-2.2.3/mcp-apps-aggregate-edit-2026-09-27.jpg)
show an App through the aggregate `/mcp` endpoint. The official host ran in the
Codex browser panel, **not as a native tool result in a Codex conversation**.
The second upstream exposed only `read_me`. A named Virtual MCP without an
assigned caller grant returned 403; that route was not used for the visual proof.
Shape editing/save could not be verified: computer automation rejected the iframe
drag coordinates, and the keyboard alternative could not focus the canvas.

The browser initially failed its MCP handshake while Python calls succeeded.
The fixture's CORS response omitted MCP request headers. Bifrost already supports
the necessary configuration; setting these additional headers in the isolated
fixture made the reference-host connection succeed without a product-code change:

```json
{
  "client": {
    "allowed_headers": ["Mcp-Protocol-Version", "Mcp-Session-Id", "Last-Event-ID"]
  }
}
```

Origins remained explicitly limited to the local host. No wildcard header grant,
production configuration change, provider request or publication was performed.
The [official testing guide](https://apps.extensions.modelcontextprotocol.io/api/documents/Testing_MCP_Apps.html)
describes this host-based validation; it does not replace a native conversational
host test.

## Historical campaign — 2026-09-25

Vérifié le **25 septembre 2026**, sur macOS ARM64, avec Go 1.27.1.

**Résultat : aucune régression détectée dans les tests exécutés du candidat local 2.2.3 + MCP Apps. Le binaire officiel 2.2.3 ne fournit toujours pas notre relais MCP Apps.** Lors de cette campagne initiale, ni la production ni la branche de travail n'avaient été mises à jour.

## Fusion sur la branche de travail — 25 septembre 2026

Le tag officiel `transports/v2.2.3` (`411d62b`) est maintenant fusionné dans `feature/mcp-apps-native`, à partir du commit personnel `df5d01136`. L'historique local tronqué a d'abord été complété avec `git fetch --unshallow upstream`, puis la fusion a été lancée avec `git pull --no-rebase upstream transports/v2.2.3`.

Le seul conflit concernait les ajouts au catalogue HTTP `provider-harness.json`. Les **127 dossiers officiels** et notre dossier de **six requêtes MCP Apps** sont conservés. La reconnaissance des réponses MCP `initialize` est également conservée. Le contenu des ajouts et suppressions de notre diff personnel a été comparé avant/après : aucun changement du patch MCP Apps.

Les suites Go existantes ont été relancées avec `-race -count=1` sur `core/mcp/...`, `core/schemas/...`, les handlers et le serveur HTTP, `framework/configstore/...` et `plugins/governance/...` : **2 267 tests de premier niveau réussis, soit 4 166 avec les sous-tests ; zéro échec ; 12 tests ignorés** (10 nécessitent PostgreSQL, absent de cette relance, et 2 tests de performance explicitement désactivés). [Journal de cette relance](qa/bifrost-2.2.3/merged-branch-race.jsonl.gz).

Les runners existants `augment-provider-harness.mjs` et `filter-collection.mjs --feature mcp-apps --include-preview` valident la structure du catalogue fusionné et sélectionnent les six requêtes. Cette étape ne relance pas le harness HTTP en direct, le dashboard ni les fournisseurs LLM réels ; les observations HTTP ci-dessous restent celles de la campagne initiale. Aucun nouveau script de test ni changement de CI n'a été ajouté pour cette fusion.

## Où en était le projet ?

La branche `feature/mcp-apps-native`, au commit `1463c93`, contient déjà trois commits d'intégration au-dessus de Bifrost 2.2.2 (`fdeef8e`) :

- `a422dac` : relais natif des métadonnées UI, ressources et résultats MCP complets.
- `8c44647` : comportement complémentaire et couverture de régression.
- `1463c93` : isolation des callbacks sur les routes d'un seul serveur amont.

Excalidraw doit être connecté avec **Code Mode désactivé**, sur `/mcp/excalidraw`. Avec plusieurs serveurs, `/mcp` conserve les outils ordinaires mais masque les interfaces et callbacks réservés aux apps. Le rendu progressif dans Codex était encore ouvert ; voir la [note précédente](mcp-apps-realtime-research.md).

## Périmètre de preuve

| Variante | Provenance | Usage |
| --- | --- | --- |
| Référence locale 2.2.2 + MCP Apps | `1463c93a75985b4623da63eda5098eecf58cea62` | Tests ciblés et base SQLite avant migration |
| Officiel 2.2.3 | Binaire Darwin ARM64 téléchargé chez Maxim | Contrôle du comportement réellement distribué |
| Candidat 2.2.3 + MCP Apps | Tag `transports/v2.2.3`, commit `411d62b28b03b03bd3b4025b2cfab50af45f05f4`, plus le diff `fdeef8e..1463c93` | Compilation, tests Go, Excalidraw réel, migration |

Le diff local s'applique **sans conflit et sans adaptation du code produit**. Le candidat a été compilé dans une copie temporaire avec le `go.work` local ; son HTML embarqué était un simple substitut pour une compilation API. **Cette compilation ne valide pas le dashboard.** Le [manifeste](qa/bifrost-2.2.3/manifest.json) conserve les identifiants et empreintes exacts. L'empreinte du binaire officiel est calculée localement, sans checksum publiée par l'éditeur pour comparaison.

## Résultats

Les nombres ci-dessous incluent les sous-tests. Les suites se recoupent : **ne pas additionner leurs lignes**.

| Vérification | Résultat |
| --- | --- |
| Tests ciblés MCP de la référence locale 2.2.2 | 846 réussis, 0 échec, 2 ignorés |
| Mêmes tests sur le candidat 2.2.3 | 854 réussis, 0 échec, 2 ignorés |
| Suite complète `core/internal/mcptests`, avec `-race` et serveurs locaux construits | 669 réussis, 0 échec, 29 ignorés |
| Tous les tests des packages concernés, avec `-race` | 4 166 réussis, 0 échec, 12 ignorés initialement |
| Suite `framework/configstore/...` relancée avec PostgreSQL 16, avec `-race` | 927 réussis, 0 échec, 2 ignorés |
| Six scénarios Newman MCP Apps déjà présents dans le dépôt | 6 requêtes, 6 assertions réussies |
| Contrôle du binaire officiel 2.2.3 | 16/16 observations attendues, dont l'absence du support Apps |
| Candidat, connexion HTTP non persistante | 23/23 contrôles |
| Candidat, connexion persistante et deux serveurs amont | 27/27 contrôles |
| Référence 2.2.2 avant migration | 23/23 contrôles |
| Candidat 2.2.3 sur la **même base SQLite** | 23/23 contrôles |
| Même base migrée, activation de la connexion persistante et redémarrage | 23/23 contrôles |

Après déduplication par package et nom de test, et remplacement des résultats initiaux par ceux obtenus avec PostgreSQL : **2 703 tests de premier niveau réussis ; 4 858 résultats réussis en comptant les sous-tests ; 0 échec ; 31 tests ignorés.** Aucune course de données signalée par les suites exécutées avec `-race`.

Les packages entièrement exécutés sont `core/mcp/...`, `core/schemas/...`, `transports/bifrost-http/handlers/...`, `transports/bifrost-http/server/...`, `framework/configstore/...`, `plugins/governance/...` et, séparément, `core/internal/mcptests`.

Les 31 tests restants sont explicitement désactivés par les suites : scénarios avec vrai LLM, exemples/TODO, cinq tests de protocole marqués incompatibles Go/Node upstream, autres scénarios nécessitant une implémentation de fixture, et deux tests de performance semant environ un million de lignes. Ils ne sont **pas** comptés comme réussis. Les motifs exacts et le détail par suite figurent dans [go-summary.json](qa/bifrost-2.2.3/go-summary.json).

## Ce qui a été observé avec Excalidraw réel

| Contrat | Officiel 2.2.3 | Candidat 2.2.3 + MCP Apps |
| --- | --- | --- |
| `initialize`, `tools/list`, `read_me`, `create_view` | Fonctionnels | Fonctionnels |
| Extension `io.modelcontextprotocol/ui` négociée | Absente | Présente |
| `_meta.ui.resourceUri` | Perdue | Conservée et URI réécrite vers Bifrost |
| Résultat `structuredContent.checkpointId` | Perdu, résultat converti en texte | Conservé |
| `resources/list` / `resources/read` | Erreur `-32601` | Pris en charge |
| Ressource UI | Inaccessible | HTML, MIME `text/html;profile=mcp-app` |
| Callbacks `save_checkpoint` / `read_checkpoint` sans préfixe | Non fournis | Fonctionnels, visibilité `app` |
| Clés limitées / refusées / invalides | Restrictions confirmées | Restrictions confirmées, y compris les ressources et alias |

Le candidat expose une **ressource à template** : `resources/list` retourne une liste vide (`null` dans la sérialisation du SDK), `resources/templates/list` expose `ui://bifrost/{client}/{resource}`, puis `resources/read` lit l'URI annoncée par l'outil. La sonde ne confond donc pas une liste vide avec l'absence de support des ressources.

Avec deux amonts, la route agrégée n'annonce ni UI ni callbacks privés, ne négocie pas l'extension UI et refuse le callback non qualifié ; `/mcp/excalidraw` continue de fonctionner. L'ancien 403 signalé après changement de connexion persistante **n'a pas été reproduit dans ce scénario isolé** ; son origine passée n'est pas établie.

Preuves : [officiel](qa/bifrost-2.2.3/official-live.json), [candidat](qa/bifrost-2.2.3/patched-live.json), [multi-serveurs](qa/bifrost-2.2.3/patched-sticky-multi-live.json), [migration](qa/bifrost-2.2.3/patched-after-upgrade-live.json), [redémarrage avec connexion persistante](qa/bifrost-2.2.3/patched-upgrade-sticky-live.json), [harness existant](qa/bifrost-2.2.3/existing-harness.log).

## Nouveautés et corrections utiles

La [recherche détaillée avec sources primaires](bifrost-2.2.3-release-research.md) couvre les 43 commits et 144 fichiers de la release :

- **Nouveautés :** clé fournisseur imposable pour chaque fallback, outils OpenAI asynchrones, cache de prompt GPT-6 et `reasoning.effort: "none"` sur Sol/Luna.
- **Corrections :** sérialisation Responses/MCP, affinité des routes LLM, raisonnement Bedrock, cache OpenRouter/Anthropic, transcription Gemini, règles de routage et télémétrie.
- **Correctif supplémentaire présent dans le tag :** alias des noms d'outils longs pour Codex/Kimi, absent du changelog de release.
- **Compatibilité Go :** certaines propriétés Responses deviennent des unions et le type `ParsedFallbacks` change. Notre code d'intégration compile et passe ses tests avec ces changements.

La 2.2.3 ne contient pas notre relais MCP Apps. Une mise à jour vers son **binaire officiel seul** supprimerait les fonctionnalités ajoutées par notre compilation locale.

## Rejouer les vérifications

Dans une copie du tag 2.2.3 sur laquelle le diff `fdeef8e..1463c93` a été appliqué, avec le workspace Go local :

```sh
make setup-mcp-tests
env -u OPENAI_API_KEY go test -json -race -count=1 -timeout=20m ./core/internal/mcptests
go test -json -race -count=1 -timeout=10m \
  ./core/mcp/... ./core/schemas/... \
  ./transports/bifrost-http/handlers/... ./transports/bifrost-http/server/... \
  ./framework/configstore/... ./plugins/governance/...
```

Les tests PostgreSQL utilisent le service `postgres` de `tests/docker-compose.yml`, utilisateur/base `bifrost`, port 5432. Cette validation l'a lancé seul dans le projet Compose `bifrost-mcp-223-qa`, avec un override limitant le port à `127.0.0.1`.

La [sonde HTTP réutilisable](../tests/mcpapps/smoke.py) accepte `--url`, `--keys`, `--output`, `--expect-apps` et `--multiple-clients`. Le fichier de clés JSON doit contenir `full`, `limited`, `denied` : clés temporaires `sk-bf-…` pour respectivement tous les outils Excalidraw, uniquement `read_me`, aucun outil. Activer `client.enforce_auth_on_inference`, configurer le client sous le nom et slug `excalidraw`, sans Code Mode. Pour le scénario multiple, ajouter l'amont `second` et l'autoriser pour `full`. La sonde ne sauvegarde ni clés ni HTML.

Les six scénarios existants ont été extraits sans modification de leurs assertions du dossier `112. MCP Apps Excalidraw (mcp-apps)` de `provider-harness.json`, puis exécutés via Newman avec une authentification Bearer héritée de la collection et une clé temporaire.

## Limites et état final

- **Validé :** code de la passerelle, protocole MCP Apps, contrôle d'accès, connexions persistantes, migration SQLite et tests PostgreSQL concernés.
- **Non validé :** rendu visuel/animation progressive dans Codex ou ChatGPT, dashboard compilé, autres fournisseurs LLM réels, autres suites du monorepo et instance de production. Le dessin progressif exige une vérification des événements hôte → iframe ; un succès HTTP ne le prouve pas.
- **Modifications du dépôt lors de la campagne initiale :** documentation, preuves et sonde de test uniquement. La fusion ultérieure sur la branche de travail est décrite en début de document ; aucune mise à jour de production.
- Les serveurs QA et le conteneur PostgreSQL dédiés sont arrêtés et supprimés après les mesures ; les preuves sans clés sont conservées dans `docs/qa/bifrost-2.2.3/`.

Les premiers essais de la nouvelle sonde ont révélé deux erreurs de **fixture de test**, corrigées avant les résultats retenus : préfixe de VK incorrect et attente d'une ressource statique alors que le relais utilise un template. Les erreurs initiales de ports/cache Go du sandbox ont été résolues en exécutant les tests avec les permissions nécessaires. Aucun de ces essais n'est présenté comme une régression produit.
