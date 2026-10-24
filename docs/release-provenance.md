# Release artifact provenance

Roadmap P0.4 / P0.5. **Implemented workflow gate.** The RC8 release notes describe this pipeline.
A source change does not retrofit attestations onto older releases; independent
download/identity/tamper verification remains a qualification requirement.

## Publication boundary

The canonical [release workflow](../.github/workflows/release.yml) first validates
the [exact 13 payloads](release-asset-verification.md) and creates `SHA256SUMS.txt`.
The immutable `actions/attest` v4.2.2 commit
`1e69f48acb82d1966a394da916b4c1698aa569d6` consumes that manifest to create a
single SLSA build-provenance attestation covering all 13 payload digests, including
both dependency inventories. The manifest itself is not a subject.

Only the non-PR publish job receives OIDC and attestation-write permissions.
The public repository uses GitHub's keyless Sigstore integration. No platform
signing credential is required for this provenance step. Registry push/storage
records are disabled; no artifact-metadata permission is requested.

The [verification wrapper](../scripts/verify_release_provenance.py) rechecks the
exact asset set and invokes `gh attestation verify` for every payload against the
returned bundle. Each invocation requires:

- The repository from `GITHUB_REPOSITORY` and its canonical `release.yml` signer.
- The exact `GITHUB_SHA` source commit and `GITHUB_REF` branch/tag ref.
- The SLSA v1 predicate and a GitHub-hosted runner identity.

Missing/empty bundles, invalid policy inputs, asset-set errors, missing CLI and
any failed cryptographic verification stop publication. The first failed subject
stops verification; release creation is a later step with no failure override.
The verified bundle is retained as `provenance-attestation` for 14 days, outside
the `release-*` download/upload set. GitHub's attestation API also stores it.

Manual prerelease dispatch still supports a feature branch; this gate verifies
the actual source ref rather than assuming main. Consumers must choose their
own accepted reviewed commit/ref. The [stable-release guard](release-signing-policy.md)
continues to reject stable releases.

## Consumer verification

After downloading an attested payload, independently choose the expected
source commit and ref from the reviewed release workflow run. Do not derive trust
policy from an unverified bundle or a filename. Replace the example values:

```bash
gh attestation verify ./ViiB-MediaHub-VERSION-windows-x64-setup.exe \
  --repo ajbergh/ViiB-MediaHub \
  --signer-workflow ajbergh/ViiB-MediaHub/.github/workflows/release.yml \
  --source-digest REVIEWED_FULL_COMMIT_SHA \
  --source-ref refs/heads/main \
  --predicate-type https://slsa.dev/provenance/v1 \
  --deny-self-hosted-runners
```

For an intentionally approved tag/feature-branch build, specify that exact ref
instead. Repeat for each downloaded payload. The CLI fetches associated
attestations by default. A separately retained bundle can be supplied with
`--bundle PATH`; the same identity restrictions still apply.

Provenance ties payload bytes to a workflow/source identity. It does not prove
independent reproducibility, safe source code, license compliance, complete
native SBOMs, Windows Authenticode, Apple notarization or installation success.
The publisher attests payloads assembled from jobs in the same workflow; it is
not an independent per-target build audit. The legacy Windows draft path has
no attestation gate and remains a separate qualification gap.

## Implementation validation and remaining consumer evidence

The checks below record validation performed when the gate was introduced.
[RC8 notes](releases/v1.0.0-rc8.md) document the release pipeline; this documentation
pass did not independently fetch or verify its published payloads or attestations.

Six standard-library tests use synthetic files and a mocked CLI to check all
13 subjects and mandatory policy arguments, stop-on-failure behavior, invalid
identity inputs, missing/empty bundles, unexpected assets and CLI failure exit
status. **Mocks do not verify signatures or establish a successful OIDC run.**

Both workflow YAML files parsed; publish-only OIDC permissions and gate ordering
were checked. The real installed GitHub CLI rejected a synthetic malformed
bundle and the wrapper exited 1. That negative probe does not qualify successful
signature verification.

Full consumer qualification requires evidence from a prerelease publication run:
inspect the attestation's source/subject claims, verify every downloaded payload
with the trusted identity, reject tampered bytes/wrong commit/ref/workflow,
record the run and attestation IDs, and retain consumer-verification evidence.
No workflow dispatch, tag, release or live attestation was created while adding
this gate.

Primary references: [pinned action documentation](https://github.com/actions/attest/tree/1e69f48acb82d1966a394da916b4c1698aa569d6),
[GitHub attestation guide](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations),
and [GitHub CLI verification policy](https://cli.github.com/manual/gh_attestation_verify).
