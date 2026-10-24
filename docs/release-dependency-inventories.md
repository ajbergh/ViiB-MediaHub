# Release dependency inventories

The implemented P0.5 release-inventory workflow publishes two dependency files
alongside release packages. It does not add files to previously published releases.
Both files are required by the release asset verification step and included in
`SHA256SUMS.txt`; missing inventories stop publishing.

| File | Contents | Limits |
| --- | --- | --- |
| `npm-sbom.cdx.json` | npm CycloneDX SBOM generated from `package-lock.json`, including development dependencies | Not an exact list of modules embedded in the optimized frontend |
| `go-modules.json` | Schema version 1, with the selected main/direct/indirect Go module graph; original and replacement versions and available sums | Not a list of packages linked into every target binary, license audit, native-library SBOM or provenance attestation |

The Go export uses `go list -mod=readonly -m -json all` from `backend`, then
converts its JSON-object stream into a single JSON document. Modules are sorted
by path. Absolute cache/checkout paths and unrelated Go tool fields are excluded.
Versioned replacements preserve their effective path/version/checksums. Local
unversioned replacements are rejected: they do not identify a reproducible public
release input. A graph error, duplicate module or missing main module also fails
inventory generation. The script does not modify `go.mod` or `go.sum`.

Generate the Go inventory locally with:

```bash
python scripts/generate_go_module_inventory.py --output output/go-modules.json
python -m unittest discover -s scripts -p test_go_module_inventory.py
```

After `npm ci`, generate the npm inventory with:

```bash
npm sbom --package-lock-only --sbom-format cyclonedx > output/npm-sbom.cdx.json
```

Inspect the `modules` array, especially `Replace`, rather than assuming the
original requested module/version was shipped. Compare inventories between
candidate tags when reviewing dependencies. Use the checksums to detect download
corruption; checksums downloaded from the same release do not independently
prove publisher identity. Signing/provenance verification remains a separate
[roadmap gate](ROADMAP.md#3-phase-0-security-and-release-foundations-p0).

The release validation job retains inventories for 14 days, including PR builds.
The canonical release publisher attaches them to the GitHub release. Native/MinGW DLLs,
OS libraries, vendored upstream code, license obligations, per-target linked
packages and security advisory acceptance still need separate review.

The [release asset verifier](release-asset-verification.md) requires the exact
package/inventory set and rejects unexpected upload inputs before generating
checksums. A recorded PR-candidate assembly check uses real downloaded artifacts;
it is separate from verifying a published release.
