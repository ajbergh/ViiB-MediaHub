// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot, type Root } from 'react-dom/client';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { MobileTopBar } from './MobileTopBar';

let root: Root;
let host: HTMLDivElement;

beforeEach(() => {
  host = document.createElement('div');
  root = createRoot(host);
  (globalThis as any).IS_REACT_ACT_ENVIRONMENT = true;
});

afterEach(async () => {
  await act(async () => root.unmount());
});

it.each([
  ['/smart-playlists', 'AI Smart Mix'],
  ['/smart-mix/genre', 'Smart Mix'],
  ['/dj', 'DJ Mode'],
])('shows the correct feature name for %s', async (path, title) => {
  await act(async () => {
    root.render(
      <MemoryRouter initialEntries={[path]}>
        <MobileTopBar onOpenMenu={vi.fn()} />
      </MemoryRouter>
    );
  });

  expect(host.querySelector('header span')?.textContent).toBe(title);
  expect(host.textContent).not.toContain('AI DJ');
});