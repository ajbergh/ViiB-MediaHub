# Viib MediaHub — Phased UX Implementation Plan (Next Concrete Steps)
**Scope:** Implement *Hierarchy (Hero vs Utility)* + *Intent-Aware Empty States* (with exact component API updates, copy, and phased delivery).  
**Audience:** Engineering + Product + Design (single source of truth).  
**Version:** v1.0  
**Last updated:** 2025-12-28

---

## 0) Goals and Success Criteria

### Goals
1. Establish a clear **visual hierarchy system** across screens so “what matters now” is obvious at a glance.
2. Replace passive/neutral blanks with **intent-aware empty states** that guide users to a successful next action.
3. Do this with **minimal UI churn** by updating a small set of primitives (Card + SectionHeader + EmptyState).

### Success Criteria
- **Hierarchy:** On Home, users can instantly identify (a) the primary action/hero area, (b) core content, (c) utility metrics.
- **Empty States:** Each empty state includes **one clear next action** and contextual copy (no dead-ends).
- **Engineering:** Fewer one-off styles; consistent usage through typed variants and tokens.

---

## 1) Definitions (Design System Additions)

### 1.1 Emphasis Tiers
Add a system-level emphasis tier that is consistent across Cards and Sections:

```ts
export type ViibEmphasis = 'hero' | 'primary' | 'secondary' | 'utility';
```

**Tier meaning:**
- `hero`: 0–1 per screen; may use gradient; larger typography; most prominent.
- `primary`: default content cards and lists; standard surface.
- `secondary`: supporting content; slightly reduced contrast.
- `utility`: counts, metadata, small summaries; lowest prominence, still readable.

### 1.2 Intent-Aware Empty State
Empty states should be explicit about “what is empty” and “what to do next”.

```ts
export type EmptyStateIntent =
  | 'ai_dj'
  | 'smart_mix'
  | 'playlist'
  | 'liked_songs'
  | 'liked_albums'
  | 'search_no_results'
  | 'queue_empty'
  | 'lyrics_unavailable'
  | 'recently_played'
  | 'stats_not_enough_data'
  | 'library_scanning';
```

---

## 2) Phase Plan Overview

### Phase 1 — Foundation + APIs (1–2 PRs)
**Outcome:** New typed primitives exist; no visual regressions; app compiles with minimal changes.

Deliverables:
- `Card` API updated to support **emphasis tiers**.
- New `EmptyState` primitive + registry of empty copy by intent.
- `SectionHeader` (or equivalent) updated to align with hierarchy model.

### Phase 2 — Screen-by-Screen Adoption (2–5 PRs)
**Outcome:** Home, AI DJ, Smart Mixes, and core library screens adopt the hierarchy + empty states.

Deliverables:
- Home hierarchy refined (1 hero, utility metrics de-emphasized).
- AI DJ empty experience upgraded with examples and a single next action.
- Library & playlists: empty states introduced and consistent.

### Phase 3 — Polish + Guardrails (1–2 PRs)
**Outcome:** Conformance is enforceable and difficult to regress.

Deliverables:
- Lint-style checks (optional) or runtime dev warnings for misuse:
  - More than 1 hero per screen
  - More than 2 accent colors per screen (if you already enforce)
  - Missing empty-state intent mapping for known states
- Visual QA checklist + snapshots.

---

## 3) Exact Card API Changes (Implementation Spec)

> Objective: Make hierarchy a first-class concept without multiplying bespoke “variants”.

### 3.1 New Card Props

