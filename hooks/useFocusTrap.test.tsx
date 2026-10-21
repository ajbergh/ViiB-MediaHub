// @vitest-environment jsdom
import React, { act } from 'react';
import { createRoot } from 'react-dom/client';
import { expect, it, vi } from 'vitest';
import { useFocusTrap } from './useFocusTrap';

it('keeps focus and the original opener across callback updates, cycles Tab, and uses the latest Escape action', async () => {
  Object.assign(globalThis, { IS_REACT_ACT_ENVIRONMENT: true });
  const visible = vi.spyOn(HTMLElement.prototype, 'offsetParent', 'get').mockImplementation(function (this: HTMLElement) { return this.parentElement; });
  const opener = document.createElement('button');
  const container = document.createElement('div');
  document.body.append(opener, container);
  opener.focus();
  const root = createRoot(container);
  function Harness({ escape }: { escape: () => void }) {
    const ref = useFocusTrap<HTMLDivElement>(true, escape);
    return <div ref={ref} tabIndex={-1}><button>First</button><input aria-label="Middle" /><button>Last</button></div>;
  }
  const original = vi.fn();
  const latest = vi.fn();
  try {
    await act(async () => root.render(<Harness escape={original} />));
    const [first, last] = container.querySelectorAll('button');
    const middle = container.querySelector('input')!;
    expect(document.activeElement).toBe(first);
    middle.focus();
    await act(async () => root.render(<Harness escape={latest} />));
    expect(document.activeElement).toBe(middle);
    await act(async () => middle.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true })));
    expect(latest).toHaveBeenCalledOnce();
    expect(original).not.toHaveBeenCalled();
    last.focus();
    last.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }));
    expect(document.activeElement).toBe(first);
    first.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', shiftKey: true, bubbles: true, cancelable: true }));
    expect(document.activeElement).toBe(last);
    await act(async () => root.unmount());
    expect(document.activeElement).toBe(opener);
  } finally {
    await act(async () => root.unmount());
    container.remove(); opener.remove(); visible.mockRestore();
  }
});
