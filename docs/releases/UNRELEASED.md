# Proposed changes for the next release

These changes are under PR review and are not part of an existing published release.

## Recovery

- Creating a backup preserves existing archives. Duplicate normalized API names
  return `409 backup_exists` instead of replacing a recovery point; omit the name
  for an automatically generated name with subsecond precision.
- Failed backup writes clean up the newly created incomplete archive. Existing
  archives and the live database remain unchanged.

See [Library Operations](../library-operations.md#backup-and-staged-restore) for
backup/restore behavior. This fix does not establish RC5 upgrade or complete
cross-platform recovery qualification.

## Maintenance

- Removal and restore confirmations explain which library details change and
  recommend creating a current backup before continuing. Declining leaves the
  repair or restore request unsent.

## Accessibility

- Proposed modal improvements in [PR #134](https://github.com/ajbergh/ViiB-MediaHub/pull/134)
  name Direct Download controls, contain keyboard focus and make confirmation
  dialogs start on Cancel. Escape dismisses idle dialogs; busy actions stay protected.

## Release dependency information

- Proposed [release inventories](../release-dependency-inventories.md) add an npm
  SBOM and Go module graph to future release assets and their checksum manifest.
  Signing, provenance, native-library inventories and licensing review remain open.

## Release channel gate

- Proposed workflows reject stable versions/tags until production signing,
  notarization and verification are qualified. RC/prerelease packaging remains
  available with existing signing limitations. Non-tag manual Windows builds
  use development artifact versions; accepted tag drafts are marked prerelease.

Proposed canonical release provenance: attest the exact 13 payloads and verify
repository/workflow/commit/ref before publication. Live attestation qualification
and production platform signing remain pending; see
[provenance policy](../release-provenance.md).

## Desktop framework

- Upgrade Wails from v2.11.0 to v2.17.0, including upstream fixes for the
  macOS 26 WebView crash during rapid updates, Linux bound-method panic handling,
  and the Windows WebView2 bootstrapper error dialog. Native platform
  qualification remains pending; see the [upgrade plan](../WAILS_V2_17_UPGRADE_PLAN.md).
