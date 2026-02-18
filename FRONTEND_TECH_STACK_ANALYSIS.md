# ViiB MediaHub - Frontend Tech Stack Analysis

**Date:** January 29, 2026  
**Author:** GitHub Copilot (Claude Opus 4.5)  
**Purpose:** Deep analysis of proposed tech stack migration for cross-platform desktop application

---

## 📊 Executive Summary

This document analyzes whether migrating ViiB MediaHub's frontend from the current stack to the proposed stack would improve the project, UI capabilities, and overall application quality while maintaining cross-platform standalone builds via Wails.

| Aspect | Current Stack | Proposed Stack | Recommendation |
|--------|--------------|----------------|----------------|
| **Framework** | React 19 + Vite 6 | Next.js 15 + React 19 | ⚠️ **Cautious - See Analysis** |
| **TypeScript** | 5.8 | 5.7 | ✅ Already ahead |
| **CSS** | Tailwind 3.4 (CDN) | Tailwind 3.4 | ✅ Keep current |
| **Theming** | Custom tokens | next-themes 0.4 | ⚠️ Partial fit |
| **UI Library** | Custom + Lucide | Tremor + Radix UI | ✅ **Recommended** |
| **Charts** | Custom SVG | Recharts 2.15 | ✅ **Recommended** |

**Overall Verdict:** Adopt **Radix UI** and **Recharts** selectively. **Do NOT migrate to Next.js** for a Wails desktop app.

---

## 🔍 Current Stack Analysis

### What ViiB MediaHub Currently Uses

| Package | Version | Purpose |
|---------|---------|---------|
| `react` | ^19.2.0 | UI framework |
| `react-dom` | ^19.2.0 | DOM rendering |
| `react-router-dom` | ^7.9.6 | Client-side routing |
| `zustand` | ^5.0.8 | State management |
| `vite` | ^6.2.0 | Build tool + dev server |
| `tailwindcss` | ^3.4.13 | Utility CSS |
| `lucide-react` | ^0.554.0 | Icons |
| `react-virtuoso` | 4.12.3 | Virtual scrolling |
| `idb` | 8.0.0 | IndexedDB wrapper |
| `typescript` | ~5.8.2 | Type system |

### Current Architecture Strengths

1. **Optimized for Wails Embedding**
   - Vite produces static assets that embed perfectly into Go binary
   - No server-side rendering complexity
   - Single-page app loads instantly from memory

2. **Custom Design System**
   - Well-defined tokens in `components/ui/tokens.ts`
   - Semantic color system: `surface-0/1/2/3`, `text-main/secondary/subtle`
   - Accent colors: `brand`, `playback`, `discovery`, `stats`, `destructive`
   - Reusable components: `Button`, `Card`, `Page`, `Chip`, `Menu`

3. **Performance Optimized**
   - Zustand slices with selective subscriptions
   - Virtual scrolling for large libraries
   - Crossfade audio with dual audio elements
   - SSE for real-time updates

4. **Bundle Size**
   - Current build is lean (~22.4 MB total with Go backend)
   - No framework overhead beyond React core

---

## 🆚 Proposed Stack Deep Dive

### 1. Next.js 15

#### ❌ **NOT RECOMMENDED for Wails Desktop Apps**

**Critical Incompatibilities:**

| Issue | Impact | Severity |
|-------|--------|----------|
| Server Components | Next.js 15 defaults to RSC; Wails has no Node.js runtime | 🔴 Critical |
| API Routes | Next.js API routes won't work; ViiB already has Go backend | 🔴 Critical |
| File-system Routing | Adds complexity; current react-router works perfectly | 🟡 Medium |
| Build Output | `next export` deprecated; static output is second-class | 🔴 Critical |
| Bundle Size | Next.js adds ~150-200KB to client bundle | 🟡 Medium |
| HMR in Wails | Next.js dev server doesn't integrate with Wails proxy | 🟠 High |

**Why This Matters:**

