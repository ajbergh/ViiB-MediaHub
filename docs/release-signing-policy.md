# Release channel and signing policy

Roadmap P0.4. **Proposed guard; stable signing is not implemented or qualified.**
The current desktop pipeline ad-hoc signs macOS bundles without notarization and
only signs Windows executables/installers when credentials are available. Those
paths remain suitable only for clearly disclosed prerelease qualification work.

## Fail-closed stable boundary

The [shared release guard](../scripts/release_channel_guard.py) rejects stable
`X.Y.Z` versions/tags. It permits valid prerelease versions such as `1.0.0-rc7`
and `1.2.3-beta.1`. Versions use numeric major/minor/patch, nonempty prerelease
identifiers, and no leading zeroes in numeric identifiers. Build metadata is not
accepted by this repository's release-version contract. Tag input must be exactly
`v` plus the resolved version. Malformed/mismatched inputs fail too.

The guard deliberately rejects stable releases even if signing secrets happen
to exist: the macOS Developer ID/notarization and complete verification paths
are still missing. There is no credential-driven or CLI override.

| Entry point | Enforced boundary |
| --- | --- |
| [Canonical release workflow](../.github/workflows/release.yml) | Guard after resolving the version, before packaging fan-out; repeat immediately before publication |
| [Legacy Windows workflow](../.github/workflows/build-windows.yml) | Guard tag refs before toolchain/build work and again before archive version output |
| Non-tag manual Windows build | Development artifact version derived from commit SHA; no release publication |
| Ordinary PR/main CI | Validation/builds continue; guard unit tests run without invoking publication |

The legacy Windows tag workflow creates a draft marked prerelease for accepted
tags. Canonical prerelease publication retains its existing prerelease flag.
Stable requests fail rather than silently yielding unsigned stable artifacts.
No signing credentials are inspected by the guard or its tests.

The proposed [artifact-provenance gate](release-provenance.md) adds keyless
workflow/source identity verification for canonical prerelease payloads. It does
not provide platform signing or relax this stable boundary.

## Replacing the temporary guard

Stable enablement needs a separate reviewed implementation and evidence:

1. Windows application/installer Authenticode signing with mandatory verification
   and a failure when required credentials, timestamping or verification fail.
2. macOS Developer ID signing, notarization/stapling and signature/Gatekeeper
   checks for both desktop architectures, with missing prerequisites failing.
3. Supported-host install/upgrade/signature smoke evidence and the remaining
   stable-release gates in [the roadmap](ROADMAP.md#7-phase-4-10-stable-release-qualification-p0-gate).
4. Replace the blanket stable rejection with those mandatory evidence-backed
   checks in every publication path. Preserve a regression proving an unsigned
   or unverified stable release cannot pass. Do not add an allow-stable flag or
   remove the guard merely to publish a release number.

This policy does not procure credentials, notarize artifacts, qualify existing
RCs, authorize a release, or certify licensing/provenance.

## Validation

```bash
python -m unittest discover -s scripts -p test_release_channel_guard.py
python scripts/release_channel_guard.py --version 1.0.0-rc7 --tag v1.0.0-rc7
```

The current test suite checks stable/malformed rejection, accepted prereleases,
tag agreement and actual CLI exit status. Local extracted-workflow experiments
also execute canonical Bash preflight/publication and legacy PowerShell guard/
version steps against synthetic inputs. They build/publish nothing. Full hosted
packaging and future real-host signing qualification remain separate evidence.
