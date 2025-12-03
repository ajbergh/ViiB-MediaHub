# ViiB MediaHub AI Instructions

## Project Overview
ViiB MediaHub is a local media player application with a React frontend and Go backend. It compiles to a single executable but runs as a web app in the browser.

## Architecture
- **Frontend**: React 19, TypeScript, Vite.
  - **State Management**: Zustand (`store.ts`, `slices/`).
  - **Data Persistence**: IndexedDB (via `idb`) for the song library; `localStorage` (via Zustand persist) for settings.
  - **Styling**: Tailwind CSS via CDN (configured in `index.html` with shimmer animations).
  - **Routing**: `react-router-dom` with `MemoryRouter`.
- **Backend**: Go 1.22.
  - **Server**: `chi` router.
  - **Database**: SQLite (`mattn/go-sqlite3`).
  - **Entry Point**: `backend/cmd/viib/main.go`.

## Development Workflow
- **Start Dev Server**: Run `scripts/dev.ps1` in PowerShell. This starts both the Go backend (port 8080) and Vite frontend (port 3000).
- **Build**: `scripts/build.ps1`.
- **Environment**:
  - Go requires `CGO_ENABLED=1` (for SQLite).
  - Frontend proxies `/api` requests to `http://127.0.0.1:8080` in development.

## Code Conventions

### Frontend
- **API Communication**: Use `services/api.ts` for all backend interactions. Types in `api.ts` should match backend structs.
- **Styling**: Use Tailwind utility classes. Note that Tailwind is loaded via CDN in `index.html` with a custom config (colors like `surface-0`, `brand`, etc.).
- **State**:
  - Use `useStore` from `store.ts` for global UI state.
  - **Do not** store the full song library in Zustand state if possible; it is managed via IndexedDB to handle large libraries.
- **Components**: Functional components with TypeScript interfaces for props.
- **Toast Notifications**: Use `showToast({ type, message })` from the store for user feedback.
- **Loading States**: Use skeleton components from `components/Skeleton.tsx`.
- **Empty States**: Use empty state components from `components/EmptyState.tsx`.
- **Keyboard Navigation**: Global shortcuts handled by `hooks/useKeyboardNavigation.ts`.

### Backend
- **Structure**: Follow Standard Go Project Layout.
  - `cmd/viib`: Main application entry point.
  - `internal/`: Private application code (api, audio, db, server).
- **Database**: Use the `db` package (`internal/db`) for all database interactions.
- **API**: Define routes in `internal/server/server.go` and handlers in `internal/api/`.

## Key Files
- `App.tsx`: Main frontend entry and routing.
- `store.ts`: Zustand store configuration.
- `services/api.ts`: API client and type definitions.
- `backend/cmd/viib/main.go`: Backend entry point.
- `backend/internal/db/db.go`: Database schema and methods.
- `index.html`: Tailwind configuration and entry point.

## UX Components
- `components/Skeleton.tsx`: Loading skeleton components (SkeletonAlbumCard, SkeletonTrackRow, etc.)
- `components/Toast.tsx`: Toast notification system (ToastContainer, showToast)
- `components/EmptyState.tsx`: Empty state displays (EmptyLibrary, EmptyPlaylists, etc.)
- `hooks/useKeyboardNavigation.ts`: Global keyboard shortcuts (Space, arrows, Q, E, Escape)
