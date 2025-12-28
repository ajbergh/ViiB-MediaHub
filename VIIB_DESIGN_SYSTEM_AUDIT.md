# VIIB Design System v1 — Implementation Audit (Viib MediaHub)

Date: 2025-12-28  
Repo: ViiB-MediaHub (React/TS + Vite + Tailwind; Go/Wails backend)  
Audit basis: code inspection of the current workspace. Any item not verifiable in code is marked **Unknown / Needs Verification**.

Last updated: 2025-12-28 (Phase 1 complete; Phase 2 complete; Phase 3 complete: playback UI/visualizer literal cleanup; Phase 4 complete: AI DJ signature styling/typography aligned; Phase 5 complete: motion/a11y polish + context-menu keyboard navigation; repo-wide checks passing)

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
2. **Typography and component consistency are mixed**: core routes now use the primitive set, but several screens still use ad-hoc sizing (e.g., Stats and some collection views), and typography scale adoption is not yet systematic.
3. **Accessibility and keyboard patterns need ongoing verification**: global focus-visible exists, context menus support full keyboard navigation, and a broad reduced-motion/tab-order sweep has been applied, but complex surfaces should still be periodically audited.

Update (2025-12-28): core primitives adoption is now substantially complete across AI DJ, Settings, Spotify, and First Launch flows; remaining inconsistency is primarily typography/spacing patterns and a few bespoke accessibility patterns outside menus.

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

- **Some screens adopt the scale** (e.g., Home uses `text-display` / `text-section`)  
  Where: `pages/Home.tsx`  
  Compliance: ⚠️ Partially compliant (many screens still use `text-3xl`, `text-4xl`, `text-5xl`, etc.)

- **Base font family enforced at document level**  
  Where: `index.css` (`font-family: 'Inter'`)  
  Compliance: ✅ Fully compliant

### Layout & Spacing
- **Consistent shell layout** (Sidebar + Main + Player; fixed app height)  
  Where: `components/Layout.tsx`  
  Compliance: ✅ Fully compliant

- **Spacing patterns are present but not standardized** (frequent `p-8`, `mb-12`, arbitrary grid gaps)  
  Where: multiple screens (`pages/Home.tsx`, `pages/Stats.tsx`, `pages/LikedSongs.tsx`, etc.)  
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
  Compliance: ⚠️ Partially compliant (role exists; keyboard navigation semantics beyond focus styles are not implemented here)

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
  What’s missing: remaining gaps are mostly typography scale adoption + spacing normalization across several pages (e.g., Stats / collection views), not core button/input primitives.  
  Why it matters: increases one-off styling, inconsistent hover/focus/motion, and higher cost for future DS changes.  
  Suggested correction: continue the typography normalization sweep, then consider a simple layout/spacing wrapper pattern.

- **Default Tailwind palette usage has been eliminated, but enforcement policy is still evolving**  
  What exists: `npm run check:palette` is in place and currently reports **0** offenders.  
  What’s missing: deciding when/where to run it (CI vs local), whether to enable strict mode by default, and whether any allowlists are needed long-term.  
  Why it matters: prevents regression back to `text-gray-*` / `bg-green-*` style drift.

- **Gradient usage still needs DS-level constraints**  
  What exists: gradients on high-visibility pages have been softened and moved to DS tokens (no default palette), but there is no formal limit/allowlist on where gradients are acceptable.  
  What’s missing: consistent gradient rules (max colors, contrast constraints, and restricted placement).  
  Why it matters: gradients can still become decorative noise even when tokenized.

- **Typography scale defined but not consistently used**  
  What exists: `text-display`, `text-section`, etc. in Tailwind config; Home (and AI DJ header) use them.  
  What’s missing: consistent headings/body/meta usage across all pages (e.g., Liked Songs uses `text-5xl font-black`, other collection/analytics screens still use ad-hoc sizes).  
  Why it matters: inconsistent hierarchy and “mood-first” cadence; harder to create consistent layouts and responsive behavior.  
  Suggested correction: codify a small set of page patterns (page title, section header, meta label) and gradually replace ad-hoc sizes.

- **Context menu system is now keyboard-accessible; Menu primitive remains styling-first**  
  What exists: global context menus support roving focus + typeahead + submenu traversal with consistent ARIA roles.  
  What’s missing: decide whether `components/ui/Menu.tsx` should grow shared keyboard semantics (or remain styling-only) to avoid bespoke non-context-menu dropdown behavior.

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
  Evidence: many pages use raw Tailwind spacing values without a shared layout primitive or spacing tokens.  
  Missing: a small set of layout wrappers/section patterns.

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

---

## 7. Recommended Next Actions

1. **Typography normalization sweep**: standardize page titles/section headers to the DS type scale across remaining high-traffic screens (Stats, Liked, Spotify, Settings, AI DJ).
2. **Gradient policy**: add explicit guidance/allowlist for where gradients are acceptable (and keep them subtle, token-based).
3. **Palette enforcement decision**: consider enabling `npm run check:palette --strict` in CI once the team is comfortable (allowlist only if truly required).
4. **Accessibility follow-up**: periodically re-audit `aria-label` on icon-only buttons, click targets that aren’t real `<button>`/`<a>`, and tab-order traps across Now Playing, library lists, and dialogs.
5. **Menu primitive follow-up**: decide whether `components/ui/Menu.tsx` should grow shared keyboard semantics (or remain styling-only) so non-context menus don’t reintroduce bespoke behavior.
6. **Typography normalization (continued)**: migrate remaining high-traffic screens (`pages/Stats.tsx`, `pages/LikedSongs.tsx`, `pages/LikedAlbums.tsx`) to DS type scale (`text-display`, `text-section`, `text-meta`).

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
