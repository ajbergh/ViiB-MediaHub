# VIIB Design System v1 — Implementation Audit (Viib MediaHub)

Date: 2025-12-28  
Repo: ViiB-MediaHub (React/TS + Vite + Tailwind; Go/Wails backend)  
Audit basis: code inspection of the current workspace. Any item not verifiable in code is marked **Unknown / Needs Verification**.

Last updated: 2025-12-28 (Phase 1 complete; Phase 2 complete; Phase 3 complete: playback UI/visualizer literal cleanup; Phase 4 complete: AI DJ signature styling/typography aligned; Phase 5 complete: motion/a11y polish + context-menu keyboard navigation; Phase 6 complete: typography sweep + menu semantics + CI enforcement; Phase 7 complete: spacing/layout wrappers + list header standardization)

---

## 1. Executive Summary

**Overall design-system adoption level: Medium–High**

**Key strengths of current implementation**
- **Centralized theme tokens exist and are actively used** via Tailwind theme extension (surfaces/text/brand/accent) in `tailwind.config.js`.
- **A small, enforceable primitive layer exists** (`components/ui/*`: Button, Card, Chip, TextInput, Menu) with consistent motion + focus treatment.
- **Playback UI is meaningfully aligned**: the bottom player and expanded Now Playing view predominantly use tokens and the new primitives.
- **Regression prevention started**: TSX-only hex color enforcement exists (`scripts/check-no-raw-colors-tsx.mjs`, `npm run check:raw-colors`) and an optional default Tailwind palette scanner exists (`scripts/check-no-tailwind-palette.mjs`, `npm run check:palette`).

**Top 3 risks or gaps**
1. **Gradients and accent usage still need stronger constraints** (even though they are now tokenized and the default Tailwind palette is eliminated). Without explicit rules/allowlists, “calm + intentional accents” can still drift over time.
2. **Row-level spacing consistency is the main remaining drift vector**: page shells/headers are now largely standardized via `Page`/`PageHeader`/`ListHeader`, but list rows and empty-state containers still contain one-off spacing patterns.
3. **Accessibility and keyboard patterns need ongoing verification**: global focus-visible exists, context menus support full keyboard navigation, and a broad reduced-motion/tab-order sweep has been applied, but complex surfaces should still be periodically audited.

Update (2025-12-28): core primitives adoption is now substantially complete across AI DJ, Settings, Spotify, and First Launch flows; remaining inconsistency is primarily row/empty-state spacing patterns and a few bespoke accessibility patterns outside menus.

---

## 2. What Is Already Implemented

### Color & Theming
- **Central palette mapped to Viib DS v1 tokens** (surfaces/text/brand/accent)  
  Where: `tailwind.config.js`  
  Compliance: ✅ Fully compliant

- **App shell uses surface/text tokens (no global gradient background)**  
  Where: `components/Layout.tsx`  
  Compliance: ✅ Fully compliant

- **Canvas visualizers use allowlisted/centralized literal helpers where practical** (canvas-only usage)  
  Where: `components/ui/tokens.ts` (allowlisted values/helpers), `components/Visualizer.tsx`, `components/now-playing/AlbumArtVisualizer.tsx`  
  Compliance: ⚠️ Partially compliant (Visualizer is centralized; AlbumArtVisualizer still contains many canvas-only `rgba(...)`/`hsla(...)` strings that are acceptable but not fully centralized)

- **Tokenized context-menu system (menus + submenus)**  
  Where: `components/ContextMenu.tsx`, `components/context-menus/*` (SongMenu, AlbumMenu, ArtistMenu, PlaylistMenu, SmartMixMenu, QueueItemMenu)  
  Compliance: ✅ Fully compliant (tokenized styling + ARIA roles + keyboard navigation)

### Typography
- **Viib type scale is defined in Tailwind theme** (display/section/card/body/meta)  
  Where: `tailwind.config.js`  
  Compliance: ✅ Fully compliant

- **Pages and key flows adopt the scale for major hierarchy** (page titles, section headers, hero headings)  
  Where: `pages/*`, `components/NowPlaying.tsx`, `components/FirstLaunchDialog.tsx`  
  Compliance: ✅ Fully compliant (microcopy may still use small Tailwind sizes where DS tokens are not expressive enough)

