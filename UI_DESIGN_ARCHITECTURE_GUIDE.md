# ViiB MediaHub — UI Design Architecture Guide (Design System v1)

Purpose: this document is the “source of truth” for how UI design elements work in ViiB MediaHub, where they are implemented, and how to extend them without breaking the design system.

This app is a React + TypeScript + Vite UI styled with Tailwind, compiled into a native desktop app via Wails (Go backend + WebView2). The UI design system is implemented primarily through Tailwind theme tokens and a small set of reusable primitives.

---

## 1) Sources of Truth (Start Here)

### Theme tokens (colors, typography, motion)
- **File:** `tailwind.config.js`
- **What lives here:**
  - Core DS colors (`surface-*`, `text-*`, `brand`, `accent-*`, `success/warning/error`)
  - DS type scale (`text-display`, `text-section`, `text-card`, `text-body`, `text-meta`)
  - Shared motion keyframes/animations (`fade-in`, `slide-up`, `scale-in`, etc.)

### Semantic token helpers + allowlisted literal colors
- **File:** `components/ui/tokens.ts`
- **What lives here:**
  - A semantic naming layer (e.g., “playback accent” vs “green”)
  - Mappings to token classes (e.g., ring/background by accent)
  - **Allowlisted literal color values** for canvas/WebAudio-only usage

### Base document styling & reduced-motion
- **File:** `index.css`
- **What lives here:**
  - Base font family
  - Global focus-visible outline
  - Reduced motion override
  - Scrollbar styling

### App shell layout
- **File:** `components/Layout.tsx`
- **What lives here:**
  - Sidebar + main content + queue panel + bottom player
  - Global context menu layer
  - Global toast container
  - Right-click prevention to support custom context menus

---

## 2) Color System

The DS is **token-first**: use Tailwind classes that reference DS tokens instead of raw hex/default Tailwind palette colors.

### 2.1 Core surfaces (background + elevation)
Defined in `tailwind.config.js` under `colors.surface`:
- `surface-0` (Background): `#0B0B0E`
- `surface-1` (Surface Dark): `#121216`
- `surface-2` (Surface Raised): `#18181E`
- `surface-3` (Dividers): `#24242B`

Common usage patterns:
- App background: `bg-surface-0`
- Panels/cards: `bg-surface-1` or `bg-surface-2`
- Dividers/rings: `border-surface-3`, `ring-surface-3`

### 2.2 Text colors
Defined in `tailwind.config.js` under `colors.text`:
- `text-text-main`: `rgba(255,255,255,0.9)`
- `text-text-secondary`: `#B8BAC6`
- `text-text-subtle`: `#7A7D8C`

### 2.3 Brand + accents
Defined in `tailwind.config.js`:
- `brand` (Viib Purple): `#9B5CFF`

Accents (use intentionally; avoid “rainbow UI”):
- `accent-purple`: `#9B5CFF`
- `accent-green`: `#3EE089` (playback state)
- `accent-orange`: `#FF9F43` (discovery)
- `accent-blue`: `#4EA1FF` (stats/info)
- `accent-crimson`: `#FF5D5D` (destructive/error)

Status aliases:
- `success`: `#3EE089`
- `warning`: `#FF9F43`
- `error`: `#FF5D5D`

### 2.4 Semantic accent mapping (preferred)
Instead of hard-coding “green/orange/blue”, prefer semantic mappings in `components/ui/tokens.ts`:
- **Accent names:** `brand | playback | discovery | stats | destructive`
- **Helpers:**
  - `accentToRingClass(accent)`
  - `accentToBgClass(accent)`

This makes intent readable and reduces drift.

### 2.5 When literal colors are allowed
Literal color strings are **allowed only** when required:
- Canvas/WebAudio visualizers (dynamic alpha, gradients, etc.)
- Certain base CSS rules in `index.css` (currently not enforced by TSX checks)

For canvas/WebAudio, use the allowlist:
- `VIIB_COLOR_VALUES` and `VIIB_COLOR_RGB` in `components/ui/tokens.ts`
- Helper: `rgbaFromRgb(rgb, alpha)`

---

## 3) Typography System

Defined in `tailwind.config.js` under `theme.extend.fontSize`:
- `text-display`: 36px / 1.25 / 600
- `text-section`: 24px / 1.25 / 500
- `text-card`: 18px / 1.25 / 500
- `text-body`: 15px / 1.5 / 400
- `text-meta`: 13px / 1.5 / 400

Guidelines:
- Page titles: `text-display`
- Section headers: `text-section`
- Card/list item titles: `text-card`
- Default copy: `text-body`
- Secondary metadata: `text-meta` + `text-text-secondary`

Font family is **Inter** (see `index.css` and Google Fonts import in `index.html`).

---

## 4) Layout & Spacing

### 4.1 Shell layout
- **File:** `components/Layout.tsx`
- Key structure:
  - Left: `Sidebar`
  - Center: `main` scroll container
  - Right: floating `Queue`
  - Bottom: `Player`
  - Global overlays: `ContextMenu`, `ToastContainer`