```ts
export type CardTone = 'surface' | 'muted' | 'elevated'; // maps to tokens
export type CardEmphasis = ViibEmphasis; // 'hero' | 'primary' | 'secondary' | 'utility'

export interface ViibCardProps {
  title?: string;
  subtitle?: string;
  icon?: React.ReactNode;

  // NEW
  emphasis?: CardEmphasis;        // default: 'primary'
  tone?: CardTone;               // default: 'surface'
  gradient?: 'none' | 'hero';    // default: 'none' (only allowed if emphasis === 'hero')

  // Existing / typical
  padding?: 'sm' | 'md' | 'lg';  // default: 'md'
  interactive?: boolean;         // default: false (adds hover lift + focus ring)
  href?: string;                 // optional; if provided, becomes clickable
  onClick?: () => void;

  headerRight?: React.ReactNode; // e.g. kebab menu / quick action
  footer?: React.ReactNode;

  children?: React.ReactNode;

  // Accessibility
  ariaLabel?: string;
}
```

### 3.2 Behavioral Rules (Enforced in Code)
**Rule A — Hero gradient gating**
- If `emphasis !== 'hero'`, force `gradient = 'none'`.
- If `emphasis === 'hero'`, allow `gradient = 'hero'` only.

**Rule B — Utility density**
- `emphasis='utility'` defaults to smaller padding + smaller metadata text.

**Rule C — Interactivity**
- `interactive=true` adds:
  - hover lift
  - subtle shadow
  - focus-visible ring

### 3.3 Styling Mappings (Token-Level Guidance)
Map emphasis to token usage (pseudocode):

```ts
const emphasisStyles = {
  hero: {
    title: 'text-display-sm',
    subtitle: 'text-body',
    surface: 'surface-raised',
    border: 'border-subtle',
    shadow: 'shadow-lg',
    padding: 'lg',
  },
  primary: {
    title: 'text-title',
    subtitle: 'text-body',
    surface: 'surface-raised',
    border: 'border-subtle',
    shadow: 'shadow-md',
    padding: 'md',
  },
  secondary: {
    title: 'text-title',
    subtitle: 'text-meta',
    surface: 'surface-dark',
    border: 'border-subtle',
    shadow: 'shadow-sm',
    padding: 'md',
  },
  utility: {
    title: 'text-body-strong',
    subtitle: 'text-meta',
    surface: 'surface-dark',
    border: 'border-subtle',
    shadow: 'shadow-none',
    padding: 'sm',
  },
} as const;
```

### 3.4 Example Usage Patterns

#### Hero (Home: “Smart Mix of the Moment” or 1 featured mix)
```tsx
<Card
  emphasis="hero"
  gradient="hero"
  title="Fresh Finds"
  subtitle="Recently added to your library"
  interactive
  onClick={...}
/>
```

#### Utility (Home: “Total Songs / Albums / Artists”)
```tsx
<Card
  emphasis="utility"
  tone="muted"
  title="24,514"
  subtitle="Total Songs"
  icon={<MusicNoteIcon />}
/>
```

#### Primary List Container
```tsx
<Card emphasis="primary" title="Recently Played" headerRight={<ButtonGhost ... />}>
  <RecentlyPlayedList />
</Card>
```

---

## 4) EmptyState Primitive (Implementation Spec)

### 4.1 Component API

```ts
export interface EmptyStateAction {
  label: string;
  onClick?: () => void;
  href?: string;
  variant?: 'primary' | 'secondary' | 'ghost'; // default: 'primary'
  iconLeft?: React.ReactNode;
}

export interface EmptyStateProps {
  intent: EmptyStateIntent;

  // Optional overrides (rare)
  title?: string;
  description?: string;
  hint?: string;
  icon?: React.ReactNode;

  actions?: EmptyStateAction[];
  secondaryActions?: EmptyStateAction[];

  // Layout
  size?: 'sm' | 'md' | 'lg'; // default: 'md'
  align?: 'left' | 'center'; // default: 'center'

  // Telemetry hooks (recommended)
  trackingId?: string;
}
```

### 4.2 Copy Registry (Single Source of Truth)
Implement a registry:

```ts
export const EMPTY_STATE_COPY: Record<EmptyStateIntent, {
  title: string;
  description: string;
  hint?: string;
  primaryAction?: { label: string };
  secondaryAction?: { label: string };
}> = { ... }
```

EmptyState renders:
- title/description from registry for `intent`
- actions default from registry unless passed explicitly

