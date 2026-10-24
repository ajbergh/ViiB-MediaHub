# Unreleased changes since v1.0.0-rc8

Reviewed against `main` at `818cf94` on 2026-10-23. These changes are merged
after the RC8 tag; this file does not announce a new release. See the
[RC8 notes](v1.0.0-rc8.md) for earlier recovery, MIDI, accessibility and release-integrity work.

## Stems

- MP3 and Ogg/Vorbis stem packages now validate and play through the backend
  decoder. Compressed packages declare `compressed-stems-v1`; hashes cover the
  encoded files and validation checks exact decoded geometry. WAV remains
  supported. See the [package contract](../VIIB_STEM_PACKAGE_V1.md). (#156)
- Stem Library discovery supports nested packages, four-stem compatibility and
  source-identity reconciliation. Successful startup catalog checks enqueue a
  background stem scan; **Scan Stem Libraries** remains available for a manual
  refresh. Ordinary music scans exclude `.viibstems` contents. See
  [Settings](../settings.md#stem-libraries). (#157)

## Desktop and dependencies

- Correct Windows tray icon corruption and icon packaging. (#158)
- Upgrade Wails and its pinned CLI from v2.11.0 to v2.17.0. The
  [upgrade record](../WAILS_V2_17_UPGRADE_PLAN.md) separates executed Windows checks
  from remaining native macOS/Linux and interaction qualification. (#159)
- Repair Wails dependency restoration and update npm dependencies/lockfile to
  resolve the audit findings recorded during the upgrade. (#160)

The current backend baseline is Go 1.26.10. Stable release signing/notarization,
hardware routing/controller validation and professional analysis accuracy remain
open gates in the [roadmap](../ROADMAP.md).