### 4.2 Page wrappers (Phase 7)
- **File:** `components/ui/Page.tsx`

Use these wrappers to keep consistent spacing:
- `Page`
  - Default: `p-8 h-full`
  - Default player padding: `pb-32`
  - Default entrance: `animate-fade-in`
  - Use `withPlayerPadding={false}` only if a screen already accounts for the player in its list/container.

- `PageHeader`
  - Standard header rhythm: title + optional subtitle + optional actions
  - Layout: column on mobile, row on desktop

- `ListHeader`
  - Standard wrapper for virtualized list headers (e.g., `react-virtuoso` `Header`)
  - Default: `p-8 pb-0`

Remaining spacing work is typically **row-level and empty-state wrappers** (e.g., `px-*`/`py-*` drift across lists).

---

## 5) UI Primitives (Reusable Components)

These primitives are the preferred building blocks. When creating new UI, start here before writing bespoke Tailwind.

### 5.1 `Button`
- **File:** `components/ui/Button.tsx`
- Variants: `primary | secondary | ghost`
- Accent support: `accent?: ViibAccent` (semantic mapping via `accentToBgClass`)
- Focus treatment: consistent `focus-visible:ring-*` with `ring-offset-surface-0`

### 5.2 `Card`
- **File:** `components/ui/Card.tsx`
- Default: `bg-surface-1`, `rounded-xl`, `ring-1`
- Optional `interactive` hover motion (reduced-motion safe)

### 5.3 `TextInput`
- **File:** `components/ui/TextInput.tsx`
- Ring + focus-within treatment, uses DS text sizes (`text-body`)

### 5.4 `Chip`
- **File:** `components/ui/Chip.tsx`
- Used for filters/toggles
- Selected state uses `accentToRingClass` for semantic accent rings

### 5.5 `Menu` / `MenuItem`
- **File:** `components/ui/Menu.tsx`
- Provides:
  - `role="menu"` / `role="menuitem"`
  - Shared keyboard navigation: ArrowUp/Down, Home/End
  - Typeahead
  - Escape/Tab close hooks via `onRequestClose`

---

## 6) Interaction Systems

### 6.1 Context menus
- **Global layer:** `components/ContextMenu.tsx`
- **Menu implementations:** `components/context-menus/*` (Song/Album/Artist/Playlist/SmartMix/QueueItem)

Key behaviors:
- Click-outside dismissal
- Escape dismissal
- Roving focus + typeahead + submenu traversal for keyboard users

### 6.2 Toast notifications
- **File:** `components/Toast.tsx`
- Toast types: `success | error | info | warning`
- Color mapping uses DS tokens (`bg-error`, `bg-warning`, etc.)

---

## 7) Motion & Accessibility

### 7.1 Motion
- DS animations live in `tailwind.config.js` (e.g., `animate-fade-in`)
- Reduced motion policy is enforced globally in `index.css` (`prefers-reduced-motion`)

### 7.2 Accessibility baseline
- Global focus-visible outline in `index.css`
- Menus and context menus use ARIA roles and keyboard navigation patterns

---

## 8) Governance & Enforcement (How We Prevent Drift)

### 8.1 Automated checks
- `npm run check:palette`
  - Scans for default Tailwind palette usage (`text-gray-*`, `bg-blue-*`, etc.)
- `npm run check:raw-colors`
  - Blocks raw hex colors in TSX (keeps UI tokenized)
- `npm run typecheck`
  - `tsc --noEmit`
- `npm run build`
  - Vite production build

### 8.2 Practical rules of thumb
- Prefer DS tokens (`bg-surface-*`, `text-text-*`, `brand`, `accent-*`) over literals.
- Prefer primitives (`Button`, `Card`, `TextInput`, `Chip`, `Menu`) over bespoke patterns.
- Avoid dynamic class construction from untrusted/variable values.
  - If you need variability (accent, status), add a **mapping function** in `components/ui/tokens.ts`.
- Use `Page`/`PageHeader`/`ListHeader` for consistent spacing.

---

## 9) Adding a New Screen (Checklist)

1. Wrap in `Page` (and usually a `PageHeader`).
2. Use DS type scale (`text-display`/`text-section`/etc.).
3. Use DS tokens for all colors.
4. Use primitives for controls.
5. Ensure keyboard/focus behavior and add `aria-label` for icon-only buttons.
6. Run: `npm run check:palette && npm run check:raw-colors && npm run typecheck && npm run build`.

---

## 10) Key File Index

Theme + tokens:
- `tailwind.config.js`
- `components/ui/tokens.ts`
- `index.css`

Layout + page structure:
- `components/Layout.tsx`
- `components/ui/Page.tsx`

Primitives:
- `components/ui/Button.tsx`
- `components/ui/Card.tsx`
- `components/ui/TextInput.tsx`
- `components/ui/Chip.tsx`
- `components/ui/Menu.tsx`

Interaction systems:
- `components/ContextMenu.tsx`
- `components/context-menus/*`
- `components/Toast.tsx`