```
Current Architecture (Works Well):
┌─────────────────────────────────────────────────┐
│              Wails Application                   │
│  ┌───────────────┐      ┌───────────────────┐   │
│  │   WebView2    │ ←──→ │   Go HTTP Server  │   │
│  │  (Vite SPA)   │      │   (chi router)    │   │
│  └───────────────┘      └───────────────────┘   │
│        ↓                                         │
│  Embedded static assets (dist/)                  │
└─────────────────────────────────────────────────┘

With Next.js (Problematic):
┌─────────────────────────────────────────────────┐
│              Wails Application                   │
│  ┌───────────────┐      ┌───────────────────┐   │
│  │   WebView2    │ ←──→ │   Go HTTP Server  │   │
│  │  (Next.js?)   │      │   (chi router)    │   │
│  └───────────────┘      └───────────────────┘   │
│        ↓                                         │
│  ❌ Where does Node.js run?                      │
│  ❌ Server Components can't hydrate              │
│  ❌ API routes conflict with Go backend          │
└─────────────────────────────────────────────────┘
```

**Verdict:** Next.js is designed for server-deployed web applications. ViiB MediaHub is a desktop app with a Go backend. **Do not migrate to Next.js.**

---

### 2. React 19

#### ✅ **ALREADY USING**

ViiB MediaHub is already on React ^19.2.0, which includes:
- Concurrent features
- Automatic batching
- `useTransition` for non-blocking updates
- Improved suspense boundaries

**Action:** No change needed. Continue using React 19.

---

### 3. TypeScript 5.7

#### ✅ **ALREADY AHEAD**

ViiB MediaHub uses TypeScript ~5.8.2, which is newer than 5.7.

**TypeScript 5.8 Benefits You Already Have:**
- Better type narrowing
- Improved module resolution
- Enhanced `satisfies` operator
- Better performance

**Action:** No change needed. Already using a newer version.

---

### 4. Tailwind CSS 3.4

#### ✅ **ALREADY USING**

Currently using Tailwind 3.4.13 with a comprehensive custom configuration:

```javascript
// Current tailwind.config.js highlights
colors: {
  surface: { 0: '#0B0B0E', 1: '#121216', 2: '#18181E', 3: '#24242B' },
  text: { main: 'rgba(255,255,255,0.9)', secondary: '#B8BAC6', subtle: '#7A7D8C' },
  brand: { DEFAULT: '#9B5CFF', hover: '#9B5CFF' },
  accent: { purple: '#9B5CFF', green: '#3EE089', orange: '#FF9F43', blue: '#4EA1FF', crimson: '#FF5D5D' }
}
```

**Recommendation:** Consider migrating from CDN to local installation for:
- Better build optimization (PurgeCSS)
- Offline development support
- Smaller production bundles

**Action:** Minor improvement possible, but current setup works.

---

### 5. next-themes 0.4

#### ⚠️ **PARTIAL FIT**

`next-themes` is specifically designed for Next.js applications with SSR/SSG. ViiB MediaHub:
- Is a SPA with no server-side rendering
- Has a fixed dark theme (no theme switching needed currently)
- Uses Zustand for all state management

**If Theme Switching is Desired:**

Better alternatives for Vite SPAs:
1. **CSS Variables + Zustand** (current approach extensible)
2. **`react-use-theme`** - lightweight, framework-agnostic
3. **Custom context** - simple toggle with localStorage

**Current Implementation is Sufficient:**
```typescript
// tailwind.config.js already defines dark-first design
colors: {
  surface: { 0: '#0B0B0E', ... } // Dark theme built-in
}
```

**Action:** Skip `next-themes`. Current design system handles theming via CSS variables.

---

### 6. Tremor

#### ⚠️ **PARTIAL FIT - Better Alternatives Exist**

**What Tremor Offers:**
- Pre-built dashboard components (cards, charts, metrics)
- Opinionated design language
- Built on Radix UI primitives

**Issues for ViiB MediaHub:**