- **Base font family enforced at document level**  
  Where: `index.css` (`font-family: 'Inter'`)  
  Compliance: ✅ Fully compliant

### Layout & Spacing
- **Consistent shell layout** (Sidebar + Main + Player; fixed app height)  
  Where: `components/Layout.tsx`  
  Compliance: ✅ Fully compliant

- **Page-level spacing is standardized; row-level spacing still drifts** (wrappers cover shell/header/list headers; list rows + empty states still have one-offs)  
  Where: `components/ui/Page.tsx` (`Page`, `PageHeader`, `ListHeader`), plus remaining one-offs across list rows/empty states in multiple screens  
  Compliance: ⚠️ Partially compliant

### Components
- **Button primitive (variants + accent support + focus ring)**  
  Where: `components/ui/Button.tsx`  
  Compliance: ✅ Fully compliant

- **Card primitive (12px radius, ring, hover motion)**  
  Where: `components/ui/Card.tsx`  
  Compliance: ✅ Fully compliant

- **TextInput primitive (focus-within ring + tokens)**  
  Where: `components/ui/TextInput.tsx`  
  Compliance: ✅ Fully compliant

- **Chip primitive (selected ring by accent)**  
  Where: `components/ui/Chip.tsx`  
  Compliance: ✅ Fully compliant

- **Menu primitive (menu role + focus-visible styling)**  
  Where: `components/ui/Menu.tsx`  
  Compliance: ✅ Fully compliant (shared Arrow/Home/End navigation, typeahead, Escape-to-close hook)

### Playback UI
- **Player bar aligned to tokens + primitives; playback state uses accent-green**  
  Where: `components/Player.tsx`  
  Compliance: ⚠️ Partially compliant (tokenized interactions/colors; remaining work is broader consistency/typography)

- **Now Playing view substantially migrated to tokens + primitives**  
  Where: `components/NowPlaying.tsx`  
  Compliance: ⚠️ Partially compliant (substantially tokenized; lyrics/queue interactions improved; remaining work is broader consistency/typography)

### AI DJ
- **AI DJ feature exists as a dedicated screen with prompt-based generation**  
  Where: `pages/SmartPlaylists.tsx`, backend endpoints described in `services/api.ts`  
  Compliance: ✅ Fully compliant (core primitives adopted; typography and state accents aligned to Brand Purple)

- **Navigation entry is present and labeled “AI DJ”**  
  Where: `components/Sidebar.tsx`  
  Compliance: ✅ Fully compliant

### Motion
- **Global motion defaults and keyframe utilities exist** (fade/slide/scale/bg-loop)  
  Where: `tailwind.config.js`  
  Compliance: ⚠️ Partially compliant (exists, but not consistently adopted across all screens)

- **Reduced motion support is implemented globally**  
  Where: `index.css` (`prefers-reduced-motion`)  
  Compliance: ✅ Fully compliant

### Accessibility
- **Global focus-visible outline is set**  
  Where: `index.css`  
  Compliance: ✅ Fully compliant

- **Many interactive controls include `aria-label`** (Player, Sidebar, multiple pages)  
  Where: e.g., `components/Player.tsx`, `components/Sidebar.tsx`, `components/NowPlaying.tsx`  
  Compliance: ⚠️ Partially compliant (coverage is good, but not systematic across all custom controls)

- **Context menu closes on Escape + click-outside**  
  Where: `components/ContextMenu.tsx`, `components/context-menus/MenuShared.tsx`, `components/context-menus/SongMenu.tsx`  
  Compliance: ✅ Fully compliant (roving focus, Enter/Space activation, Escape, typeahead, submenu ArrowRight/ArrowLeft)

---

## 3. Partial Implementations & Inconsistencies

- **Primitives exist, but adoption is incomplete**  
  What exists: `components/ui/*` primitives used in Home/Player/NowPlaying and some library views.  
  What’s missing: remaining gaps are mostly row/empty-state spacing normalization across some pages (e.g., collection lists), not core button/input primitives.  
  Why it matters: increases one-off styling, inconsistent hover/focus/motion, and higher cost for future DS changes.  
  Suggested correction: continue consolidating row/empty-state spacing patterns on screens that still use bespoke wrappers.