### 4.3 Visual Rules
- Use `emphasis="secondary"` card-like container or a dedicated surface.
- Show one primary action whenever possible.
- Never show more than 2 actions total.
- Use calm iconography; no heavy illustration required.

---

## 5) Empty State Copy (All Intents)

> Copy style: calm, human, actionable, no guilt.  
> All titles in Title Case, descriptions as full sentences.

### 5.1 AI DJ (`ai_dj`)
**Title:** Set the vibe  
**Description:** Describe a mood, genre, or era and I’ll build a playlist from your library.  
**Hint:** Try: “Late-night acid jazz with tight drums.”  
**Primary Action:** Use an Example  
**Secondary Action:** Browse Genres

### 5.2 Smart Mix (`smart_mix`)
**Title:** No mix yet  
**Description:** This mix needs a little listening history before it can shape itself to you.  
**Hint:** Play a few songs or import more music to get started.  
**Primary Action:** Play Something  
**Secondary Action:** Import Music

### 5.3 Playlist (`playlist`)
**Title:** This playlist is empty  
**Description:** Add a few tracks and Viib will keep it flowing.  
**Primary Action:** Add Songs  
**Secondary Action:** Start with Recently Played

### 5.4 Liked Songs (`liked_songs`)
**Title:** Nothing liked yet  
**Description:** Tap the heart on songs you love so they’re always close.  
**Primary Action:** Explore Songs  
**Secondary Action:** Play a Smart Mix

### 5.5 Liked Albums (`liked_albums`)
**Title:** No liked albums yet  
**Description:** Save albums you revisit so they’re one click away.  
**Primary Action:** Browse Albums  
**Secondary Action:** Explore Artists

### 5.6 Search No Results (`search_no_results`)
**Title:** No matches found  
**Description:** Try a different search or browse by artist, album, or genre.  
**Hint:** Tip: shorten your query or remove punctuation.  
**Primary Action:** Clear Search  
**Secondary Action:** Browse Genres

### 5.7 Queue Empty (`queue_empty`)
**Title:** Your queue is empty  
**Description:** Add tracks to line up what’s next.  
**Primary Action:** Add to Queue  
**Secondary Action:** Start a Smart Mix

### 5.8 Lyrics Unavailable (`lyrics_unavailable`)
**Title:** Lyrics unavailable  
**Description:** Lyrics aren’t available for this track right now.  
**Hint:** You can still view the queue or keep the visualizer up.  
**Primary Action:** Show Queue  
**Secondary Action:** Hide Panel

### 5.9 Recently Played (`recently_played`)
**Title:** Nothing played yet  
**Description:** Start listening and Viib will keep your recent rotation here.  
**Primary Action:** Play Something  
**Secondary Action:** Start a Smart Mix

### 5.10 Stats Not Enough Data (`stats_not_enough_data`)
**Title:** Not enough data yet  
**Description:** Listen a bit more and your stats will start to take shape.  
**Hint:** Your top artists and albums will appear after a few sessions.  
**Primary Action:** Play Something  
**Secondary Action:** View Library

### 5.11 Library Scanning (`library_scanning`)
**Title:** Scanning your library  
**Description:** Viib is checking your files and updating your collection.  
**Hint:** You can keep using the app—results will appear as they’re found.  
**Primary Action:** View Progress  
**Secondary Action:** Continue Browsing

---

## 6) Adoption Plan by Screen (Phase 2 Detail)

### 6.1 Home
**Hierarchy changes**
- Ensure only **one hero** (either one featured Smart Mix card or a hero strip).
- Convert **Total Songs / Albums / Artists** into `Card emphasis="utility"`.
- “Recently Played” becomes `primary` with clearer separation and reduced height density.

**Acceptance criteria**
- First fold clearly communicates: “What should I play?” + 1 featured entry point.

### 6.2 AI DJ
**Empty state**
- Replace center text with `<EmptyState intent="ai_dj" />`.
- Add “Use an Example” primary action that injects a random example prompt.
- Add optional “Browse Genres” secondary action.