| Concern | Impact |
|---------|--------|
| Bundle size | Tremor adds ~200KB+ gzipped |
| Design conflict | ViiB has established design tokens that would conflict |
| Overkill | Most Tremor components duplicate existing ViiB components |
| Dependencies | Brings in Recharts, Radix, and its own styling layer |

**What ViiB Already Has:**
- `Card` component with variants (default, hero, utility)
- `StatCard` component in Stats.tsx
- Custom button system with accent colors
- Consistent design language

**Better Approach:** Cherry-pick **Radix UI** primitives directly instead of Tremor.

**Action:** Skip Tremor. Use Radix UI directly for primitives.

---

### 7. Radix UI

#### ✅ **RECOMMENDED - Selective Adoption**

**Why Radix UI is a Good Fit:**

| Benefit | Explanation |
|---------|-------------|
| Unstyled primitives | Works with existing Tailwind design system |
| Accessibility | WAI-ARIA compliant out of the box |
| Composable | Only install what you need |
| No visual conflicts | Headless components don't impose design |
| Small bundles | Each primitive is ~5-15KB |

**Recommended Radix Primitives for ViiB:**

| Component | Use Case in ViiB |
|-----------|------------------|
| `@radix-ui/react-dialog` | ConfirmDialog, FirstLaunchDialog, DirectDownloadDialog |
| `@radix-ui/react-dropdown-menu` | Context menus (currently custom) |
| `@radix-ui/react-slider` | Volume, EQ, crossfader sliders |
| `@radix-ui/react-tooltip` | Hover tooltips throughout |
| `@radix-ui/react-popover` | Dropdown panels (EQ, Sleep Timer) |
| `@radix-ui/react-tabs` | Settings page tabs |
| `@radix-ui/react-scroll-area` | Custom scrollbars |

**Migration Example - Current vs Radix:**

```tsx
// Current: Custom slider in Player.tsx
<input
  type="range"
  min="0"
  max={duration || 100}
  value={currentTime}
  onChange={(e) => seek(parseFloat(e.target.value))}
  className="..."
/>

// With Radix: Accessible, styleable slider
import * as Slider from '@radix-ui/react-slider';

<Slider.Root value={[currentTime]} max={duration} onValueChange={([v]) => seek(v)}>
  <Slider.Track className="bg-surface-3 h-1 rounded-full">
    <Slider.Range className="bg-brand h-full rounded-full" />
  </Slider.Track>
  <Slider.Thumb className="w-3 h-3 bg-white rounded-full" />
</Slider.Root>
```

**Bundle Impact:** ~50-80KB total for recommended primitives (acceptable).

**Action:** ✅ Adopt Radix UI selectively for dialog, slider, menu, and tooltip components.

---

### 8. Recharts 2.15

#### ✅ **RECOMMENDED for Stats Page**

**Current State:**
- Stats.tsx uses custom components (`StatCard`, `TopItem`)
- No actual charts/graphs currently
- Listening history visualization is basic

**What Recharts Would Enable:**

| Chart Type | Use Case |
|------------|----------|
| `AreaChart` | Listening activity over time |
| `BarChart` | Top artists/albums/genres |
| `PieChart` | Genre distribution |
| `LineChart` | Play trends |

**Integration Example:**

```tsx
// Stats.tsx - Listening activity chart
import { AreaChart, Area, XAxis, YAxis, Tooltip, ResponsiveContainer } from 'recharts';

const ListeningChart: React.FC<{ data: ActivityData[] }> = ({ data }) => (
  <ResponsiveContainer width="100%" height={200}>
    <AreaChart data={data}>
      <defs>
        <linearGradient id="brandGradient" x1="0" y1="0" x2="0" y2="1">
          <stop offset="5%" stopColor="#9B5CFF" stopOpacity={0.8}/>
          <stop offset="95%" stopColor="#9B5CFF" stopOpacity={0}/>
        </linearGradient>
      </defs>
      <XAxis dataKey="date" stroke="#7A7D8C" />
      <YAxis stroke="#7A7D8C" />
      <Tooltip 
        contentStyle={{ 
          backgroundColor: '#18181E', 
          border: '1px solid #24242B',
          borderRadius: '8px'
        }} 
      />
      <Area 
        type="monotone" 
        dataKey="minutes" 
        stroke="#9B5CFF" 
        fillOpacity={1} 
        fill="url(#brandGradient)" 
      />
    </AreaChart>
  </ResponsiveContainer>
);
```