- **Default Tailwind palette usage has been eliminated, but enforcement policy is still evolving**  
  What exists: `npm run check:palette` is in place and currently reports **0** offenders.  
  What’s missing: deciding when/where to run it (CI vs local), whether to enable strict mode by default, and whether any allowlists are needed long-term.  
  Why it matters: prevents regression back to `text-gray-*` / `bg-green-*` style drift.

- **Gradient usage still needs DS-level constraints**  
  What exists: gradients on high-visibility pages have been softened and moved to DS tokens (no default palette), but there is no formal limit/allowlist on where gradients are acceptable.  
  What’s missing: consistent gradient rules (max colors, contrast constraints, and restricted placement).  
  Why it matters: gradients can still become decorative noise even when tokenized.

- **Typography scale is now consistent for major hierarchy; row/empty-state spacing remains inconsistent**  
  What exists: `text-display`, `text-section`, etc. are used for page titles/section headers across pages and key flows.  
  What’s missing: further reduction of one-off spacing in list rows and empty states to fully align rhythm across collection views.  
  Why it matters: typography reads cohesive, but the UI can still feel “variable” due to row-level spacing drift.

- **Context menu system is keyboard-accessible; Menu primitive now shares keyboard semantics**  
  What exists: context menus support roving focus + typeahead + submenu traversal; `components/ui/Menu.tsx` supports Arrow/Home/End + typeahead + Escape-to-close hook for non-context menus.

---

## 4. Missing or Not Yet Implemented

- **A single, authoritative design system reference in-repo**  
  Status: Unknown / Needs Verification (no `Viib Design System v1` document found in repo files inspected during this audit).  
  Impact: engineering lacks a canonical artifact to validate changes and prevent drift.

- **System-wide restriction on gradients and accent count**  
  Evidence: multiple screens use multiple bright gradients (e.g., `pages/Stats.tsx` genre gradient array; `pages/LikedSongs.tsx` brand gradient hero). Gradients are now token-based, but there is no formal limit on where/how many gradients are acceptable.  
  Missing: an enforceable rule that limits gradient placement and accent frequency per screen.

- **AI DJ “signature experience” styling** (distinct but calm brand expression)  
  Evidence: AI DJ (`pages/SmartPlaylists.tsx`) now uses core primitives (TextInput/Button/Chip), DS typography tokens (`text-display`, `text-section`, `text-meta`), and Brand Purple for all state accents.  
  Status: ✅ Complete as of Phase 4.

- **Spacing system normalization**  
  Evidence: `Page`/`PageHeader`/`ListHeader` now exist and are used on many high-traffic screens; remaining drift is concentrated in list rows and empty states.  
  Missing: a systematic pass to unify row/empty-state spacing patterns across library lists.

- **Enforcement beyond hex colors**  
  Evidence: `npm run check:raw-colors` blocks hex in TSX and `npm run check:palette` exists to block Tailwind default palette usage. `index.css` still contains literal color values for browser/platform chrome.  
  Missing: clear policy/allowlist for non-hex literal colors (e.g., `rgba(...)`) outside canvas/WebAudio contexts and whether CSS should be tokenized over time.

---

## 5. Technical Debt & Risks

- **Hardcoded literal colors not covered by current enforcement**
  - `index.css` uses raw hex and rgba for body + focus outline + scrollbar styling.
  Impact: token changes won’t automatically propagate; inconsistent appearance across WebView/platform.

- **Repeated one-off button/input styles**
  Evidence: core button/input usage has been migrated to primitives across AI DJ and First Launch; remaining drift is mostly typography/spacing patterns and a few bespoke status “badge” treatments.
  Impact: drift in hierarchy/spacing and higher maintenance cost.

- **Overly expressive color usage in analytics/collections screens**
  Evidence: gradients and accents remain a prominent visual device in analytics/collections screens (now tokenized).  
  Impact: can still violate calm/mood-first principle if not constrained.