**Acceptance criteria**
- New user can generate a playlist with one click (example) or one prompt.

### 6.3 Smart Mixes
**Empty behavior**
- If a mix has 0 tracks or cannot be generated, show `<EmptyState intent="smart_mix" />` within the card or in the mix details view.
- “Import Music” action routes to your import flow; “Play Something” routes to All Songs or a default mix.

### 6.4 Library Screens (Songs / Albums / Artists / Genres)
**Empty behavior**
- If filters yield no results: `<EmptyState intent="search_no_results" />`.
- If library truly empty (rare): reuse `smart_mix` or create a dedicated `library_empty` intent (optional).

### 6.5 Now Playing / Queue / Lyrics
**Empty behavior**
- Queue empty: `<EmptyState intent="queue_empty" />` in the queue panel.
- Lyrics unavailable: `<EmptyState intent="lyrics_unavailable" />`.

### 6.6 Stats
**Empty behavior**
- If listening time < threshold or dataset empty: `<EmptyState intent="stats_not_enough_data" />`.

---

## 7) Engineering Task Breakdown (PR-Friendly)

### Phase 1 — Foundation + APIs
**PR 1: Card API**
- [ ] Add `ViibEmphasis` type
- [ ] Add `emphasis`, `tone`, `gradient` props
- [ ] Implement gating rules (hero gradient only)
- [ ] Update existing card usages with minimal changes (default to primary)

**PR 2: EmptyState Primitive**
- [ ] Add `EmptyStateIntent`
- [ ] Add `EMPTY_STATE_COPY` registry
- [ ] Implement `EmptyState` component
- [ ] Add action rendering (primary + secondary)
- [ ] Add analytics hooks (optional)

**PR 3: Section Header (if applicable)**
- [ ] Add `emphasis` to `SectionHeader`
- [ ] Adjust spacing defaults: hero sections breathe more

### Phase 2 — Adoption
**PR 4: Home**
- [ ] Introduce 1 hero entry point
- [ ] Convert stat cards to utility
- [ ] Align section rhythm (spacing)

**PR 5: AI DJ**
- [ ] Replace default empty UI with `EmptyState(ai_dj)`
- [ ] Add “Use an Example” behavior

**PR 6: Library (Search + Filters)**
- [ ] Add `search_no_results` state
- [ ] Add `recently_played` state where relevant

**PR 7: Now Playing Panels**
- [ ] Queue empty state
- [ ] Lyrics unavailable state

**PR 8: Stats**
- [ ] Stats insufficient data state

### Phase 3 — Guardrails + QA
**PR 9: Dev warnings / checks**
- [ ] Warn if more than 1 hero card is rendered in a view container
- [ ] Warn if gradient used without hero emphasis
- [ ] Snapshot tests or visual QA notes

---

## 8) QA Checklist (Targeted)

### Hierarchy
- [ ] Only 1 hero per screen (Home, Stats)
- [ ] Utility cards do not compete with content cards
- [ ] Titles, subtitles follow the type scale

### Empty States
- [ ] Every empty state has at least 1 primary action (unless truly informational)
- [ ] Copy is consistent with registry (no one-off text)
- [ ] No empty state traps the user (always a path forward)

### Accessibility
- [ ] Buttons focus-visible clearly
- [ ] Empty state actions keyboard reachable
- [ ] Contrast remains ≥ 4.5:1 for text

---

## 9) Optional Enhancements (After These Phases)
- Add “Experience Modes” (`focus | balanced | vibe`) to modulate motion and visuals.
- Add a dev “Design Drift Dashboard” showing token usage and rule violations.
- Add templated “Hero strip” component for Home and Stats.

---

## 10) Appendix — Example Prompt Set (AI DJ)
Use these as “Use an Example” pool:
- Late-night acid jazz with tight drums
- Chill electronic for focused work, no vocals
- 90s alternative with upbeat energy
- Soulful grooves and warm basslines
- Instrumental hip-hop, dusty and calm
