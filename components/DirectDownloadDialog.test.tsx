// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
vi.mock('../store', () => ({ useStore: (selector: (state: { spotifyConnected: boolean; spotifyAccountProduct: string }) => unknown) => selector({ spotifyConnected: true, spotifyAccountProduct: 'premium' }) }));
vi.mock('./SpotifyAccountNotice', () => ({ SpotifyAccountNotice: () => null }));
import { DirectDownloadDialog } from './DirectDownloadDialog';

it('names the dialog and URL field, dismisses from its named close control or Escape, and restores the opener', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const visible = vi.spyOn(HTMLElement.prototype, 'offsetParent', 'get').mockImplementation(function () { return this.parentElement; });
  const opener = document.createElement('button');
  const container = document.createElement('div');
  document.body.append(opener, container); opener.focus();
  const root = createRoot(container);
  const close = vi.fn();
  try {
    await act(async () => root.render(<DirectDownloadDialog isOpen onClose={close} />));
    const dialog = container.querySelector('[role="dialog"]')!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(document.getElementById(dialog.getAttribute('aria-labelledby')!)?.textContent).toBe('Direct Download');
    const input = container.querySelector('input')!;
    expect(container.querySelector('label')?.htmlFor).toBe(input.id);
    const closeButton = container.querySelector<HTMLButtonElement>('button[aria-label="Close direct download dialog"]')!;
    expect(document.activeElement).toBe(closeButton);
    await act(async () => closeButton.click());
    expect(close).toHaveBeenCalledOnce();
    await act(async () => input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(close).toHaveBeenCalledTimes(2);
    await act(async () => root.render(<DirectDownloadDialog isOpen={false} onClose={close} />));
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(document.activeElement).toBe(opener);
  } finally {
    await act(async () => root.unmount());
    container.remove(); opener.remove(); visible.mockRestore();
  }
});