- **Context menu accessibility gap**
  Evidence: context menus now support roving focus, typeahead, and keyboard submenu traversal.
  Impact: reduced; remaining accessibility risk is broader ARIA labeling/tab-order consistency outside the context menu system.

- **Dynamic class construction that may bypass design constraints**
  Evidence: previously present in `pages/Stats.tsx`; now removed in favor of a token-safe mapping.
  Impact: reduced; keep watch for similar patterns elsewhere.

---

## 6. Phased Implementation Plan

### Phase 1 — Foundation Stabilization
**Status:** ✅ Largely complete (default Tailwind palette eliminated; `npm run check:palette` reports 0 offenders)
**Intent:** ensure tokens + typography + layout primitives are the default path, and prevent regressions.  
**Scope:**
- Remove or token-map remaining default Tailwind palette usage (`gray-*`, `green-*`, `amber-*`, `purple-*`, etc.) in high-traffic screens.
- Reduce literal color usage outside allowlisted canvas contexts (add policy for rgba/white/black utility usage where applicable).
- Normalize top-level page paddings and section spacing.

**Outcomes / Deliverables:**
- Centralized theme remains authoritative (`tailwind.config.js`) and is used consistently.
- Reduced visual noise on key pages; fewer “special-case” colors.
- No visual regressions + stable build checks.

### Phase 2 — Core Components Alignment
**Status:** ✅ Complete (core routes migrated to primitives)
**Intent:** make the primitive set the only supported UI building blocks for new work.  
**Scope:**
- Migrate remaining bespoke UI controls to primitives: Buttons, inputs, chips/toggles, menus.
- Replace `components/context-menus/MenuShared.tsx` styling to token-based text/icon colors; align `components/ContextMenu.tsx` wrapper typography/colors.
- Add a small set of layout wrappers (page header, section header, panel) if needed.

**Outcomes / Deliverables:**
- Reusable primitives used across all major routes.
- Fewer one-off styles; consistent hover/focus/motion.

### Phase 3 — Playback Experience Upgrade
**Status:** ✅ Complete (playback UI/visualizer literal cleanup)
**Intent:** ensure playback is the most emotionally engaging, consistent part of the app and adheres to DS rules.  
**Scope:**
- Remove remaining non-token literals in playback UI (e.g., range thumb `bg-white`, visualizer rgba barColor).
- Audit lyric/queue behaviors in Now Playing for interaction consistency.
- Ensure accent usage remains intentional: playback state = Pulse Green, AI DJ = Brand Purple.

**Outcomes / Deliverables:**
- Cohesive playback states and consistent feedback.
- Playback UI fully aligned to tokens/primitives.

### Phase 4 — AI DJ Signature Experience
**Status:** ✅ Complete
**Intent:** make AI DJ feel like a distinct, premium “Brand Purple” feature without visual noise.  
**Scope:**
- Finish normalizing `pages/SmartPlaylists.tsx` typography to DS tokens (titles/section headers/meta) and remove remaining `text-white` / ad-hoc sizing where practical.
- Replace any remaining non-DS accent usage (or overuse of accents) while preserving meaning.
- Keep generation feedback patterns (loading/errors) calm and consistent using existing DS status colors/toasts.

**Outcomes / Deliverables:**
- Distinct AI DJ identity aligned to DS v1.
- Clear value perception and consistent interactions.

### Phase 5 — Motion, Polish & Accessibility
**Status:** ✅ Complete
**Intent:** make the UI production-grade and consistent for keyboard/reduced-motion users.  

**Progress to date:**
- ✅ Context menus support keyboard navigation (ArrowUp/Down, Home/End), activation (Enter/Space), Escape, typeahead, and submenu traversal (ArrowRight/ArrowLeft).
- ✅ Menu items and submenus use consistent ARIA roles (`role="menu"` / `role="menuitem"`) and predictable focus behavior.
- ✅ Reduced-motion support applied to major animated overlays/components (`motion-reduce:animate-none` / `motion-reduce:transition-none`) and common tab-order issues fixed (e.g., click-outside overlays as non-tabbable buttons).

