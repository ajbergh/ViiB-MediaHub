import { expect, it, vi } from 'vitest';
import { canvasRoundedRect } from './canvasRoundedRect';

it('uses the native path when supported and clamps the corner radius', () => {
  const ctx = { roundRect: vi.fn() };
  canvasRoundedRect(ctx as unknown as CanvasRenderingContext2D, 2, 3, 6, 2, 5);
  expect(ctx.roundRect).toHaveBeenCalledWith(2, 3, 6, 2, 1);
});

it('closes a rounded path when WKWebView does not expose roundRect', () => {
  const ctx = { moveTo: vi.fn(), lineTo: vi.fn(), quadraticCurveTo: vi.fn(), closePath: vi.fn() };
  canvasRoundedRect(ctx as unknown as CanvasRenderingContext2D, 2, 3, 6, 2, 5);
  expect(ctx.moveTo).toHaveBeenCalledWith(3, 3);
  expect(ctx.quadraticCurveTo.mock.calls).toEqual([
    [8, 3, 8, 4], [8, 5, 7, 5], [2, 5, 2, 4], [2, 3, 3, 3],
  ]);
  expect(ctx.closePath).toHaveBeenCalledOnce();
});
