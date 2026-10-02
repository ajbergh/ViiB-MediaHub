// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { beforeEach, expect, it, vi } from 'vitest';

const mocks = vi.hoisted(() => ({ state: {} as any, songs: [] as any[] }));
vi.mock('../store', () => ({ useStore: (selector: any) => selector(mocks.state) }));
vi.mock('react-router', () => ({ useNavigate: () => vi.fn() }));
vi.mock('../components/home/useHomeContent', () => ({ useHomeContent: () => ({ songs: mocks.songs }) }));
vi.mock('../components/home/HomeSearchBar', () => ({ HomeSearchBar: () => null }));
vi.mock('../components/EmptyState', () => ({ EmptyLibrary: () => <div>Empty library</div> }));
vi.mock('../components/home/layouts/HomeShelvesLayout', () => ({ HomeShelvesLayout: () => <div>Library shelves</div> }));
vi.mock('../components/home/layouts/HomeDashboardLayout', () => ({ HomeDashboardLayout: () => <div>Library dashboard</div> }));
vi.mock('../components/home/layouts/HomeCoverWallLayout', () => ({ HomeCoverWallLayout: () => <div>Library covers</div> }));
import { Home } from './Home';

beforeEach(() => {
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
  mocks.songs = [];
  mocks.state = { homeLayoutVariant: 'shelves', isLibraryInitializing: true, isScanning: false, refreshSmartMixes: vi.fn() };
});

it('shows loading until an empty library has finished initializing and scanning', async () => {
  const container = document.createElement('div');
  const root = createRoot(container);
  const render = () => act(async () => root.render(<Home />));
  try {
    await render();
    expect(container.querySelector('[role="status"]')?.textContent).toContain('Loading library');
    expect(container.textContent).not.toContain('Empty library');
    mocks.state.isLibraryInitializing = false;
    mocks.state.isScanning = true;
    await render();
    expect(container.querySelector('[aria-busy="true"]')).not.toBeNull();
    expect(container.textContent).not.toContain('Empty library');
    mocks.state.isScanning = false;
    await render();
    expect(container.textContent).toContain('Empty library');
    expect(container.querySelector('[role="status"]')).toBeNull();
  } finally { await act(async () => root.unmount()); }
});

it('renders available songs during scanning without regenerating mixes on visits', async () => {
  mocks.songs = [{ id: 'song' }];
  mocks.state.isScanning = true;
  const container = document.createElement('div');
  let root = createRoot(container);
  try {
    await act(async () => root.render(<Home />));
    expect(container.textContent).toContain('Library shelves');
    expect(container.querySelector('[role="status"]')).toBeNull();
    await act(async () => root.unmount());
    root = createRoot(container);
    await act(async () => root.render(<Home />));
    expect(mocks.state.refreshSmartMixes).not.toHaveBeenCalled();
  } finally { await act(async () => root.unmount()); }
});
