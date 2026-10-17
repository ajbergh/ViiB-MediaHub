import { drawMainWaveform, getWaveformBandIndices, type MainWaveformFrame, type WaveformDrawContext } from './canvasWaveformRenderer';
import { DECK_WAVEFORM_PALETTE } from './waveformPalette';

export type RasterCanvas = HTMLCanvasElement | OffscreenCanvas;
export type RasterFactory = (width: number, height: number) => RasterCanvas;
export const createRasterCanvas: RasterFactory = (width, height) => {
  if (typeof OffscreenCanvas !== 'undefined') return new OffscreenCanvas(width, height);
  const canvas = document.createElement('canvas');
  canvas.width = width; canvas.height = height;
  return canvas;
};

/** Bounded LRU of static 512-CSS-pixel timeline tiles. Position-only frames
 * composite cached tiles, then render fresh loop/grid/cue/playhead overlays. */
export class CachedWaveformRenderer {
  private tiles = new Map<number, RasterCanvas>();
  private key: unknown[] = [];
  readonly stats = { builds: 0, hits: 0 };
  constructor(private createCanvas: RasterFactory = createRasterCanvas) {}
  clear(): void { this.tiles.clear(); this.key = []; }

  draw(ctx: WaveformDrawContext, width: number, height: number, frame: MainWaveformFrame, ratio = 1): void {
    if (frame.duration <= 0 || (!frame.threeBands && !frame.peaks?.length) || width <= 0 || height <= 0) {
      drawMainWaveform(ctx, width, height, frame); return;
    }
    const key = [frame.sourceKey, frame.peaks, frame.threeBands?.data, frame.threeBands?.peak, width, height, ratio,
      frame.duration, frame.visibleSeconds, frame.colorMode, frame.bandSelection, frame.orientation, frame.deck];
    if (key.some((value, index) => value !== this.key[index])) { this.tiles.clear(); this.key = key; }
    const vertical = frame.orientation === 'vertical';
    const span = vertical ? height : width;
    const secondsPerPixel = frame.visibleSeconds / span;
    const startPixel = frame.position / secondsPerPixel - span / 2;
    const tileSize = 512;
    const firstTile = Math.floor(startPixel / tileSize);
    const lastTile = Math.floor((startPixel + span) / tileSize);
    for (let index = firstTile; index <= lastTile; index++) {
      let tile = this.tiles.get(index);
      if (tile) { this.stats.hits++; this.tiles.delete(index); }
      else {
        const tileWidth = vertical ? width : tileSize;
        const tileHeight = vertical ? tileSize : height;
        tile = this.createCanvas(Math.max(1, Math.round(tileWidth * ratio)), Math.max(1, Math.round(tileHeight * ratio)));
        const tileContext = tile.getContext('2d') as WaveformDrawContext | null;
        if (!tileContext) { drawMainWaveform(ctx, width, height, frame); return; }
        tileContext.setTransform(ratio, 0, 0, ratio, 0, 0);
        drawMainWaveform(tileContext, tileWidth, tileHeight, {
          ...frame, position: (index * tileSize + tileSize / 2) * secondsPerPixel,
          visibleSeconds: tileSize * secondsPerPixel,
        }, { waveformOnly: true, skipBandLabels: true });
        this.stats.builds++;
      }
      this.tiles.set(index, tile);
      const offset = index * tileSize - startPixel;
      ctx.drawImage(tile, 0, 0, tile.width, tile.height, vertical ? 0 : offset, vertical ? offset : 0,
        vertical ? width : tileSize, vertical ? tileSize : height);
    }
    // Bound each cache by both tile count and approximately 16 MiB of pixels.
    const bytes = tileSize * (vertical ? width : height) * ratio * ratio * 4;
    const limit = Math.max(2, Math.min(24, Math.floor(16 * 1024 * 1024 / Math.max(1, bytes))));
    while (this.tiles.size > limit) this.tiles.delete(this.tiles.keys().next().value!);
    drawMainWaveform(ctx, width, height, frame, { omitWaveform: true });
    if (frame.threeBands && !vertical) {
      const indices = getWaveformBandIndices(frame.bandSelection);
      const palette = DECK_WAVEFORM_PALETTE[frame.deck];
      ctx.font = '9px system-ui'; ctx.textAlign = 'left';
      indices.forEach((band, lane) => {
        ctx.fillStyle = [palette.gradient.center, palette.gradient.mid, palette.gradient.edge][band];
        ctx.fillText(`${frame.threeBands!.data.source === 'spotify' ? 'Spotify' : 'Local'} ${['Low', 'Mid', 'High'][band]}`, 4, lane * height / indices.length + 10);
      });
    }
  }
}