**Bundle Impact:** ~150KB gzipped (acceptable for rich visualization).

**Action:** ✅ Add Recharts for the Stats page. Enhances data visualization significantly.

---

## 📋 Recommendation Summary

### ✅ DO Adopt

| Package | Reason | Priority |
|---------|--------|----------|
| **@radix-ui/react-dialog** | Better accessibility, animations | High |
| **@radix-ui/react-slider** | Accessible volume/EQ controls | High |
| **@radix-ui/react-dropdown-menu** | Replace custom context menus | Medium |
| **@radix-ui/react-tooltip** | Consistent hover hints | Medium |
| **recharts** | Stats page visualization | Medium |

### ❌ DO NOT Adopt

| Package | Reason |
|---------|--------|
| **Next.js 15** | Incompatible with Wails desktop architecture |
| **next-themes** | Requires Next.js; overkill for fixed dark theme |
| **Tremor** | Too opinionated; conflicts with existing design system |

### ⚠️ Consider Later

| Package | When |
|---------|------|
| **Framer Motion** | When adding more complex animations |
| **TanStack Table** | If building sortable/filterable data tables |
| **Vaul** | For drawer components (mobile-first, if targeting mobile) |

---

## 🔧 Recommended Migration Path

### Phase 1: Radix UI Primitives (Low Risk)

```bash
npm install @radix-ui/react-dialog @radix-ui/react-slider @radix-ui/react-tooltip
```

**Files to Update:**
- `components/ConfirmDialog.tsx` → Use `@radix-ui/react-dialog`
- `components/Player.tsx` → Use `@radix-ui/react-slider` for volume
- `components/Equalizer.tsx` → Use `@radix-ui/react-slider` for EQ bands
- `components/dj/DJMixer.tsx` → Use `@radix-ui/react-slider` for crossfader

### Phase 2: Recharts for Stats (Medium Effort)

```bash
npm install recharts
```

**Files to Update:**
- `pages/Stats.tsx` → Add listening activity charts
- New: `components/charts/ListeningChart.tsx`
- New: `components/charts/GenreDistribution.tsx`

### Phase 3: Context Menu Migration (Optional)

```bash
npm install @radix-ui/react-dropdown-menu @radix-ui/react-context-menu
```

**Files to Update:**
- `components/ContextMenu.tsx`
- `components/context-menus/*.tsx`

---

## 📊 Bundle Size Projection

| Change | Size Impact |
|--------|-------------|
| Current bundle | ~850KB gzipped |
| + Radix primitives | +60KB |
| + Recharts | +150KB |
| **Projected total** | ~1,060KB gzipped |

This is acceptable for a desktop application with no bandwidth constraints.

---

## 🎯 Final Verdict

**Keep:**
- React 19 + Vite 6 (optimal for Wails)
- Tailwind CSS 3.4 (well-configured)
- Zustand (excellent for this app)
- react-router-dom (simple, works)
- TypeScript 5.8 (already ahead)

**Add:**
- Radix UI (selective primitives)
- Recharts (Stats visualization)

**Avoid:**
- Next.js (architectural mismatch)
- Tremor (design conflict, bloat)
- next-themes (Next.js dependency)

---

## 📚 References

- [Wails Documentation](https://wails.io/docs/introduction)
- [Radix UI Primitives](https://www.radix-ui.com/primitives)
- [Recharts Documentation](https://recharts.org/en-US/)
- [Vite vs Next.js for SPAs](https://vitejs.dev/guide/why.html)
- [React 19 Release Notes](https://react.dev/blog/2024/04/25/react-19)

---

*This analysis was generated based on the actual ViiB MediaHub codebase structure, Wails integration architecture, and cross-platform desktop application requirements.*
