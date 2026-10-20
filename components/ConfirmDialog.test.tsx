// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { ConfirmDialog } from './ConfirmDialog';

it('starts on Cancel, traps destructive-action focus, blocks busy Escape and restores the trigger', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const visible = vi.spyOn(HTMLElement.prototype, 'offsetParent', 'get').mockImplementation(function () { return this.parentElement; });
  const opener = document.createElement('button');
  const container = document.createElement('div');
  document.body.append(opener, container); opener.focus();
  const root = createRoot(container);
  const cancel = vi.fn();
  const confirm = vi.fn();
  const render = (open: boolean, busy = false) => act(async () => root.render(<ConfirmDialog isOpen={open} isLoading={busy} title="Delete fixture" message="Review the synthetic deletion" variant="danger" onCancel={cancel} onConfirm={confirm} />));
  try {
    await render(true);
    const [cancelButton, confirmButton] = container.querySelectorAll('button');
    expect(document.activeElement).toBe(cancelButton);
    cancelButton.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }));
    expect(document.activeElement).toBe(confirmButton);
    confirmButton.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }));
    expect(document.activeElement).toBe(cancelButton);
    await render(true, true);
    const dialog = container.querySelector('[role="dialog"]')!;
    await act(async () => dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(cancel).not.toHaveBeenCalled(); expect(confirm).not.toHaveBeenCalled();
    await render(true);
    await act(async () => dialog.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(cancel).toHaveBeenCalledOnce(); expect(confirm).not.toHaveBeenCalled();
    await render(false);
    expect(document.activeElement).toBe(opener);
  } finally {
    await act(async () => root.unmount());
    container.remove(); opener.remove(); visible.mockRestore();
  }
});
