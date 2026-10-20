# Accessibility qualification plan and findings

Status: draft plan for maintainer review, 2026-10-20. Roadmap: P3.4 and P4.5.
No WCAG conformance claim is made. The plan/register is delivered separately
from executing it on browser and native hosts.

## Standard and decision rule

Use the [W3C WCAG 2.2 quick reference](https://www.w3.org/WAI/WCAG22/quickref/)
filtered to levels A and AA as the complete criterion inventory. The workflow
matrix below prioritizes application checks; it does not replace that inventory.
[WAI Easy Checks](https://www.w3.org/WAI/test-evaluate/preliminary/) provides an
initial review, not a complete evaluation.

For each candidate, retain one ledger row per applicable A/AA criterion, with
criterion ID, workflow, host, result, evidence and reviewer. Allowed results:
pass, fail, not tested, or not applicable with a reviewed rationale. Missing rows
or evidence remain not tested. Specialized music/DJ surfaces receive an explicit
applicability review; their complexity does not automatically exempt them.

A maintainer approves the plan, supported host/assistive-technology matrix and
any release exceptions before qualification. A successful automated audit or a
keyboard shortcut list cannot establish conformance. Close findings only after
retesting the fix on the affected surface/host; retain the earlier failure.

## Reproducible test setup

Record commit/PR, build flavor (browser or Wails), OS/architecture, browser or
WebView version, assistive technology/version, viewport/zoom, input device,
reviewer/date and fixture revision. Use synthetic song/playlist/source names and
redistributable media; never upload private libraries, accounts, tokens or paths.

Start with a fresh profile, then repeat relevant flows with saved preferences.
Cover empty and populated catalogs, loading, unavailable sources, validation
errors and recovery. Save sanitized screenshots, keyboard steps, accessibility
observations and contrast measurements with findings. For audio actions, record
the expected audible outcome without requiring a user to hear status messages.

Proposed manual host matrix (all **not tested** until evidence is attached):

| Host | Assistive technology/input | Required surfaces |
| --- | --- | --- |
| Windows browser + Wails WebView2 | NVDA, keyboard, pointer | Standard flows and DJ alternatives |
| macOS browser + Wails WKWebView | VoiceOver, keyboard, pointer | Standard flows and DJ alternatives |
| Linux browser + Wails WebKitGTK | Orca, keyboard, pointer | Standard flows and DJ alternatives |
| Narrow browser viewport | Keyboard/pointer at 320 CSS px and zoomed desktop | Standard flows; separately assess spatial DJ layout |

Record the exact tested browser/screen-reader pairing. Build availability does
not imply screen-reader support. Add touch and switch/voice input where included
in the supported-product matrix; mark absent coverage explicitly.

## Application workflow matrix

| ID | Journey and concrete checks | Relevant criterion examples | Evidence required |
| --- | --- | --- | --- |
| A11Y-01 | Traverse main navigation, page heading, player, queue and back; verify usable focus order and a way past repeated navigation | 1.3.1, 2.1.1, 2.1.2, 2.4.1, 2.4.3 | Keyboard sequence and screen-reader outline |
| A11Y-02 | Choose a first-run source, back, skip and return in Settings; open/close dialogs and restore focus to the trigger | 2.4.3, 2.4.7, 2.4.11, 4.1.2 | Focus observations for each transition |
| A11Y-03 | Search/filter library, create/rename a playlist, like a song and inspect metadata; verify labels, state and errors | 2.4.6, 2.5.3, 3.3.1, 3.3.2, 4.1.2 | Named controls and announced validation/state |
| A11Y-04 | Start/pause playback, seek and adjust volume with keyboard; open/close queue and EQ; type shortcut letters into fields without transport effects | 2.1.1, 2.1.4, 4.1.2 | Input sequence, focus target and observed transport state |
| A11Y-05 | Scan/import, simulate source failure, retry and inspect progress; review backup/restore and deletion confirmation with synthetic data | 3.3.4, 4.1.3 | Announced status, confirmation and recovery outcome |
| A11Y-06 | Check text/control contrast, selected/focus states and color-independent source/analysis notices | 1.4.1, 1.4.3, 1.4.11 | Measured foreground/background pairs and UI states |
| A11Y-07 | Resize text, apply text spacing and zoom/reflow; inspect popovers, sticky player and dialogs for clipped/obscured content | 1.4.4, 1.4.10, 1.4.12, 1.4.13, 2.4.11 | Geometry, zoom/spacing settings and screenshots |
| A11Y-08 | Operate small controls and reorder/adjust draggable values without dragging; inspect responsive target spacing | 2.5.1, 2.5.7, 2.5.8 | Physical CSS-pixel measurements and alternate operations |
| A11Y-09 | Pause/stop motion and visualization; inspect reduced-motion behavior and flashing risk | 2.2.2, 2.3.1 | Settings/state, visual evidence and manual assessment |
| A11Y-10 | Complete provider authentication and reauthentication, preserve user input and explain expiry/errors accessibly | 3.3.7, 3.3.8, 4.1.3 | Authorized host/provider session evidence; otherwise not tested |

Use the normative criterion text for thresholds and exceptions. In particular,
the existing DJ audit's 44 px authored-canvas transport target is a product rule;
it is not a measured WCAG target-size result after the canvas is scaled.

## DJ alternatives to qualify

These are candidate alternatives, not verified equivalent access:

| Interaction | Candidate alternative | Qualification task |
| --- | --- | --- |
| Deck transport and active-deck selection | Visible buttons plus [documented shortcuts](keyboard-shortcuts.md#dj-mode) | Reach every control, announce deck/state and return to normal navigation; check Tab behavior |
| Crossfader, gains, EQ and parameter dragging | Focusable sliders/numeric controls and keyboard increments where implemented | Verify name, value, bounds, increments and a non-drag operation for each control |
| Waveform/cue positioning | Time/position readouts and explicit seek/cue controls where implemented | Verify equivalent task completion; record any missing alternative |
| Color-coded key, energy, source and analysis state | Textual facts/status and track metadata | Verify state is available without hue or visual waveform interpretation |
| Hot cues, loops and library search | Named controls and documented keyboard paths | Verify labels, editing, shortcuts, focus restoration and screen-reader discoverability |
| Full spatial workstation | Standard player/library for ordinary listening | Covers ordinary listening only; do not claim it substitutes for all DJ tasks |

If an alternative is absent, file a concrete finding with task impact. Do not
label an unimplemented alternative supported or silently exclude a DJ feature.

## Existing automation and limits

- [DJ audit](../scripts/djv2-audit.mjs): names, toggle states, authored transport
  sizes and overflow at fixed geometries. It needs a running app and a usable
  Chromium installation. No execution/conformance pass is recorded by this plan.
- [PR #131](https://github.com/ajbergh/ViiB-MediaHub/pull/131): three synthetic
  browser-only navigation/storage tests and a named playlist textbox. These are
  focused regressions; they do not evaluate the full workflow matrix.
- Contrast, screen-reader operation, native focus, provider login, spatial DJ
  exceptions and equivalent alternatives require dedicated evidence.

## Findings register

Severity records user impact, not an automatic release verdict. Use blocker
(core task impossible), major (substantial barrier), minor, or investigation.
Every confirmed finding needs reproducible steps, expected/actual behavior,
affected criterion, evidence, owner/issue, proposed remedy and retest result.

| ID | Status/severity | Observation and evidence | Next action |
| --- | --- | --- | --- |
| AX-001 | Fix proposed; manual qualification open / minor | Playlist-name field now has an explicit name in [PR #131](https://github.com/ajbergh/ViiB-MediaHub/pull/131); its Chromium regression finds it by role/name | Retest with screen reader and verify visible labeling/instructions; no complete 3.3.2 pass claimed |
| AX-002 | Source observation; not a confirmed conformance failure / investigation | [Global keyboard handler](../hooks/useKeyboardNavigation.ts) implements bare character shortcuts and typing guards; [user guide](keyboard-shortcuts.md) documents them | Review 2.1.4 applicability and whether off/remap/focus-only options meet it; reproduce with speech input before choosing a fix |
| AX-003 | Measurement gap / investigation | [DJ audit](../scripts/djv2-audit.mjs) measures authored canvas sizes, while the workstation scales | Measure rendered CSS-pixel targets/spacing and assess 2.5.8 exceptions; retain actual viewport/scale |
| AX-004 | Fix proposed; host qualification open / major | Direct Download lacked a named close control/dialog semantics; confirmation focus could escape the modal. The follow-up adds dialog/field names, a shared focus trap, Cancel-first confirmation focus and busy Escape protection | Automated dialog/hook and synthetic deletion-cancel regressions; retest native/screen-reader operation |

Qualification gaps (not observed application defects): all manual matrix rows,
full A/AA ledger, assistive-technology host coverage, DJ alternatives and plan
approval remain open. Assign reviewers and append evidence as work is performed.
An empty defect list must never be interpreted as a passing audit.

## Review record

| Date | Reviewer | Decision/evidence | Remaining gate |
| --- | --- | --- | --- |
| 2026-10-20 | Codex (source/document review) | Initial plan/register; W3C references checked; no new manual host audit | Maintainer plan approval, complete ledger and execute supported-host matrix |
