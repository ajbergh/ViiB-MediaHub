import type { DJThreeBands } from '../../../../hooks/useDJThreeBands';

/** RGB bytes are supported by WebGL1 and WebGL2 without float extensions.
 * Pool maxima, never averages, to retain transients when GPU capacity is lower. */
export function packThreeBandTexture({ data, peak }: DJThreeBands, maxTextureSize: number) {
  const arrays = [data.low, data.mid, data.high];
  const length = arrays[0].length;
  const width = Math.max(1, Math.min(4096, maxTextureSize, length));
  const count = Math.min(length, width * maxTextureSize, 1 << 20);
  const height = Math.max(1, Math.ceil(count / width));
  const pixels = new Uint8Array(width * height * 4);
  for (let i = 0; i < count; i++) {
    const first = Math.floor(i * length / count);
    const end = Math.min(length, Math.ceil((i + 1) * length / count));
    for (let band = 0; band < 3; band++) {
      let value = 0;
      for (let n = first; n < end; n++) value = Math.max(value, arrays[band][n] || 0);
      pixels[i * 4 + band] = peak > 0 ? Math.round(Math.min(1, value / peak) * 255) : 0;
    }
    pixels[i * 4 + 3] = 255;
  }
  return { pixels, width, height, count };
}
