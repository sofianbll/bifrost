# Fork issue tracking

This file maps the approved Bifrost MCP repair spec to the fork's GitHub issues. The spec remains the source of acceptance criteria; issue bodies should link to it rather than copy it.

Use explicit repository scoping for every issue command or REST path:

```sh
gh issue view 21 --repo sofianbll/bifrost
gh issue list --repo sofianbll/bifrost --state open
gh api repos/sofianbll/bifrost/issues/21
gh api repos/sofianbll/bifrost/issues/21/comments
```

The accepted spec and dependency graph is:

```text
#21 approved spec (parent of the implementation subissues)
 ├─ #22 test and fixture preparation
 │    ├─blocks→ #23 Code Mode error semantics
 │    ├─blocks→ #24 first-boot governance grants
 │    └─blocks→ #25 native MCP Apps behavior
 └─ #23, #24, and #25 each block #26 integration qualification
```

Issue #22 prepares immutable stock/candidate source identities, fixture coverage, and the credential-free Actions comparison. It does not change product behavior. The first push-triggered run on `codex/spec21-integration` uses `characterize` mode against the unchanged candidate and records actual stock and candidate assertions. For the #25 fixture, synthetic Virtual MCP key attachments are created through the configured setup-token API only after the disposable gateway starts; this keeps #25 independent of #24's first-boot ordering. Strict mode is for a later candidate once the repair tickets are integrated.

After the initial implementation attempt, each issue gets at most two repair cycles:

1. Make one bounded implementation or test repair, run the applicable checks, and update the issue with the changed files and evidence.
2. If review or evidence finds a defect, make one final bounded repair and repeat the checks. If acceptance still fails, report the blocker with evidence instead of starting another cycle.

Track #21 as the authority, #22 as preparation, #23–#25 as independent repairs, and #26 as the final cross-layer gate. Keep issue comments concise and point to committed test reports or Actions artifacts; do not duplicate the full spec or paste raw logs.
