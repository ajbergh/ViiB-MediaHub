# Main-branch governance

**Status: proposed; not activated.** Hosted main remains unprotected. Activation
is pending explicit owner approval.

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

## Existing PR stack

The roadmap PRs are stacked. Merge an ancestor first, retarget its next child to
main, bring that child current and wait for fresh required checks. Merge commits
preserve the stack's original ancestry. If using squash/rebase instead, rebuild
the next child on the merged main while preserving only its own changes; review
the resulting diff before updating its remote branch. Do not use emergency
bypass merely to avoid stack maintenance or queued checks.

When renaming checks, first introduce/observe the new context, update the hosted
ruleset and this file together, and retire the old context only after cutover.
Keep replacement checks running during the transition so PRs remain mergeable.

## Reviewed emergency bypass

The proposed policy grants only repository owner `ajbergh` (user ID 53879585) a PR-only bypass. There is no always/exempt
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

2026-10-20: owner administration access, unprotected main and an empty ruleset
inventory were verified before preparation. All 16 proposed contexts were
observed passing from GitHub Actions on PR #131/#132. Hosted activation was not performed and requires owner approval; no live
protection, merge or bypass test is claimed. The configuration was checked
against observed successful check names/source and parsed as JSON.
This document and its PR are the concrete activation proposal, not proof of
hosted enforcement.