**Outcomes / Deliverables:**
- Motion consistency + reduced-motion respected.
- Accessibility baseline met (keyboard + focus + ARIA semantics).

### Phase 6 — Typography, Gradients & DS Governance
**Status:** ✅ Complete
**Intent:** reduce long-term drift by standardizing typography/layout patterns and adding enforceable UI “policy” (gradients/accents/menus).

**Scope:**
- Typography normalization sweep on remaining high-traffic screens (Stats, Liked, remaining collections): adopt the DS type scale (`text-display`, `text-section`, `text-body`, `text-meta`) and reduce ad-hoc sizes.
- Gradient policy: document/align an allowlist for where gradients are acceptable (and keep them subtle + token-based).
- Enforcement policy decisions: decide where `npm run check:palette` should run (CI vs local), and whether strict mode is desired.
- Menu primitive follow-up: decide whether `components/ui/Menu.tsx` stays styling-only or gains shared keyboard semantics to prevent bespoke dropdown behavior.

**Outcomes / Deliverables:**
- Clear typography hierarchy across pages and key flows (no ad-hoc `text-3xl+` sizing in screens/components audited).
- Reduced visual noise from gradients (notably removing gradient-heavy genre pills on Stats; gradients kept to intentional hero surfaces).
- Reduced regressions via enforcement in CI (Windows workflow runs `check:palette`, `check:raw-colors`, and `typecheck`).
- Shared non-context-menu dropdown behavior via `components/ui/Menu.tsx` keyboard semantics and an `onRequestClose` hook.

### Phase 7 — Spacing & Layout Standardization
**Status:** ✅ Complete
**Intent:** make screens feel consistently “built from the same grid” by standardizing spacing and repeated layout patterns.

**Scope:**
- Define 2–3 layout wrappers/patterns (page header, section block, panel/grid spacing) and migrate the highest-traffic screens.
- Reduce one-off `p-*` / `mb-*` / arbitrary grid gaps; prefer consistent section rhythm.
- Keep gradients/accent usage constrained to intentional hero surfaces; expand policy only if needed.

**Outcomes / Deliverables:**
- More consistent vertical rhythm and screen composition across the app.
- Lower maintenance cost for future DS changes.

**Progress to date (2025-12-28):**
- Added `components/ui/Page.tsx` with `Page` and `PageHeader` wrappers.
- Migrated page shells/headers on Home, Albums, Artists, Search, Playlists, Downloads, Stats, Settings, and Liked Albums to use the wrappers.
- Added `ListHeader` and migrated Songs + Liked Songs Virtuoso headers to remove remaining one-off `p-8 pb-0`.

---

## 7. Recommended Next Actions

1. **Spacing consistency follow-up**: do a quick spot-check for remaining one-off spacing patterns outside `Page`/`PageHeader`/`ListHeader` (especially in list rows and empty states).
2. **A11y follow-up (ongoing)**: periodically re-audit `aria-label` on icon-only buttons, non-button click targets, and tab-order traps across Now Playing, library lists, and dialogs.
3. **Visualizer literals (optional)**: continue centralizing canvas-only `rgba(...)`/`hsla(...)` strings to `components/ui/tokens.ts` where practical.
4. **CSS literal policy (optional)**: decide whether `index.css` literal colors (scrollbars, focus outline) should be tokenized over time.

---

### Appendix — Evidence Quick Index (Non-exhaustive)
- Tokens + type scale: `tailwind.config.js`
- Global base + reduced motion: `index.css`
- Primitives: `components/ui/Button.tsx`, `components/ui/Card.tsx`, `components/ui/TextInput.tsx`, `components/ui/Chip.tsx`, `components/ui/Menu.tsx`
- Shell: `components/Layout.tsx`, `components/Sidebar.tsx`
- Playback: `components/Player.tsx`, `components/NowPlaying.tsx`
- AI DJ: `pages/SmartPlaylists.tsx`
- Gradient-heavy pages: `pages/Stats.tsx`, `pages/LikedSongs.tsx`
- Context menu system: `components/ContextMenu.tsx`, `components/context-menus/MenuShared.tsx`
- First launch flow (migrated to primitives): `components/FirstLaunchDialog.tsx`
