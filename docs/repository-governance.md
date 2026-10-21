# Main-branch governance

**Status: active since October 20, 2026, with explicit owner approval.** Hosted
main protection and feature-branch exclusion were read back successfully.
Merged-head required-check qualification remains open.

Roadmap P0.3. The [versioned ruleset](../.github/rulesets/main.json) targets only
`refs/heads/main`. Its hosted activation and verification record is below; a JSON
file alone does not establish enforcement.

## Normal changes

Use a pull request, keep the branch current with main, pass all required checks,
and resolve review conversations before merging. Direct changes, force pushes
and deletion of main are restricted. Formal approval count is zero for the
current sole-maintainer workflow; that is not a claim of independent review.
If approvals are submitted, new commits dismiss stale approvals.

The ruleset binds these 16 always-on contexts to GitHub Actions (observed app ID
15368). Upstream analysis/semantic contexts are explicit requirements so a failed
prerequisite cannot be hidden behind a skipped downstream package job.

| Required contexts | Scope |
| --- | --- |
| `frontend` | Style/type/unit/build checks and production npm audit |
| `browser-e2e` | Deterministic Chromium browser journeys |
| `backend` | Go tests/race/vet/Staticcheck, web/Wails builds and binary vulnerability scans |
| `Track analysis (linux)`, `Track analysis (windows)`, `Track analysis (darwin)` | Cross-host analysis tests |
| `Track analysis scalar determinism` | Cross-host scalar comparison |
| `Semantic foundation (windows/amd64)`, `Semantic foundation (windows/arm64)` | Windows semantic compile checks |
| `Semantic foundation (linux/amd64)` | Linux semantic compile check |
| `Semantic foundation (darwin/amd64)`, `Semantic foundation (darwin/arm64)` | macOS semantic compile checks |
| `Desktop build (linux-amd64)`, `Desktop build (macos-arm64)`, `Desktop build (macos-amd64)` | Native desktop packaging |
| `windows-build` | Windows packaging and packaged-binary vulnerability scan |

Release workflow contexts are conditional on version/workflow changes and are
not unconditional branch requirements. Release qualification still requires
that workflow and the roadmap's signing, installer and real-host evidence.

## Roadmap integration record

[PR #153](https://github.com/ajbergh/ViiB-MediaHub/pull/153) was squash merged
as `3400c0df720d36b421718a562ff1a0e364518910` with explicit owner authorization
after all 16 required contexts and 11 release-validation/package jobs passed.
The normal protected PR process was used; no emergency bypass was requested.
The merged tree equals reviewed head `26e002f`. Source PRs #131–#152 were
closed as superseded after verifying their unchanged heads were included.
Their branches and history remain available; do not merge duplicate changes.
See the [dated audit](ROADMAP_EASY_PASS_2026-10-20.md) and
[integration evidence](evidence/roadmap-easy-pass-2026-10-20.json).

[Post-merge CI](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/39530977041)
was running at the documentation checkpoint. Keep later changes in fresh PRs
from current main and wait for their required checks. Never use emergency
bypass for ordinary CI delay.

When renaming checks, first introduce/observe the new context, update the hosted
ruleset and this file together, and retire the old context only after cutover.
Keep replacement checks running during the transition so PRs remain mergeable.

## Reviewed emergency bypass

The active policy grants only repository owner `ajbergh` (user ID 53879585) a PR-only bypass. There is no always/exempt
bypass, deploy-key bypass or direct-push emergency path in this configuration.
Use it only for an urgent repair when ordinary required checks cannot complete.

1. Keep the repair in a PR. Record the incident, exact head SHA, unavailable or
   failing checks, impact of waiting, test evidence, risk assessment and rollback.
2. Obtain an independent maintainer review when one is available. If the sole
   owner must act without one, explicitly record that exception and the owner's
   review in the PR. Ordinary CI delay is not an emergency by itself.
3. The owner explicitly selects the PR bypass and records why. Do not
   weaken required checks, switch enforcement off or force-push main to proceed.
4. Link a follow-up to restore/run the missing checks and review the result.
   Preserve the PR and GitHub bypass/audit record; do not treat bypass as a pass.

This process defines a per-incident review record. It does not retroactively
approve any current PR or authorize an automatic merge/release.

## Apply and inspect

After explicit owner approval, use repository administration access. Snapshot the existing rules first. If a
ruleset named `main-required-ci` exists, update that ID; do not create duplicates.
For initial activation from the repository root:

```bash
gh api --method POST repos/ajbergh/ViiB-MediaHub/rulesets --input .github/rulesets/main.json
```

For later reviewed updates, use the same payload with `--method PUT` and
`repos/ajbergh/ViiB-MediaHub/rulesets/RULESET_ID`. Verify the returned active
policy against the versioned JSON, then inspect effective rules:

```bash
gh api repos/ajbergh/ViiB-MediaHub/rules/branches/main
gh api repos/ajbergh/ViiB-MediaHub/branches/main --jq .protected
```

Also inspect a feature-branch name to confirm this ruleset does not match it.
Check required names/source app against a current successful PR head. Do not
probe enforcement with a real merge, destructive push or bypass.

GitHub documents the [ruleset API](https://docs.github.com/en/rest/repos/rules)
and [required-check/source/strict behavior](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/available-rules-for-rulesets).

## Verification record

2026-10-20: before activation, main was unprotected and the ruleset inventory
was empty. Following explicit owner approval, ruleset
[`25487842`](https://github.com/ajbergh/ViiB-MediaHub/rules/25487842) was activated.
The [API evidence](evidence/main-governance-2026-10-20.json) records:

- All versioned policy fields matched the hosted policy, including the 16
  app-bound strict checks and sole-owner PR-only bypass.
- Main reported `protected: true` and all four expected effective rule types.
- A feature-branch scope probe returned no effective rules.

At activation, all 16 contexts had been observed passing on PR #131/#132.
No merge or bypass was attempted as part of that activation check. The later
owner-authorized #153 merge and source-PR cleanup are recorded above; inspect
the linked post-merge CI for merged-head validation.
