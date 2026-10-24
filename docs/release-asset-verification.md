# Release asset verification

Roadmap P0.5 / P4.6. The implemented
[verifier](../scripts/verify_release_assets.py) requires the exact release input
set before writing `SHA256SUMS.txt`. The publisher uploads the whole directory,
so an unexpected file must fail validation rather than ship outside the checksum
manifest. The manifest itself is the only extra allowed entry and is regenerated.

## Contract

The current set is 13 nonempty regular files: Windows installer, two macOS DMGs,
Linux DEB/tarball, six web ZIPs, npm CycloneDX inventory and Go module inventory.
Missing or extra entries, empty files, directories and artifact/manifest symlinks
fail before a new manifest is written. Files are hashed in a deterministic order
using bounded reads; output uses the standard SHA-256 checksum-file format.
Adding a target or inventory requires updating the reviewed asset contract too.

From the repository root, with candidate artifacts assembled under one directory:

```bash
python scripts/verify_release_assets.py --directory output/candidate-assets --version 1.0.0-rc7
cd output/candidate-assets
sha256sum -c SHA256SUMS.txt
```

This reads/hashes packages and writes a checksum manifest; it does not launch
binaries or publish a release. Channel eligibility is enforced separately by the
[release guard](release-signing-policy.md). Hash consistency does not prove a
publisher identity, signature, license approval or successful installation.

The canonical publish job invokes this same verifier before release creation.
The implemented [provenance gate](release-provenance.md) consumes the manifest and
verifies every payload identity before publication.
CI and release preparation run its standard-library unit tests. A failed verifier
step prevents the later wildcard upload from executing.

## October 20 candidate evidence

[Recorded evidence](evidence/release-assets-pr133-2026-10-20.json) covers the real
artifacts from [PR #133's validation run](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/39441891481),
source head `00260f3ac88cb573fb47f526c7f3f0d03d8f6471`. Those artifacts have an
RC7 version label, but **they are not the published RC7 release assets**. Do not
use this evidence file as the checksum source for that published release.

All 13 expected files (156,404,395 bytes total) were assembled locally without
running them. Python-generated checksums passed an independent `sha256sum -c`
check. An injected extra-input probe was rejected and left the prior manifest
unchanged. The source artifacts were retained without modification.
The npm inventory contains 287 components; the Go inventory contains 302 modules,
preserves its versioned replacement and excludes checkout/cache fields.

Local asset tests passed six cases; Windows could not create the symlink fixture,
so that case was skipped locally. The [Linux CI job](https://github.com/ajbergh/ViiB-MediaHub/actions/runs/39452605678/job/118731096338)
executed all seven asset tests successfully, including both symlink boundaries,
with no skips on verifier head `726a9d7`. The seven channel
and four inventory tests also passed. Published-release verification, production
signing/notarization, install/upgrade, per-target/native SBOMs, provenance and
licensing acceptance remain separate open gates.
