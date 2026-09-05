import { describe, expect, it, vi } from 'vitest';
import { DJWebGLRenderer } from './DJWebGLRenderer';

describe('waveform texture filtering', () => {
  it.each([
    { webgl2: true, floatLinear: true, expected: 'linear' },
    { webgl2: true, floatLinear: false, expected: 'nearest' },
    { webgl2: false, floatLinear: false, expected: 'linear' },
  ])('uses $expected filtering for WebGL2=$webgl2, float-linear=$floatLinear', ({ webgl2, floatLinear, expected }) => {
    const gl = {
      TEXTURE_2D: 3553, TEXTURE_MIN_FILTER: 10241, TEXTURE_MAG_FILTER: 10240,
      TEXTURE_WRAP_S: 10242, TEXTURE_WRAP_T: 10243, CLAMP_TO_EDGE: 33071,
      LINEAR: 9729, NEAREST: 9728, R32F: 33326, RED: 6403,
      FLOAT: 5126, LUMINANCE: 6409, UNSIGNED_BYTE: 5121,
      createTexture: vi.fn(() => ({})), bindTexture: vi.fn(),
      texImage2D: vi.fn(), texParameteri: vi.fn(),
      getExtension: vi.fn(() => floatLinear ? {} : null),
    };
    // Exercise actual texture allocation without a GPU/shader compiler.
    const renderer = new DJWebGLRenderer() as unknown as {
      gl: typeof gl; isWebGL2: boolean; createWaveformTextures: () => void;
    };
    renderer.gl = gl;
    renderer.isWebGL2 = webgl2;
    renderer.createWaveformTextures();
    const filteringCalls = gl.texParameteri.mock.calls.filter(([, name]) =>
      name === gl.TEXTURE_MIN_FILTER || name === gl.TEXTURE_MAG_FILTER);
    expect(filteringCalls).toHaveLength(4); // Both deck textures, min + mag.
    expect(filteringCalls.every(([, , filter]) => filter === (expected === 'linear' ? gl.LINEAR : gl.NEAREST))).toBe(true);
    if (webgl2) expect(gl.getExtension).toHaveBeenCalledWith('OES_texture_float_linear');
    else expect(gl.getExtension).not.toHaveBeenCalled();
  });
});
